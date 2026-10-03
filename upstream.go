package main

import (
	"log/slog"
	"net/http"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"
)

// mcpEndpoints are the last path segments the two server transports answer
// on: streamable HTTP's /mcp, and SSE's /sse stream and /message endpoint.
var mcpEndpoints = map[string]bool{"mcp": true, "sse": true, "message": true}

// echoableName limits which unknown names a 404 repeats back. Anything the
// caller sent is untrusted, so a name that is long or carries markup is
// answered with a generic message instead.
var echoableName = regexp.MustCompile(`^[A-Za-z0-9._-]+(/[A-Za-z0-9._-]+)*$`)

const maxEchoedNameLength = 128

// unreachableWarnInterval bounds how often one unreachable server is logged.
// Every MCP client retries, so an unlimited WARN per request would bury the
// "Retrying connection" lines that actually explain the outage.
const unreachableWarnInterval = time.Minute

// mcpServerNameFromPath reports which server an MCP request path under
// basePath addresses: "<basePath>/<name>/{mcp,sse,message}". ok is false for
// any other path, which is left to the dashboard or a plain 404.
func mcpServerNameFromPath(basePath, requestPath string) (name string, ok bool) {
	base := path.Clean("/" + basePath)
	if base != "/" {
		base += "/"
	}
	clean := path.Clean("/" + requestPath)
	rest, found := strings.CutPrefix(clean, base)
	if !found {
		return "", false
	}
	name, endpoint := path.Split(rest)
	name = strings.TrimSuffix(name, "/")
	if name == "" || !mcpEndpoints[endpoint] {
		return "", false
	}
	return name, true
}

// warnLimiter lets through at most one event per key and interval. Keys are
// configured server names only, so the map stays as small as the config.
type warnLimiter struct {
	interval time.Duration
	now      func() time.Time

	mu   sync.Mutex
	last map[string]time.Time
}

func newWarnLimiter(interval time.Duration, now func() time.Time) *warnLimiter {
	return &warnLimiter{interval: interval, now: now, last: make(map[string]time.Time)}
}

func (l *warnLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if last, seen := l.last[key]; seen && now.Sub(last) < l.interval {
		return false
	}
	l.last[key] = now
	return true
}

// upstreamFallback answers MCP requests for which no connected server is
// mounted. A route is published only once its backend connects, so before
// this a request for a server that was down fell through to the dashboard's
// catch-all and came back as a 405 (POST) or a 200 HTML page (GET), which MCP
// clients report as a protocol error and which hides the real outage.
//
// It answers 502 for a configured server that is not connected, 503 for a
// disabled one, and 404 for a name that is not configured. Every other path is
// handed on unchanged.
type upstreamFallback struct {
	basePath string
	// config is read per request: the management API adds and removes
	// servers while the process runs.
	config func() *Config
	warn   *warnLimiter
	// logger is the destination of the rate-limited WARN; nil means
	// slog.Default().
	logger *slog.Logger
}

func newUpstreamFallback(basePath string, config func() *Config) *upstreamFallback {
	return &upstreamFallback{
		basePath: basePath,
		config:   config,
		warn:     newWarnLimiter(unreachableWarnInterval, time.Now),
	}
}

// install puts the fallback in the two places an unmounted MCP request can
// land: the router stub of a server that was mounted and has since stopped,
// and the mux's "/" catch-all, in front of the dashboard when there is one.
// Without the dashboard "/" used to be unregistered, so non-MCP paths keep
// getting the mux's plain 404.
func (f *upstreamFallback) install(mux *http.ServeMux, router *dynamicRouter, webUI http.Handler) {
	router.setFallback(f.handler(http.NotFoundHandler()))
	if webUI == nil {
		webUI = http.NotFoundHandler()
	}
	mux.Handle("/", f.handler(webUI))
}

// handler serves MCP-shaped paths and passes everything else to next.
func (f *upstreamFallback) handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name, ok := mcpServerNameFromPath(f.basePath, r.URL.Path)
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		f.serveServer(w, r, name)
	})
}

func (f *upstreamFallback) serveServer(w http.ResponseWriter, r *http.Request, name string) {
	var clientConfig *MCPClientConfigV2
	if config := f.config(); config != nil {
		clientConfig = config.McpServers[name]
	}
	if clientConfig == nil {
		message := "unknown server"
		if len(name) <= maxEchoedNameLength && echoableName.MatchString(name) {
			message += " " + name
		}
		writeFallbackError(w, http.StatusNotFound, message)
		return
	}

	options := clientConfig.Options
	if options == nil {
		options = &OptionsV2{}
	}
	respond := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if options.Disabled {
			writeFallbackError(w, http.StatusServiceUnavailable, "server "+name+" disabled")
			return
		}
		if f.warn.allow(name) {
			f.log().Warn("upstream unreachable", "server", name, "method", r.Method, "path", r.URL.Path)
		}
		writeFallbackError(w, http.StatusBadGateway, "upstream "+name+" unreachable")
	})
	// The mounted route checks the server's tokens first, so its state is
	// revealed to no one who could not have called it when it was up.
	if len(options.AuthTokens) > 0 {
		newAuthMiddleware(options.AuthTokens)(respond).ServeHTTP(w, r)
		return
	}
	respond.ServeHTTP(w, r)
}

func (f *upstreamFallback) log() *slog.Logger {
	if f.logger != nil {
		return f.logger
	}
	return slog.Default()
}

// writeFallbackError writes {"error": message}. nosniff keeps a browser from
// rendering the body as anything but JSON.
func writeFallbackError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	writeJSON(w, status, map[string]string{"error": message})
}
