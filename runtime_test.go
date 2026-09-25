package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
)

func handlerWriting(body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	})
}

func getVia(mux *http.ServeMux, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

// This is the whole reason dynamicRouter exists: http.ServeMux cannot
// unregister a pattern and panics when one is registered twice, so a server
// that is removed and added back under the same name would take the process
// down.
func TestDynamicRouterMountUnmountRemount(t *testing.T) {
	mux := http.NewServeMux()
	router := newDynamicRouter(mux)
	const route = "/notes/"

	if router.isMounted(route) {
		t.Error("a fresh router reports a route as mounted")
	}

	router.mount(route, handlerWriting("first"))
	if !router.isMounted(route) {
		t.Error("isMounted is false right after mount")
	}
	if rec := getVia(mux, route); rec.Body.String() != "first" {
		t.Errorf("body = %q, want %q", rec.Body.String(), "first")
	}

	// Mounting again swaps the handler under the stub rather than registering
	// the pattern a second time.
	router.mount(route, handlerWriting("second"))
	if rec := getVia(mux, route); rec.Body.String() != "second" {
		t.Errorf("after remount body = %q, want %q", rec.Body.String(), "second")
	}

	router.unmount(route)
	if router.isMounted(route) {
		t.Error("isMounted is true after unmount")
	}
	rec := getVia(mux, route)
	if rec.Code != http.StatusNotFound {
		t.Errorf("after unmount status = %d, want 404", rec.Code)
	}

	// The pattern is still claimed on the mux, so this must not panic.
	router.mount(route, handlerWriting("third"))
	if rec := getVia(mux, route); rec.Code != http.StatusOK || rec.Body.String() != "third" {
		t.Errorf("after remount status = %d body = %q, want 200 %q", rec.Code, rec.Body.String(), "third")
	}

	// Unmounting something that was never mounted is how reload treats a
	// server that failed to start, so it has to be a no-op.
	router.unmount("/never-mounted/")
}

func TestDynamicRouterServesSubtree(t *testing.T) {
	mux := http.NewServeMux()
	router := newDynamicRouter(mux)
	router.mount("/notes/", handlerWriting("notes"))

	for _, target := range []string{"/notes/", "/notes/sse", "/notes/messages/1"} {
		if rec := getVia(mux, target); rec.Body.String() != "notes" {
			t.Errorf("%s: body = %q, want the subtree handler", target, rec.Body.String())
		}
	}
	if rec := getVia(mux, "/other/"); rec.Code != http.StatusNotFound {
		t.Errorf("/other/: status = %d, want 404", rec.Code)
	}
}

// mount and unmount run from the management API's goroutine while the HTTP
// server serves from its own, so the handler map is read and written
// concurrently. Meaningful only under -race, harmless otherwise.
func TestDynamicRouterConcurrentMountAndServe(t *testing.T) {
	mux := http.NewServeMux()
	router := newDynamicRouter(mux)
	const route = "/churn/"
	router.mount(route, handlerWriting("x"))

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			router.mount(route, handlerWriting(fmt.Sprintf("v%d", i)))
			router.unmount(route)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			getVia(mux, route)
			router.isMounted(route)
		}
	}()
	wg.Wait()
}

func TestProxyRuntimeRoute(t *testing.T) {
	cases := []struct {
		baseURL string
		name    string
		want    string
	}{
		{"http://localhost:9090", "notes", "/notes/"},
		{"http://localhost:9090/", "notes", "/notes/"},
		{"http://localhost:9090/mcp", "notes", "/mcp/notes/"},
		{"http://localhost:9090/mcp/", "notes", "/mcp/notes/"},
		{"http://localhost:9090/mcp/", "group/notes", "/mcp/group/notes/"},
	}
	for _, tc := range cases {
		baseURL, err := url.Parse(tc.baseURL)
		if err != nil {
			t.Fatalf("parse %q: %v", tc.baseURL, err)
		}
		rt := &proxyRuntime{baseURL: baseURL}
		if got := rt.route(tc.name); got != tc.want {
			t.Errorf("route(%q) under %q = %q, want %q", tc.name, tc.baseURL, got, tc.want)
		}
	}
}

