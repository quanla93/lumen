// Package web embeds the built React+Vite bundle at compile time and exposes
// it as an http.Handler. The embed always succeeds because dist/ contains at
// least a committed .gitkeep; at runtime, if no real index.html is present,
// the handler serves an instructional fallback page.
//
// Production build pipeline:
//   pnpm --filter web build      # outputs web/dist/
//   cp -r web/dist/. internal/hub/web/dist/   # Makefile build-web does this
//   go build ./cmd/lumen-hub     # //go:embed picks up the copied files
//
// Dev workflow: ignore the embed entirely and run `pnpm --filter web dev`
// on :5173 (Vite proxies /api/* to the hub).
package web

import (
	"bytes"
	"embed"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"time"
)

//go:embed all:dist
var distFS embed.FS

// FS returns the embedded bundle rooted at dist/.
func FS() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return distFS
	}
	return sub
}

// Handler returns an http.Handler that serves embedded files with SPA
// fallback: unknown paths get index.html. If index.html is missing
// (web bundle wasn't built before go build), a "how to build" page is
// served with HTTP 503.
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serve(w, r, FS())
	})
}

func serve(w http.ResponseWriter, r *http.Request, root fs.FS) {
	name := strings.TrimPrefix(r.URL.Path, "/")
	if name == "" {
		name = "index.html"
	}
	if data, modTime, ok := readFile(root, name); ok {
		http.ServeContent(w, r, name, modTime, bytes.NewReader(data))
		return
	}
	// SPA fallback — serve index.html for unknown paths (React Router etc.).
	if data, modTime, ok := readFile(root, "index.html"); ok {
		http.ServeContent(w, r, "index.html", modTime, bytes.NewReader(data))
		return
	}
	notBuilt(w)
}

func readFile(root fs.FS, name string) ([]byte, time.Time, bool) {
	f, err := root.Open(name)
	if err != nil {
		return nil, time.Time{}, false
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil || stat.IsDir() {
		return nil, time.Time{}, false
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, time.Time{}, false
	}
	return data, stat.ModTime(), true
}

func notBuilt(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = w.Write([]byte(`<!doctype html>
<html><head><title>Lumen — web not embedded</title>
<meta name="viewport" content="width=device-width,initial-scale=1">
<style>body{font-family:system-ui,-apple-system,Segoe UI,Roboto,sans-serif;max-width:680px;margin:48px auto;padding:0 16px;color:#222}code,pre{background:#f4f4f5;padding:2px 6px;border-radius:4px}pre{padding:12px;overflow:auto}h1{margin-top:0}</style>
</head><body>
<h1>Lumen — web bundle not embedded</h1>
<p>The hub binary was built without the web UI baked in (dist/ only had the placeholder .gitkeep). Two options:</p>
<h3>Build for production</h3>
<pre>pnpm install
make build         # builds web, embeds, builds hub + agent</pre>
<h3>Or run the dev server</h3>
<pre>pnpm --filter web dev    # http://localhost:5173</pre>
<p>The Vite dev server proxies <code>/api/*</code> and <code>/healthz</code> back to this hub.</p>
</body></html>`))
}
