package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/server"
)

// fastReconnectOptions probes fast and self-heals, which is the production
// shape (mcpProxy.options.autoReconnect=true) that exposed the leak.
func fastReconnectOptions() *OptionsV2 {
	o := &OptionsV2{
		PingInterval:      Duration(20 * time.Millisecond),
		ReconnectInterval: Duration(20 * time.Millisecond),
	}
	o.AutoReconnect.Set(true)
	return o
}

func newStopTestRuntime(t *testing.T) (*proxyRuntime, *http.ServeMux) {
	t.Helper()
	baseURL, _ := url.Parse("http://localhost/")
	config := &Config{McpProxy: &MCPProxyConfigV2{
		BaseURL: baseURL.String(),
		Name:    "test",
		Version: "1",
		Type:    MCPServerTypeStreamable,
	}}
	mux := http.NewServeMux()
	return newProxyRuntime(config, baseURL, mux), mux
}

func fastStreamableConfig(rawURL string) *MCPClientConfigV2 {
	return &MCPClientConfigV2{
		TransportType: MCPClientTypeStreamable,
		URL:           rawURL,
		Options:       fastReconnectOptions(),
	}
}

// superviseAsAtBoot runs a supervisor the way startHTTPServer does. It returns
// a channel closed when the supervisor returns, and the context the server's
// supervisor and keepalive loop run under.
func superviseAsAtBoot(rt *proxyRuntime, ctx context.Context, name string, conf *MCPClientConfigV2) (<-chan struct{}, context.Context) {
	done := make(chan struct{})
	serverCtx := rt.serverContext(ctx, name)
	go func() {
		defer close(done)
		_ = rt.supervise(serverCtx, name, conf)
	}()
	return done, serverCtx
}

func waitDone(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatalf("%s did not end", what)
	}
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// Regression for the boot path: servers started by startHTTPServer ran their
// supervisor under the proxy context with no cancel registered, so stop() (a
// reload that removed or changed them) closed the client but could not end the
// supervisor nor the keepalive loop, which kept probing a closed client for the
// lifetime of the process.
func TestStopEndsBootTimeServerCompletely(t *testing.T) {
	t.Parallel()

	downstream := newRawDownstream(t)
	downstream.setTools("alpha")
	rt, mux := newStopTestRuntime(t)

	supervisorDone, serverCtx := superviseAsAtBoot(rt, t.Context(), "boot", fastStreamableConfig(downstream.url))
	waitUntil(t, "route to mount", func() bool { return rt.router.isMounted(rt.route("boot")) })
	waitDone(t, supervisorDone, "supervisor after connect")

	client := rt.lookup("boot")
	if client == nil {
		t.Fatal("connected client is not tracked")
	}
	if err := rt.stop("boot"); err != nil {
		t.Fatalf("stop: %v", err)
	}
	// The keepalive loop runs under serverCtx and returns when it ends.
	waitDone(t, serverCtx.Done(), "context of a stopped boot-time server")
	if !client.closed.Load() {
		t.Error("stopped client was not closed")
	}

	if rt.lookup("boot") != nil {
		t.Error("stopped server is still tracked")
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/boot/", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("stopped route answered %d, want 404", rec.Code)
	}
}

// A server that is still retrying when a reload removes it must stop retrying
// and must never register a client afterwards (which would leak a transport -
// a subprocess for stdio - and resurrect a removed route).
func TestStopEndsRetryingSupervisor(t *testing.T) {
	t.Parallel()

	dead := httptest.NewServer(http.NotFoundHandler())
	deadURL := dead.URL
	dead.Close()

	rt, _ := newStopTestRuntime(t)
	supervisorDone, _ := superviseAsAtBoot(rt, t.Context(), "down", fastStreamableConfig(deadURL))
	waitUntil(t, "client to be adopted", func() bool { return rt.lookup("down") != nil })

	if err := rt.stop("down"); err != nil {
		t.Fatalf("stop: %v", err)
	}
	waitDone(t, supervisorDone, "retrying supervisor after stop")
	if rt.lookup("down") != nil {
		t.Error("a stopped supervisor re-registered its client")
	}
	if rt.router.isMounted(rt.route("down")) {
		t.Error("a stopped supervisor mounted its route")
	}
}

// adopt and mountIfCurrent must refuse a supervisor whose context ended, so a
// stop that lands mid-connect wins over the connect.
func TestStoppedSupervisorCannotAdoptOrMount(t *testing.T) {
	t.Parallel()

	rt, _ := newStopTestRuntime(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	c := &Client{name: "late"}
	if rt.adopt(ctx, "late", c) {
		t.Error("adopt accepted a client from a cancelled supervisor")
	}

	live := t.Context()
	if !rt.adopt(live, "late", c) {
		t.Fatal("adopt refused a live supervisor")
	}
	srv := &Server{mcpServer: server.NewMCPServer("late", "1"), handler: http.NotFoundHandler()}
	conf := fastStreamableConfig("http://127.0.0.1:1/")
	if rt.mountIfCurrent(ctx, "late", c, conf, srv) {
		t.Error("mountIfCurrent mounted for a cancelled supervisor")
	}
	if rt.mountIfCurrent(live, "late", &Client{name: "other"}, conf, srv) {
		t.Error("mountIfCurrent mounted a client that is not the tracked one")
	}
	if !rt.mountIfCurrent(live, "late", c, conf, srv) {
		t.Error("mountIfCurrent refused the live client")
	}
}

// A reload that changes a server stops it and starts it again under the same
// name. The replacement must serve, and the old keepalive loop must be gone.
func TestRestartReplacesClientWithoutLeakingOldLoop(t *testing.T) {
	t.Parallel()

	first := newRawDownstream(t)
	first.setTools("alpha")
	second := newRawDownstream(t)
	second.setTools("beta")
	rt, _ := newStopTestRuntime(t)

	done, oldCtx := superviseAsAtBoot(rt, t.Context(), "svc", fastStreamableConfig(first.url))
	waitDone(t, done, "first supervisor")
	old := rt.lookup("svc")
	if old == nil {
		t.Fatal("first client not tracked")
	}

	if err := rt.stop("svc"); err != nil {
		t.Fatalf("stop: %v", err)
	}
	rt.start(t.Context(), "svc", fastStreamableConfig(second.url))
	waitUntil(t, "replacement to mount", func() bool {
		c := rt.lookup("svc")
		return c != nil && c != old && c.Health() == healthOK && rt.router.isMounted(rt.route("svc"))
	})
	waitDone(t, oldCtx.Done(), "context of the replaced client's keepalive loop")
	if !old.closed.Load() {
		t.Error("replaced client was not closed")
	}
}