// A downstream that spews must not be able to grow the proxy's memory without
// bound; the UI only ever shows a recent tail.
func TestLogStoreRingIsBounded(t *testing.T) {
	store := newLogStore()
	for i := 0; i < logRingSize*2; i++ {
		store.append("notes", logEntry{Message: fmt.Sprintf("line-%d", i)})
	}

	all := store.tail("notes", 0)
	if len(all) != logRingSize {
		t.Fatalf("kept %d lines, want the ring size %d", len(all), logRingSize)
	}
	// Oldest first, and the oldest surviving line is the one that just fits.
	if want := fmt.Sprintf("line-%d", logRingSize*2-logRingSize); all[0].Message != want {
		t.Errorf("first kept line = %q, want %q", all[0].Message, want)
	}
	if want := fmt.Sprintf("line-%d", logRingSize*2-1); all[len(all)-1].Message != want {
		t.Errorf("last kept line = %q, want %q", all[len(all)-1].Message, want)
	}

	if got := store.tail("notes", 5); len(got) != 5 || got[4].Message != all[len(all)-1].Message {
		t.Errorf("tail(5) = %d lines ending %q, want the 5 newest", len(got), got[len(got)-1].Message)
	}
	if got := store.tail("notes", logRingSize*10); len(got) != logRingSize {
		t.Errorf("a limit above the ring size returned %d lines", len(got))
	}
	if got := store.tail("unknown-server", 0); len(got) != 0 {
		t.Errorf("an unknown server returned %d lines", len(got))
	}
	// tail hands the UI a copy; mutating it must not reach the ring.
	all[0].Message = "mutated"
	if store.tail("notes", 0)[0].Message == "mutated" {
		t.Error("tail returned the ring itself, not a copy")
	}
}

// Uninstalling and reinstalling under the same name must not resurrect the
// previous server's output.
func TestLogStoreForget(t *testing.T) {
	store := newLogStore()
	store.append("notes", logEntry{Message: "old"})
	store.append("other", logEntry{Message: "kept"})

	store.forget("notes")
	if got := store.tail("notes", 0); len(got) != 0 {
		t.Errorf("forget left %d lines", len(got))
	}
	if got := store.tail("other", 0); len(got) != 1 {
		t.Errorf("forget dropped an unrelated server's %d lines", len(got))
	}
	store.forget("never-existed")
}

// With -web off there is no store, and the proxy must behave exactly as it did
// before this feature existed: no allocation, no bookkeeping.
func TestRecordServerLogIsNoOpWithoutStore(t *testing.T) {
	activeLogStore.Store(nil)
	recordServerLog("notes", logStreamStderr, "error", "nobody is listening")

	store := newLogStore()
	activeLogStore.Store(store)
	t.Cleanup(func() { activeLogStore.Store(nil) })

	recordServerLog("notes", logStreamStderr, "error", "boom\n")
	// Blank and whitespace-only lines are what a process flushing its buffer
	// produces; they would be noise in the viewer.
	recordServerLog("notes", logStreamStdout, "info", "\n")
	recordServerLog("notes", logStreamStdout, "info", "")

	entries := store.tail("notes", 0)
	if len(entries) != 1 {
		t.Fatalf("recorded %d lines, want 1", len(entries))
	}
	entry := entries[0]
	if entry.Message != "boom" {
		t.Errorf("message = %q, want the trailing newline trimmed", entry.Message)
	}
	if entry.Stream != logStreamStderr || entry.Level != "error" || entry.Source != "notes" {
		t.Errorf("entry = %+v, want the stream, level and source carried through", entry)
	}
	if entry.Timestamp == "" {
		t.Error("entry has no timestamp")
	}
}
