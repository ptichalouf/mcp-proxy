package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/mark3labs/mcp-go/mcp"
)

// dynamicRouter serves a set of routes that may change while the HTTP server
// is running.
//
// http.ServeMux cannot unregister a pattern, and registering one twice panics,
// so a proxy that gains and loses downstream servers at runtime cannot hand
// its routes to the mux directly. Each route is instead registered once, on
// first mount, with a stub that resolves the current handler per request.
// Mounting again swaps the handler under the stub; unmounting clears it, and
// the stub answers 404 until something is mounted there again.
//
// The zero value is not usable; call newDynamicRouter.
type dynamicRouter struct {
	mux *http.ServeMux

	mu         sync.RWMutex
	handlers   map[string]http.Handler
	registered map[string]bool
}

func newDynamicRouter(mux *http.ServeMux) *dynamicRouter {
	return &dynamicRouter{
		mux:        mux,
		handlers:   make(map[string]http.Handler),
		registered: make(map[string]bool),
	}
}

// mount publishes handler at route, replacing whatever was there.
func (r *dynamicRouter) mount(route string, handler http.Handler) {
	r.mu.Lock()
	r.handlers[route] = handler
	first := !r.registered[route]
	r.registered[route] = true
	r.mu.Unlock()

	if first {
		// ServeMux is safe to register on while it is serving; the stub keeps
		// the pattern claimed for the lifetime of the process so a later
		// remount never panics.
		r.mux.Handle(route, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			r.mu.RLock()
			current := r.handlers[route]
			r.mu.RUnlock()
			if current == nil {
				http.NotFound(w, req)
				return
			}
			current.ServeHTTP(w, req)
		}))
	}
}

// unmount makes route stop serving. The pattern stays claimed on the mux.
func (r *dynamicRouter) unmount(route string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.handlers, route)
}

// isMounted reports whether route currently has a handler.
func (r *dynamicRouter) isMounted(route string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.handlers[route] != nil
}

// proxyRuntime owns the mutable half of a running proxy: which downstream
// servers are connected, and which routes they are mounted on. It is split out
// of startHTTPServer so that the management API can start and stop individual
// servers after startup without restarting the HTTP server or disturbing the
// connections of the servers it did not touch.
type proxyRuntime struct {
	proxyConfig *MCPProxyConfigV2
	baseURL     *url.URL
	info        mcp.Implementation
	router      *dynamicRouter

	mu      sync.Mutex
	clients map[string]*Client
	// cancels ends the supervisor goroutine of each server (boot-time or
	// started later), so stopping one does not wait for the whole proxy to
	// shut down.
	cancels map[string]context.CancelFunc

	// shuttingDown tells the supervisor goroutines that shutdown has already
	// walked the clients map, so a client that finishes connecting after that
	// has to close itself instead of being left running.
	shuttingDown atomic.Bool
}

func newProxyRuntime(config *Config, baseURL *url.URL, mux *http.ServeMux) *proxyRuntime {
	return &proxyRuntime{
		proxyConfig: config.McpProxy,
		baseURL:     baseURL,
		info:        mcp.Implementation{Name: config.McpProxy.Name},
		router:      newDynamicRouter(mux),
		clients:     make(map[string]*Client, len(config.McpServers)),
		cancels:     make(map[string]context.CancelFunc),
	}
}

// route is the URL subtree a server is served under. It mirrors what
// validateServerName guarantees: the result is always a clean, rooted,
// slash-terminated path below baseURL.
func (rt *proxyRuntime) route(name string) string {
	mcpRoute := path.Join(rt.baseURL.Path, name)
	if !strings.HasPrefix(mcpRoute, "/") {
		mcpRoute = "/" + mcpRoute
	}
	if !strings.HasSuffix(mcpRoute, "/") {
		mcpRoute += "/"
	}
	return mcpRoute
}

// adopt records a freshly created client. It reports false when shutdown has
// already run, or when this supervisor was stopped (ctx ended) while the client
// was being created, in which case the caller owns closing the client - for
// stdio it holds a subprocess nobody else knows about. stop cancels ctx under
// rt.mu, so the check and the insert cannot straddle a stop.
func (rt *proxyRuntime) adopt(ctx context.Context, name string, client *Client) bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.shuttingDown.Load() || ctx.Err() != nil {
		return false
	}
	rt.clients[name] = client
	return true
}

// snapshot copies the live client set, so callers can inspect it without
// holding the lock across a downstream call.
func (rt *proxyRuntime) snapshot() map[string]*Client {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return cloneClients(rt.clients)
}

