package main

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// firstHashedAsset returns the name of one file under assets/, which Vite
// content-hashes. The exact name changes on every UI rebuild, so it is
// discovered rather than hardcoded.
func firstHashedAsset(t *testing.T, assets fs.FS) string {
	t.Helper()
	entries, err := fs.ReadDir(assets, "assets")
	if err != nil {
		t.Fatalf("read embedded assets dir: %v", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			return "assets/" + entry.Name()
		}
	}
	t.Fatal("embedded assets dir contains no files")
	return ""
}

func TestWebAssetsEmbedIndex(t *testing.T) {
	assets, err := webAssets()
	if err != nil {
		t.Fatalf("webAssets: %v", err)
	}
	if _, err := fs.Stat(assets, "index.html"); err != nil {
		t.Fatalf("index.html must be embedded at the root of the sub FS: %v", err)
	}
	// Rooted at web/dist, so the build-time prefix must not leak into paths.
	if _, err := fs.Stat(assets, "web/dist/index.html"); err == nil {
		t.Fatal("assets FS is not rooted at web/dist")
	}
}

func TestWebUIHandlerServesIndexAtRoot(t *testing.T) {
	handler, err := newWebUIHandler("/")
	if err != nil {
		t.Fatalf("newWebUIHandler: %v", err)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	if !strings.Contains(rec.Body.String(), "<div id=\"root\"") {
		t.Errorf("body does not look like the SPA shell: %q", rec.Body.String())
	}
}

// A deep link is the whole point of the fallback: the client-side router owns
// these paths, so the server must answer with the shell rather than a 404.
func TestWebUIHandlerFallsBackToIndex(t *testing.T) {
	handler, err := newWebUIHandler("/")
	if err != nil {
		t.Fatalf("newWebUIHandler: %v", err)
	}

	for _, path := range []string{"/marketplace", "/servers/github/logs", "/does/not/exist"} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

		if rec.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200", path, rec.Code)
		}
		if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "no-cache") {
			t.Errorf("%s: Cache-Control = %q, want no-cache (index.html names hashed bundles)", path, got)
		}
	}
}

func TestWebUIHandlerCachesHashedAssetsImmutably(t *testing.T) {
	assets, err := webAssets()
	if err != nil {
		t.Fatalf("webAssets: %v", err)
	}
	asset := firstHashedAsset(t, assets)

	handler, err := newWebUIHandler("/")
	if err != nil {
		t.Fatalf("newWebUIHandler: %v", err)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/"+asset, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("%s: status = %d, want 200", asset, rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "immutable") {
		t.Errorf("%s: Cache-Control = %q, want immutable", asset, got)
	}
	if rec.Body.Len() == 0 {
		t.Errorf("%s: served an empty body", asset)
	}
}

func TestWebUIHandlerRejectsWrites(t *testing.T) {
	handler, err := newWebUIHandler("/")
	if err != nil {
		t.Fatalf("newWebUIHandler: %v", err)
	}

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(method, "/", nil))

		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: status = %d, want 405", method, rec.Code)
		}
		if got := rec.Header().Get("Allow"); got != "GET, HEAD" {
			t.Errorf("%s: Allow = %q, want \"GET, HEAD\"", method, got)
		}
	}
}

func TestWebUIHandlerHeadHasNoBody(t *testing.T) {
	handler, err := newWebUIHandler("/")
	if err != nil {
		t.Fatalf("newWebUIHandler: %v", err)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodHead, "/dashboard", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("HEAD returned %d bytes of body", rec.Body.Len())
	}
}

// The handler reads from an embedded FS, so a traversal cannot reach the real
// filesystem - but it must not serve another embedded file either, and must
// not 500. Cleaning the path first sends these to the SPA fallback.
func TestWebUIHandlerRejectsTraversal(t *testing.T) {
	handler, err := newWebUIHandler("/")
	if err != nil {
		t.Fatalf("newWebUIHandler: %v", err)
	}

	for _, path := range []string{"/../config.json", "/assets/../../go.mod", "/../../etc/passwd"} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

		if rec.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want the SPA fallback (200)", path, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
			t.Errorf("%s: served %q, want the SPA shell", path, ct)
		}
	}
}

func TestWebUIHandlerHonoursBasePath(t *testing.T) {
	assets, err := webAssets()
	if err != nil {
		t.Fatalf("webAssets: %v", err)
	}
	asset := firstHashedAsset(t, assets)

	handler, err := newWebUIHandler("/ui")
	if err != nil {
		t.Fatalf("newWebUIHandler: %v", err)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ui/", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<div id=\"root\"") {
		t.Fatalf("/ui/ did not serve the shell: status %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ui/"+asset, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/ui/%s: status = %d, want 200", asset, rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "immutable") {
		t.Errorf("/ui/%s: Cache-Control = %q, want immutable", asset, got)
	}
}

// http.FileServerFS answers a request for an index.html with a 301 to "./".
// The shell has to be served, not redirected to, or the mount root of a
// subtree is a redirect loop away from the page.
func TestWebUIHandlerServesIndexPathWithoutRedirect(t *testing.T) {
	for prefix, target := range map[string]string{
		"/":   "/index.html",
		"/ui": "/ui/index.html",
	} {
		handler, err := newWebUIHandler(prefix)
		if err != nil {
			t.Fatalf("newWebUIHandler(%q): %v", prefix, err)
		}

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))

		if rec.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200", target, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "<div id=\"root\"") {
			t.Errorf("%s: did not serve the SPA shell", target)
		}
	}
}

func TestNormalizeWebBasePath(t *testing.T) {
	cases := map[string]string{
		"":         "/",
		"/":        "/",
		"  ":       "/",
		"ui":       "/ui/",
		"/ui":      "/ui/",
		"/ui/":     "/ui/",
		"//ui//":   "/ui/",
		"/a/b":     "/a/b/",
		"/ui/../x": "/x/",
	}
	for in, want := range cases {
		if got := normalizeWebBasePath(in); got != want {
			t.Errorf("normalizeWebBasePath(%q) = %q, want %q", in, got, want)
		}
	}
}
