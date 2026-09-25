package main

import (
	"embed"
	"errors"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// webDist holds the compiled management UI. It is embedded so that a release
// stays what it has always been: one static binary with no runtime assets to
// ship alongside it.
//
// The directory is committed (see web/README.md) because go:embed resolves at
// compile time: a checkout without it would not build, which would make the
// Go toolchain depend on Node. `make web` regenerates it.
//
//go:embed all:web/dist
var webDist embed.FS

const webDistRoot = "web/dist"

// errWebUIUnavailable reports a binary built against a placeholder web/dist.
// The proxy still serves MCP; only -web is refused.
var errWebUIUnavailable = errors.New("web UI assets are not embedded in this binary; rebuild with `make web`")

// webAssets returns the embedded dist directory rooted at its own top level,
// so "index.html" addresses the SPA entry point rather than
// "web/dist/index.html".
func webAssets() (fs.FS, error) {
	sub, err := fs.Sub(webDist, webDistRoot)
	if err != nil {
		return nil, err
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return nil, errWebUIUnavailable
	}
	return sub, nil
}

// newWebUIHandler serves the embedded single-page application.
//
// Anything that exists on disk is served as a file. Everything else falls back
// to index.html so that a deep link (or a browser refresh on one) reaches the
// client-side router instead of a 404 - the standard SPA contract. The
// fallback deliberately does not apply to /api/, which is routed separately
// and must keep returning JSON errors.
func newWebUIHandler(basePath string) (http.Handler, error) {
	assets, err := webAssets()
	if err != nil {
		return nil, err
	}
	fileServer := http.FileServerFS(assets)
	prefix := normalizeWebBasePath(basePath)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		name := strings.TrimPrefix(path.Clean("/"+strings.TrimPrefix(r.URL.Path, prefix)), "/")
		// The shell is written directly rather than delegated: FileServerFS
		// redirects every request for an index.html to "./", which would turn
		// the mount root of a subtree into a 301 instead of a page.
		if name == "" || name == "." || name == "index.html" {
			serveWebIndex(w, r, assets)
			return
		}

		info, statErr := fs.Stat(assets, name)
		if statErr != nil || info.IsDir() {
			// Unknown path: hand it to the SPA router. The status stays 200
			// because the client decides whether the route exists.
			serveWebIndex(w, r, assets)
			return
		}

		setWebCacheHeaders(w, name)
		// FileServerFS resolves against the request path, so strip the mount
		// prefix before delegating.
		if prefix != "/" {
			r = r.Clone(r.Context())
			r.URL.Path = "/" + name
		}
		fileServer.ServeHTTP(w, r)
	}), nil
}

// serveWebIndex writes the SPA shell. It is never cached: index.html names the
// content-hashed bundles, so a stale copy pins a browser to a deleted build.
func serveWebIndex(w http.ResponseWriter, r *http.Request, assets fs.FS) {
	data, err := fs.ReadFile(assets, "index.html")
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, must-revalidate")
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(data)
	}
}

// setWebCacheHeaders marks Vite's content-hashed bundles immutable. Their
// names change whenever their contents do, so a year-long cache is safe and
// removes a round trip per asset on every page load.
func setWebCacheHeaders(w http.ResponseWriter, name string) {
	if strings.HasPrefix(name, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		return
	}
	w.Header().Set("Cache-Control", "no-cache, must-revalidate")
}

// normalizeWebBasePath returns basePath as a rooted, slash-terminated prefix,
// which is both what http.ServeMux needs to match a subtree and what the
// handler strips from an incoming path.
func normalizeWebBasePath(basePath string) string {
	trimmed := strings.TrimSpace(basePath)
	if trimmed == "" || trimmed == "/" {
		return "/"
	}
	cleaned := path.Clean("/" + strings.Trim(trimmed, "/"))
	if cleaned == "/" {
		return "/"
	}
	return cleaned + "/"
}