// cloneClients returns a shallow copy of m.
func cloneClients(m map[string]*Client) map[string]*Client {
	out := make(map[string]*Client, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// lookup returns the live client for a server, if it has one.
func (rt *proxyRuntime) lookup(name string) *Client {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.clients[name]
}

// serverContext derives the context one server's supervisor runs under and
// registers its cancel, so stop(name) ends that supervisor - and through it
// the client's keepalive loop - whether the server was started at boot or
// after it. Boot-time servers used to run directly under the proxy's context,
// so a reload that stopped one left its supervisor and keepalive loop running
// for the lifetime of the process.
func (rt *proxyRuntime) serverContext(ctx context.Context, name string) context.Context {
	serverCtx, cancel := context.WithCancel(ctx)
	rt.track(name, cancel)
	return serverCtx
}

// track stores the cancel func of a server's supervisor goroutine.
func (rt *proxyRuntime) track(name string, cancel context.CancelFunc) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if previous, ok := rt.cancels[name]; ok {
		previous()
	}
	rt.cancels[name] = cancel
}

// stop takes a server off the air: its route stops serving immediately, its
// supervisor goroutine ends, and its client (and stdio subprocess) is closed.
// Every other server keeps its connection.
func (rt *proxyRuntime) stop(name string) error {
	rt.mu.Lock()
	client := rt.clients[name]
	delete(rt.clients, name)
	cancel := rt.cancels[name]
	delete(rt.cancels, name)
	// Cancel under the lock: adopt and mountIfCurrent check the supervisor's
	// context under the same lock, so a supervisor that is mid-connect cannot
	// re-register or re-mount the server after this returns.
	if cancel != nil {
		cancel()
	}
	rt.mu.Unlock()

	// Unmount after the supervisor is cancelled, so it cannot mount again in
	// between.
	rt.router.unmount(rt.route(name))
	if client == nil {
		return nil
	}
	slog.Info("Stopping client", "client", name)
	recordServerLog(name, logStreamSystem, "info", "Server stopped")
	if err := client.Close(); err != nil {
		return fmt.Errorf("close client %q: %w", name, err)
	}
	return nil
}

// closeAll shuts every client down. After it returns, adopt refuses new ones.
func (rt *proxyRuntime) closeAll() error {
	rt.shuttingDown.Store(true)
	rt.mu.Lock()
	clients := cloneClients(rt.clients)
	cancels := make([]context.CancelFunc, 0, len(rt.cancels))
	for _, cancel := range rt.cancels {
		cancels = append(cancels, cancel)
	}
	rt.clients = make(map[string]*Client)
	rt.cancels = make(map[string]context.CancelFunc)
	rt.mu.Unlock()

	for _, cancel := range cancels {
		cancel()
	}
	var errs []error
	for name, client := range clients {
		slog.Info("Shutting down", "client", name)
		if err := client.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close client %q: %w", name, err))
		}
	}
	return errors.Join(errs...)
}

// readiness reports what /_readyz asks about the connected clients. started is
// supplied by the caller, which owns the startup grace period.
func (rt *proxyRuntime) readiness(started bool) readinessReport {
	if !started {
		return readinessReport{}
	}
	report := readinessReport{started: true}
	for name, client := range rt.snapshot() {
		health := client.Health()
		if health == healthUnknown {
			// Created but never connected: it has no route, so there is
			// nothing to route around.
			continue
		}
		report.mounted++
		if health == healthFailed {
			report.unhealthy = append(report.unhealthy, name)
		}
	}
	slices.Sort(report.unhealthy)
	return report
}

// mountIfCurrent mounts the server only if its supervisor is still the live
// one: a supervisor stopped (by a reload or a delete) while it was connecting
// must not publish a route for a client that has already been closed, nor
// overwrite the route of its replacement.
func (rt *proxyRuntime) mountIfCurrent(ctx context.Context, name string, client *Client, clientConfig *MCPClientConfigV2, server *Server) bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if ctx.Err() != nil || rt.clients[name] != client {
		return false
	}
	rt.mount(name, clientConfig, server)
	return true
}

// mount publishes a connected server's endpoint, wrapped in the middlewares
// its config asks for.
func (rt *proxyRuntime) mount(name string, clientConfig *MCPClientConfigV2, server *Server) {
	// Outermost first: recover also guards the middlewares below it, and the
	// logger records requests that auth rejects.
	middlewares := make([]MiddlewareFunc, 0, 3)
	middlewares = append(middlewares, recoverMiddleware(name))
	if clientConfig.Options.logEnabled() {
		middlewares = append(middlewares, loggerMiddleware(name))
	}
	if len(clientConfig.Options.AuthTokens) > 0 {
		middlewares = append(middlewares, newAuthMiddleware(clientConfig.Options.AuthTokens))
	}
	mcpRoute := rt.route(name)
	slog.Info("Handling requests", "client", name, "route", mcpRoute)
	rt.router.mount(mcpRoute, chainMiddleware(server.handler, middlewares...))
}

