package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"slices"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"golang.org/x/sync/errgroup"
)

type MiddlewareFunc func(http.Handler) http.Handler

// chainMiddleware wraps h so that middlewares run in the order given: the
// first one is outermost and sees every request, including those the ones
// after it reject or panic on.
func chainMiddleware(h http.Handler, middlewares ...MiddlewareFunc) http.Handler {
	for _, middleware := range slices.Backward(middlewares) {
		h = middleware(h)
	}
	return h
}

const bearerPrefix = "Bearer "

// bearerToken extracts the credentials from an Authorization header. RFC 7235
// makes the scheme name case-insensitive, so "bearer" is as valid as "Bearer".
// A bare token with no scheme is also accepted, which earlier versions allowed.
func bearerToken(header string) string {
	header = strings.TrimSpace(header)
	if len(header) >= len(bearerPrefix) && strings.EqualFold(header[:len(bearerPrefix)], bearerPrefix) {
		header = header[len(bearerPrefix):]
	}
	return strings.TrimSpace(header)
}

// tokenAllowed compares against every configured token in constant time, so
// response timing cannot be used to recover a valid one.
func tokenAllowed(tokens []string, candidate string) bool {
	if candidate == "" {
		return false
	}
	allowed := false
	for _, token := range tokens {
		if subtle.ConstantTimeCompare([]byte(token), []byte(candidate)) == 1 {
			allowed = true
		}
	}
	return allowed
}

// newAuthMiddleware is only attached when at least one token is configured.
func newAuthMiddleware(tokens []string) MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !tokenAllowed(tokens, bearerToken(r.Header.Get("Authorization"))) {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func loggerMiddleware(prefix string) MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			slog.Info("Request", "client", prefix, "method", r.Method, "path", r.URL.Path)
			next.ServeHTTP(w, r)
		})
	}
}

func recoverMiddleware(prefix string) MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					slog.Error("Recovered from panic", "client", prefix, "err", err)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// healthHandler returns an unauthenticated handler for liveness/readiness
// probes. It responds to GET with a small JSON status document and to HEAD with
// an empty body, so it can be used by Docker, reverse proxies, and monitoring
// without speaking MCP or providing the proxy auth token.
//
// A nil readiness func makes the handler report OK as soon as the process
// serves requests (liveness). The readiness endpoint passes one that reports
// 503 while clients are still mounting their routes, and again whenever a
// downstream connection that used to work has broken, so a load balancer can
// route around a proxy whose backends are gone.
func healthHandler(config *Config, readiness func() readinessReport) http.HandlerFunc {
	return healthHandlerFunc(func() *Config { return config }, readiness)
}

