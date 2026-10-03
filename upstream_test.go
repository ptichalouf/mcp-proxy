package main

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeClock is the injected time source of warnLimiter.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

const fallbackTestToken = "secured-token"

func fallbackTestConfig(baseURL string) *Config {
	return &Config{
		McpProxy: &MCPProxyConfigV2{Name: "test", BaseURL: baseURL},
		McpServers: map[string]*MCPClientConfigV2{
			"notes":       {Command: "server", Options: &OptionsV2{}},
			"group/notes": {Command: "server", Options: &OptionsV2{}},
			"secured":     {Command: "server", Options: &OptionsV2{AuthTokens: []string{fallbackTestToken}}},
			"off":         {Command: "server", Options: &OptionsV2{Disabled: true}},
			"live":        {Command: "server", Options: &OptionsV2{}},
		},
	}
}

// fallbackFixture wires a mux the way startHTTPServerWithOptions does: the
// dynamic router for connected servers, the fallback in front of the web UI
// (or of nothing, without -web), and a log buffer behind the fallback.
type fallbackFixture struct {
	mux      *http.ServeMux
	router   *dynamicRouter
	fallback *upstreamFallback
	clock    *fakeClock
	logs     *bytes.Buffer
}

func newFallbackFixture(t *testing.T, config *Config, withWebUI bool) *fallbackFixture {
	t.Helper()
	mux := http.NewServeMux()
	router := newDynamicRouter(mux)
	clock := &fakeClock{now: time.Date(2026, 10, 3, 11, 41, 0, 0, time.UTC)}
	logs := &bytes.Buffer{}

	var webUI http.Handler
	if withWebUI {
		h, err := newWebUIHandler("/")
		if err != nil {
			t.Fatalf("newWebUIHandler: %v", err)
		}
		webUI = h
	}
	basePath := "/"
	if config.McpProxy.BaseURL != "" {
		basePath = mustParseURL(t, config.McpProxy.BaseURL).Path
	}
	fallback := newUpstreamFallback(basePath, func() *Config { return config })
	fallback.warn = newWarnLimiter(time.Minute, clock.Now)
	fallback.logger = slog.New(slog.NewTextHandler(&syncWriter{w: logs}, nil))
	fallback.install(mux, router, webUI)
	return &fallbackFixture{mux: mux, router: router, fallback: fallback, clock: clock, logs: logs}
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	return u
}

// syncWriter serializes writes to the shared log buffer.
type syncWriter struct {
	mu sync.Mutex
	w  *bytes.Buffer
}

func (s *syncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}