// supervise runs one downstream server for the lifetime of ctx: it creates the
// client, connects it (retrying while the backend is down, when the config
// allows), and mounts its route once it is up.
//
// It returns an error only when the failure is fatal for the whole process
// (options.panicIfInvalid). Every other failure is logged and the server is
// simply left unmounted, so one broken downstream cannot take the proxy down.
func (rt *proxyRuntime) supervise(ctx context.Context, name string, clientConfig *MCPClientConfigV2) error {
	slog.Info("Connecting", "client", name)
	recordServerLog(name, logStreamSystem, "info", "Connecting to downstream server")
	autoReconnect := clientConfig.Options.autoReconnect()
	interval := clientConfig.Options.reconnectInterval()

	var (
		mcpClient *Client
		server    *Server
	)
	for {
		// Creating a stdio client already spawns the subprocess, so a missing
		// command fails here rather than while connecting. Both have to obey
		// the same panicIfInvalid policy.
		if mcpClient == nil {
			created, err := newMCPClient(name, clientConfig)
			if err != nil {
				// Terminal unless it will be retried: log an error for a
				// failure that ends the attempt, and stay quiet on the retry
				// path (an unreachable backend is not an error).
				if fatal := fatalStartupError(clientConfig, err); fatal != nil {
					slog.Error("Failed to start client", "client", name, "err", redactURLCredentials(err))
					recordServerLog(name, logStreamSystem, "error", redactURLCredentials(err).Error())
					return fatal
				}
				if !autoReconnect {
					slog.Error("Failed to start client", "client", name, "err", redactURLCredentials(err))
					recordServerLog(name, logStreamSystem, "error", redactURLCredentials(err).Error())
					return nil
				}
				slog.Warn("Retrying client creation", "client", name, "err", redactURLCredentials(err), "retryIn", interval)
				recordServerLog(name, logStreamSystem, "warn", fmt.Sprintf("Retrying in %s: %s", interval, redactURLCredentials(err)))
				if !retryWait(ctx, interval) {
					return nil
				}
				continue
			}
			mcpClient = created
			if !rt.adopt(ctx, name, mcpClient) {
				// Shutdown already closed everything it knew about, and this
				// client is not in that map. Nobody else will close it - and
				// for stdio it owns a subprocess.
				slog.Info("Shutting down", "client", name)
				if cErr := mcpClient.Close(); cErr != nil {
					slog.Error("Failed to close client", "client", name, "err", cErr)
				}
				return nil
			}

			newServer, sErr := newMCPServer(name, rt.proxyConfig, clientConfig)
			if sErr != nil {
				// A malformed server definition will not fix itself, so it is
				// never retried.
				recordServerLog(name, logStreamSystem, "error", sErr.Error())
				return clientStartupError(name, clientConfig, sErr)
			}
			server = newServer
		}

		err := mcpClient.addToMCPServer(ctx, rt.info, server.mcpServer)
		if err == nil {
			if !rt.mountIfCurrent(ctx, name, mcpClient, clientConfig, server) {
				// Stopped while connecting. stop() may have run before this
				// client was in the map it walks, so close it here too
				// (Close is idempotent).
				_ = mcpClient.Close()
				return nil
			}
			slog.Info("Connected", "client", name)
			recordServerLog(name, logStreamSystem, "info", "Connected")
			return nil
		}
		if fatal := fatalStartupError(clientConfig, err); fatal != nil {
			slog.Error("Failed to start client", "client", name, "err", redactURLCredentials(err))
			recordServerLog(name, logStreamSystem, "error", redactURLCredentials(err).Error())
			return fatal
		}
		if mcpClient.closed.Load() || ctx.Err() != nil {
			// Shutdown or stop() already closed this client; not a startup
			// error.
			return nil
		}
		if !autoReconnect {
			slog.Error("Failed to start client", "client", name, "err", redactURLCredentials(err))
			recordServerLog(name, logStreamSystem, "error", redactURLCredentials(err).Error())
			return nil
		}
		if permanentStartupError(err) {
			// Retrying cannot help - interactive OAuth is needed. Report it
			// once, with the actionable message, and stop rather than
			// hammering the provider every interval.
			slog.Error("Not retrying, downstream cannot connect without intervention", "client", name, "err", redactURLCredentials(err))
			recordServerLog(name, logStreamSystem, "error", redactURLCredentials(err).Error())
			return nil
		}
		// The backend is simply not up yet: wait and try again. The route is
		// mounted only once it connects, so readiness keeps reporting it as
		// not mounted until then.
		slog.Warn("Retrying connection", "client", name, "err", redactURLCredentials(err), "retryIn", interval)
		recordServerLog(name, logStreamSystem, "warn", fmt.Sprintf("Retrying in %s: %s", interval, redactURLCredentials(err)))
		if !retryWait(ctx, interval) {
			return nil
		}
	}
}

// start launches a server that was not in the config at boot, or that has just
// been reconfigured. It returns immediately; the supervisor runs until ctx
// ends or stop is called for this name.
func (rt *proxyRuntime) start(ctx context.Context, name string, clientConfig *MCPClientConfigV2) {
	serverCtx := rt.serverContext(ctx, name)
	go func() {
		if err := rt.supervise(serverCtx, name, clientConfig); err != nil {
			// panicIfInvalid is a startup-time contract: after boot, refusing
			// to serve the other downstreams because one new entry is broken
			// would be worse than reporting it.
			slog.Error("Server failed to start", "client", name, "err", err)
		}
	}()
}