// healthHandlerFunc is healthHandler over a config that may change. The
// management API can add and remove servers while the process runs, so the
// server count has to be read per request rather than captured once.
func healthHandlerFunc(currentConfig func() *Config, readiness func() readinessReport) http.HandlerFunc {
	type healthResponse struct {
		Name        string   `json:"name"`
		ServerCount int      `json:"serverCount"`
		Status      string   `json:"status"`
		Unhealthy   []string `json:"unhealthy,omitempty"`
		Version     string   `json:"version"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		config := currentConfig()
		enabled := 0
		for _, clientConfig := range config.McpServers {
			if clientConfig.Options == nil || !clientConfig.Options.Disabled {
				enabled++
			}
		}
		resp := healthResponse{
			Name:        config.McpProxy.Name,
			ServerCount: enabled,
			Status:      "ok",
			Version:     config.McpProxy.Version,
		}
		code := http.StatusOK
		if readiness != nil {
			report := readiness()
			switch {
			case !report.started:
				code, resp.Status = http.StatusServiceUnavailable, "initializing"
			case len(report.unhealthy) > 0:
				code, resp.Status, resp.Unhealthy = http.StatusServiceUnavailable, "degraded", report.unhealthy
			case report.mounted == 0 && enabled > 0:
				// Nothing to name as broken, but every MCP route 404s, so this
				// proxy must not stay in a load balancer's rotation.
				code, resp.Status = http.StatusServiceUnavailable, "unavailable"
			}
		}
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(code)
			_ = json.NewEncoder(w).Encode(resp)
		case http.MethodHead:
			w.WriteHeader(code)
		}
	}
}

// readinessReport is what /_readyz asks the running proxy about its clients.
type readinessReport struct {
	// started is false until every client has finished connecting, or the
	// startup grace period has expired.
	started bool
	// mounted counts the clients that connected at least once, and so have a
	// route. Zero means the proxy has nothing to serve at all.
	mounted int
	// unhealthy names the clients that connected and later failed their
	// keepalive ping.
	unhealthy []string
}

// fatalStartupError applies the panicIfInvalid half of the per-server failure
// policy: it returns err when that server asked for a fast, fatal failure, and
// nil otherwise. It deliberately does not log, because the caller knows whether
// the failure is terminal (log an error) or about to be retried (log nothing;
// the retry line carries the detail). Logging here unconditionally would emit
// an ERROR for every retry attempt of a backend that is simply not up yet.
//
// The returned error is redacted, because it is propagated out of the startup
// errorGroup and logged by main: a transport error embeds the downstream URL,
// which may carry a credential in its query.
func fatalStartupError(clientConfig *MCPClientConfigV2, err error) error {
	if clientConfig.Options.panicIfInvalid() {
		return redactURLCredentials(err)
	}
	return nil
}

// clientStartupError reports a downstream that could not be started and will
// not be retried, so the rest of the proxy still serves. It returns err when
// panicIfInvalid makes the failure fatal for the whole process.
func clientStartupError(name string, clientConfig *MCPClientConfigV2, err error) error {
	slog.Error("Failed to start client", "client", name, "err", redactURLCredentials(err))
	return fatalStartupError(clientConfig, err)
}

// permanentStartupError reports whether retrying cannot help: a downstream that
// needs interactive OAuth authorization stays unconnectable until someone runs
// `mcp-proxy -authorize`, so autoReconnect must stop rather than retry it every
// interval forever and keep hitting the provider. The message already carries
// the authorize command (see oauthAwareError), so the single ERROR is actionable.
//
// Only this case is classified. A plain 401 from a static bearer-token server is
// a transport error indistinguishable from any other connection failure, and a
// missing stdio command is expected to appear later, so both still retry.
func permanentStartupError(err error) bool {
	return client.IsOAuthAuthorizationRequiredError(err)
}

// retryWait sleeps out one reconnect interval, reporting false if the context
// ended first (the proxy is shutting down).
func retryWait(ctx context.Context, interval time.Duration) bool {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// proxyOptions carries what the management API needs and the proxy itself does
// not. The zero value is exactly the upstream behaviour: no management API, no
// web UI, and nothing written to the config file.
type proxyOptions struct {
	// configPath is the file the management API edits. Empty disables it.
	configPath string
	// loadOpts are the flags load() was called with, so a reload parses the
	// config the same way the initial boot did.
	loadOpts loadOptions
	// webUI enables the management API and the embedded dashboard.
	webUI bool
}

func startHTTPServer(config *Config) error {
	return startHTTPServerWithOptions(config, proxyOptions{})
}

func startHTTPServerWithOptions(config *Config, opts proxyOptions) error {
	baseURL, uErr := url.Parse(config.McpProxy.BaseURL)
	if uErr != nil {
		// baseURL is validated in validateConfig, so this is belt-and-braces;
		// redact anyway since the parse error embeds the URL.
		return redactURLCredentials(uErr)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var errorGroup errgroup.Group
	httpMux := http.NewServeMux()
	httpServer := &http.Server{
		Addr:    config.McpProxy.Addr,
		Handler: httpMux,
	}
	runtime := newProxyRuntime(config, baseURL, httpMux)

	// Unauthenticated health endpoints for liveness/readiness probes.
	var started atomic.Bool
	readiness := func() readinessReport {
		return runtime.readiness(started.Load())
	}

	// currentConfig is what the health endpoints count servers from. Without
	// the management API it never changes; with it, servers come and go.
	currentConfig := func() *Config { return config }
	var (
		manager *Manager
		webUI   http.Handler
	)
	if opts.webUI {
		manager = newManager(ctx, opts.configPath, opts.loadOpts, config, runtime)
		manager.registerRoutes(httpMux, config.McpProxy.Options.AuthTokens)
		currentConfig = manager.snapshotConfig

		handler, wErr := newWebUIHandler("/")
		if wErr != nil {
			return fmt.Errorf("web UI: %w", wErr)
		}
		webUI = handler
		slog.Info("Management UI enabled", "route", "/", "config", opts.configPath)
	}
	// "/" is the least specific pattern ServeMux knows, so the MCP subtrees
	// and the /api and /_healthz routes all still win. The fallback claims the
	// MCP-shaped paths of servers that are not connected (502) or not
	// configured (404), and hands every other path to the dashboard - or, without
	// -web, to the same plain 404 as before.
	newUpstreamFallback(baseURL.Path, currentConfig).install(httpMux, runtime.router, webUI)

	// Method-less patterns: with "/" claiming every method, a "GET /_healthz"
	// pattern would send a POST to the catch-all instead of answering 405, so
	// the handler rejects other methods itself.
	httpMux.HandleFunc("/_healthz", healthHandlerFunc(currentConfig, nil))
	httpMux.HandleFunc("/_readyz", healthHandlerFunc(currentConfig, readiness))

	for name, clientConfig := range config.McpServers {
		if clientConfig.Options.Disabled {
			slog.Info("Disabled", "client", name)
			continue
		}
		serverCtx := runtime.serverContext(ctx, name)
		errorGroup.Go(func() error {
			return runtime.supervise(serverCtx, name, clientConfig)
		})
	}

	initializationDone := make(chan error, 1)
	go func() {
		initializationDone <- errorGroup.Wait()
	}()

	serverDone := make(chan error, 1)
	go func() {
		slog.Info("Starting server", "type", config.McpProxy.Type, "addr", config.McpProxy.Addr)
		hErr := httpServer.ListenAndServe()
		if errors.Is(hErr, http.ErrServerClosed) {
			hErr = nil
		}
		serverDone <- hErr
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	shutdown := func() error {
		cancel()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		var shutdownErrors []error
		if err := httpServer.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			// An SSE stream never goes idle, so a client that is still
			// connected will always outlast the grace period. Force those
			// connections closed rather than reporting a failed shutdown.
			slog.Warn("Graceful shutdown timed out, closing open connections", "err", err)
			if err := httpServer.Close(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				shutdownErrors = append(shutdownErrors, err)
			}
		}
		if err := runtime.closeAll(); err != nil {
			shutdownErrors = append(shutdownErrors, err)
		}
		return errors.Join(shutdownErrors...)
	}

	// A downstream can be slow for legitimate reasons (npx fetching a package on
	// first run) and the servers that did connect are already serving, so
	// readiness waits only this long for the stragglers.
	grace := config.McpProxy.startupGrace()
	graceTimer := time.NewTimer(grace)
	defer graceTimer.Stop()

	for {
		select {
		case err := <-initializationDone:
			initializationDone = nil
			if err != nil {
				_ = shutdown()
				return fmt.Errorf("failed to initialize clients: %w", err)
			}
			started.Store(true)
			slog.Info("All clients initialized")
		case <-graceTimer.C:
			// Never let one slow downstream keep the proxy out of rotation.
			if started.CompareAndSwap(false, true) {
				slog.Warn("Reporting ready while clients are still connecting", "grace", grace)
			}
		case err := <-serverDone:
			_ = shutdown()
			if err != nil {
				return fmt.Errorf("HTTP server failed: %w", err)
			}
			return nil
		case <-sigChan:
			slog.Info("Shutdown signal received")
			return shutdown()
		}
	}
}