func (f *fallbackFixture) do(method, target, token string) *httptest.ResponseRecorder {
	var body *strings.Reader
	if method == http.MethodPost {
		body = strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`)
	} else {
		body = strings.NewReader("")
	}
	req := httptest.NewRequest(method, target, body)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	f.mux.ServeHTTP(rec, req)
	return rec
}

func TestUpstreamFallbackRoutes(t *testing.T) {
	t.Parallel()

	const (
		jsonError = "json"
		html      = "html"
		plain     = "plain"
	)
	cases := []struct {
		name      string
		method    string
		target    string
		token     string
		wantCode  int
		wantKind  string
		wantError string
	}{
		// Configured but not connected: the real failure, named.
		{"configured POST mcp", http.MethodPost, "/notes/mcp", "", http.StatusBadGateway, jsonError, "upstream notes unreachable"},
		{"configured GET mcp", http.MethodGet, "/notes/mcp", "", http.StatusBadGateway, jsonError, "upstream notes unreachable"},
		{"configured DELETE mcp", http.MethodDelete, "/notes/mcp", "", http.StatusBadGateway, jsonError, "upstream notes unreachable"},
		{"configured GET sse", http.MethodGet, "/notes/sse", "", http.StatusBadGateway, jsonError, "upstream notes unreachable"},
		{"configured POST message", http.MethodPost, "/notes/message?sessionId=abc", "", http.StatusBadGateway, jsonError, "upstream notes unreachable"},
		{"configured trailing slash", http.MethodPost, "/notes/mcp/", "", http.StatusBadGateway, jsonError, "upstream notes unreachable"},
		{"configured nested name", http.MethodPost, "/group/notes/mcp", "", http.StatusBadGateway, jsonError, "upstream group/notes unreachable"},

		// Unknown under an MCP-shaped path: honest JSON 404, not 405 or HTML.
		{"unknown POST mcp", http.MethodPost, "/does-not-exist/mcp", "", http.StatusNotFound, jsonError, "unknown server does-not-exist"},
		{"unknown GET mcp", http.MethodGet, "/does-not-exist/mcp", "", http.StatusNotFound, jsonError, "unknown server does-not-exist"},
		{"unknown GET sse", http.MethodGet, "/does-not-exist/sse", "", http.StatusNotFound, jsonError, "unknown server does-not-exist"},
		{"unknown POST message", http.MethodPost, "/does-not-exist/message", "", http.StatusNotFound, jsonError, "unknown server does-not-exist"},

		// Disabled on purpose is neither unreachable nor unknown.
		{"disabled", http.MethodPost, "/off/mcp", "", http.StatusServiceUnavailable, jsonError, "server off disabled"},

		// The server's own auth runs before its state is revealed, exactly
		// as it would on the mounted route.
		{"auth missing token", http.MethodPost, "/secured/mcp", "", http.StatusUnauthorized, plain, ""},
		{"auth wrong token", http.MethodPost, "/secured/mcp", "not-the-token", http.StatusUnauthorized, plain, ""},
		{"auth wrong scheme only", http.MethodPost, "/secured/mcp", " ", http.StatusUnauthorized, plain, ""},
		{"auth valid token", http.MethodPost, "/secured/mcp", fallbackTestToken, http.StatusBadGateway, jsonError, "upstream secured unreachable"},

		// The dashboard keeps its own paths.
		{"dashboard root", http.MethodGet, "/", "", http.StatusOK, html, ""},
		{"dashboard SPA route", http.MethodGet, "/servers/notes", "", http.StatusOK, html, ""},
		{"dashboard rejects writes", http.MethodPost, "/", "", http.StatusMethodNotAllowed, plain, ""},
		{"dashboard non-MCP subpath", http.MethodGet, "/notes/settings", "", http.StatusOK, html, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFallbackFixture(t, fallbackTestConfig("http://localhost:9090"), true)
			rec := f.do(tc.method, tc.target, tc.token)
			if rec.Code != tc.wantCode {
				t.Fatalf("%s %s: status = %d, want %d (body %q)", tc.method, tc.target, rec.Code, tc.wantCode, rec.Body.String())
			}
			switch tc.wantKind {
			case jsonError:
				assertJSONError(t, rec, tc.wantError)
			case html:
				if !strings.Contains(strings.ToLower(rec.Body.String()), "<!doctype html>") {
					t.Errorf("body is not the dashboard HTML: %.80q", rec.Body.String())
				}
			case plain:
				if strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
					t.Errorf("unexpected JSON body %q", rec.Body.String())
				}
			}
		})
	}
}

func assertJSONError(t *testing.T, rec *httptest.ResponseRecorder, want string) {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q is not JSON: %v", rec.Body.String(), err)
	}
	if len(body) != 1 || body["error"] != want {
		t.Errorf("body = %v, want exactly {\"error\":%q}", body, want)
	}
}

// A backend that comes back must serve again without a restart, and one that
// is stopped again falls back to the 502 rather than the router's bare 404.
func TestUpstreamFallbackFollowsMountState(t *testing.T) {
	t.Parallel()
	f := newFallbackFixture(t, fallbackTestConfig("http://localhost:9090"), true)

	if rec := f.do(http.MethodPost, "/live/mcp", ""); rec.Code != http.StatusBadGateway {
		t.Fatalf("before connect: status = %d, want 502", rec.Code)
	}

	f.router.mount("/live/", handlerWriting("live"))
	for _, target := range []string{"/live/mcp", "/live/sse", "/live/message"} {
		rec := f.do(http.MethodPost, target, "")
		if rec.Code != http.StatusOK || rec.Body.String() != "live" {
			t.Fatalf("after connect %s: status = %d body = %q, want 200 from the server", target, rec.Code, rec.Body.String())
		}
	}

	f.router.unmount("/live/")
	rec := f.do(http.MethodPost, "/live/mcp", "")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("after unmount: status = %d, want 502", rec.Code)
	}
	assertJSONError(t, rec, "upstream live unreachable")

	// The router stub keeps the pattern claimed; a non-MCP path under it keeps
	// the router's plain 404.
	if rec := f.do(http.MethodGet, "/live/", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("after unmount /live/: status = %d, want 404", rec.Code)
	}

	f.router.mount("/live/", handlerWriting("again"))
	if rec := f.do(http.MethodPost, "/live/mcp", ""); rec.Code != http.StatusOK || rec.Body.String() != "again" {
		t.Fatalf("after remount: status = %d body = %q, want 200", rec.Code, rec.Body.String())
	}
}

// The management API can add a server at runtime: the fallback must read the
// current config per request, not a copy taken at boot.
func TestUpstreamFallbackReadsCurrentConfig(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	config := fallbackTestConfig("http://localhost:9090")
	current := func() *Config {
		mu.Lock()
		defer mu.Unlock()
		return config
	}
	mux := http.NewServeMux()
	fallback := newUpstreamFallback("/", current)
	fallback.logger = slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	fallback.install(mux, newDynamicRouter(mux), nil)

	get := func() int {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/added/mcp", nil))
		return rec.Code
	}
	if code := get(); code != http.StatusNotFound {
		t.Fatalf("before add: status = %d, want 404", code)
	}
	mu.Lock()
	next := fallbackTestConfig("http://localhost:9090")
	next.McpServers["added"] = &MCPClientConfigV2{Command: "server", Options: &OptionsV2{}}
	config = next
	mu.Unlock()
	if code := get(); code != http.StatusBadGateway {
		t.Fatalf("after add: status = %d, want 502", code)
	}
}

// Without -web nothing used to be registered at "/": unknown paths must keep
// their plain 404, health endpoints their 405 on writes, and only MCP-shaped
// paths gain the JSON answers.
func TestUpstreamFallbackWithoutWebUI(t *testing.T) {
	t.Parallel()
	config := fallbackTestConfig("http://localhost:9090")
	f := newFallbackFixture(t, config, false)
	f.mux.HandleFunc("/_healthz", healthHandler(config, nil))

	if rec := f.do(http.MethodGet, "/", ""); rec.Code != http.StatusNotFound || strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		t.Errorf("GET /: status = %d ct = %q, want a plain 404", rec.Code, rec.Header().Get("Content-Type"))
	}
	if rec := f.do(http.MethodPost, "/_healthz", ""); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /_healthz: status = %d, want 405", rec.Code)
	}
	rec := f.do(http.MethodPost, "/does-not-exist/mcp", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown: status = %d, want 404", rec.Code)
	}
	assertJSONError(t, rec, "unknown server does-not-exist")
	rec = f.do(http.MethodPost, "/notes/mcp", "")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("configured: status = %d, want 502", rec.Code)
	}
	assertJSONError(t, rec, "upstream notes unreachable")
}

// Routes live under baseURL's path; the same names outside it are not MCP
// routes and belong to the dashboard.
func TestUpstreamFallbackHonoursBasePath(t *testing.T) {
	t.Parallel()
	f := newFallbackFixture(t, fallbackTestConfig("http://localhost:9090/hub"), true)

	rec := f.do(http.MethodPost, "/hub/notes/mcp", "")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("under base: status = %d, want 502", rec.Code)
	}
	assertJSONError(t, rec, "upstream notes unreachable")

	if rec := f.do(http.MethodGet, "/notes/mcp", ""); rec.Code != http.StatusOK || !strings.Contains(strings.ToLower(rec.Body.String()), "<!doctype html>") {
		t.Fatalf("outside base: status = %d, want the dashboard", rec.Code)
	}
}

func TestMCPServerNameFromPath(t *testing.T) {
	t.Parallel()
	cases := []struct {
		base, path string
		want       string
		ok         bool
	}{
		{"/", "/notes/mcp", "notes", true},
		{"/", "/notes/sse", "notes", true},
		{"/", "/notes/message", "notes", true},
		{"/", "/notes/mcp/", "notes", true},
		{"/", "/group/notes/mcp", "group/notes", true},
		{"", "/notes/mcp", "notes", true},
		{"/hub", "/hub/notes/mcp", "notes", true},
		{"/hub/", "/hub/notes/sse", "notes", true},
		{"/hub", "/hubnotes/mcp", "", false},
		{"/hub", "/notes/mcp", "", false},
		{"/", "/mcp", "", false},
		{"/", "/", "", false},
		{"/", "/notes/", "", false},
		{"/", "/notes/messages", "", false},
		{"/", "/notes/mcp/extra", "", false},
		{"/", "/../notes/mcp", "notes", true},
		{"/", "/a/../mcp", "", false},
	}
	for _, tc := range cases {
		got, ok := mcpServerNameFromPath(tc.base, tc.path)
		if got != tc.want || ok != tc.ok {
			t.Errorf("mcpServerNameFromPath(%q, %q) = %q, %v; want %q, %v", tc.base, tc.path, got, ok, tc.want, tc.ok)
		}
	}
}

// A name that could not be a server is never echoed back.
func TestUpstreamFallbackDoesNotEchoInvalidNames(t *testing.T) {
	t.Parallel()
	f := newFallbackFixture(t, fallbackTestConfig("http://localhost:9090"), true)
	long := strings.Repeat("a", 300)
	for _, target := range []string{"/" + long + "/mcp", "/%3Cscript%3E/mcp"} {
		rec := f.do(http.MethodPost, target, "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: status = %d, want 404", target, rec.Code)
		}
		assertJSONError(t, rec, "unknown server")
	}
}

func TestWarnLimiterAllowsOncePerServerPerInterval(t *testing.T) {
	t.Parallel()
	clock := &fakeClock{now: time.Date(2026, 10, 3, 11, 41, 0, 0, time.UTC)}
	limiter := newWarnLimiter(time.Minute, clock.Now)

	steps := []struct {
		advance time.Duration
		server  string
		want    bool
	}{
		{0, "notes", true},
		{0, "notes", false},
		{30 * time.Second, "notes", false},
		{0, "other", true},
		{29 * time.Second, "notes", false},
		{time.Second, "notes", true}, // exactly one interval after the first
		{0, "notes", false},
		{0, "other", false},
		{30 * time.Second, "other", true},
	}
	for i, step := range steps {
		clock.Advance(step.advance)
		if got := limiter.allow(step.server); got != step.want {
			t.Errorf("step %d (%s at %s): allow = %v, want %v", i, step.server, clock.Now().Format(time.TimeOnly), got, step.want)
		}
	}
}

func TestUpstreamFallbackWarnIsRateLimited(t *testing.T) {
	t.Parallel()
	f := newFallbackFixture(t, fallbackTestConfig("http://localhost:9090"), true)
	warnings := func() int {
		return strings.Count(f.logs.String(), `msg="upstream unreachable"`)
	}

	for range 5 {
		f.do(http.MethodPost, "/notes/mcp", "")
	}
	if got := warnings(); got != 1 {
		t.Fatalf("after 5 requests in the same minute: %d warnings, want 1\n%s", got, f.logs.String())
	}
	if !strings.Contains(f.logs.String(), "level=WARN") || !strings.Contains(f.logs.String(), "server=notes") {
		t.Errorf("warning lacks level or server: %s", f.logs.String())
	}

	// A second server is limited on its own.
	f.do(http.MethodGet, "/live/sse", "")
	if got := warnings(); got != 2 {
		t.Fatalf("after a request for another server: %d warnings, want 2", got)
	}

	// Unknown servers and rejected credentials do not log: anyone can send
	// them, and they say nothing about a backend.
	f.do(http.MethodPost, "/does-not-exist/mcp", "")
	f.do(http.MethodPost, "/secured/mcp", "wrong")
	if got := warnings(); got != 2 {
		t.Fatalf("unknown server or bad token logged: %d warnings, want 2", got)
	}

	f.clock.Advance(time.Minute)
	f.do(http.MethodPost, "/notes/mcp", "")
	if got := warnings(); got != 3 {
		t.Fatalf("a minute later: %d warnings, want 3", got)
	}
}

// The fallback sits in front of every dashboard request and every request for
// a down server, which clients retry in a loop; it must stay cheap.
func BenchmarkUpstreamFallback(b *testing.B) {
	config := fallbackTestConfig("http://localhost:9090")
	fallback := newUpstreamFallback("/", func() *Config { return config })
	fallback.logger = slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	handler := fallback.handler(http.NotFoundHandler())
	for _, target := range []string{"/notes/mcp", "/does-not-exist/mcp", "/servers/notes"} {
		b.Run(target, func(b *testing.B) {
			req := httptest.NewRequest(http.MethodPost, target, nil)
			b.ReportAllocs()
			for b.Loop() {
				handler.ServeHTTP(httptest.NewRecorder(), req)
			}
		})
	}
}
