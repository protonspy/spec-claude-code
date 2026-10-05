// Package view serves the workspace's knowledge base, plans and specs as a local,
// read-only web page — `scc view`.
//
// Everything a person reads arrives through three JSON routes, and every route
// reads the disk on each request through the same parsers the rest of scc uses,
// so the page cannot disagree with `scc map` about what a file says. The frontend
// is built from web/ into dist/ and embedded, which keeps `go build` free of node.
//
// The boundary is loopback: the listener binds 127.0.0.1, and the middleware
// refuses a Host header that does not name loopback, which is what stops a page on
// the open web from reaching this server through DNS rebinding.
package view

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"path"
	"strings"
	"time"
)

//go:embed all:dist
var dist embed.FS

// csp keeps scripts, styles and connections on this origin. 'unsafe-inline' is for
// styles only — React sets style attributes — and never for scripts, so injected
// Markdown that slipped past the renderer still could not run or phone home.
const csp = "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; " +
	"object-src 'none'; base-uri 'none'; frame-ancestors 'none'"

// Handler is the whole server: the three API routes, then the embedded app.
func Handler(root string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/tree", func(w http.ResponseWriter, r *http.Request) {
		t, err := BuildTree(root)
		if err != nil {
			http.Error(w, "could not read the workspace", http.StatusInternalServerError)
			return
		}
		writeJSON(w, t)
	})
	mux.HandleFunc("/api/page", func(w http.ResponseWriter, r *http.Request) {
		p, ok := LoadPage(root, r.URL.Query().Get("path"))
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, p)
	})
	mux.HandleFunc("/api/source", func(w http.ResponseWriter, r *http.Request) {
		s, ok := LoadSource(root, r.URL.Query().Get("path"))
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, s)
	})
	mux.HandleFunc("/api/", http.NotFound)
	mux.Handle("/", app())
	return guard(mux)
}

// guard is the middleware every request passes before any route sees it.
func guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		if !loopbackHost(r.Host) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			h.Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// loopbackHost reports whether a Host header names this machine, with or without
// a port. A name that merely resolves to 127.0.0.1 is refused on purpose: that is
// exactly what a rebinding attacker's domain does.
func loopbackHost(host string) bool {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	switch strings.ToLower(host) {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}

// app serves the embedded frontend. A missing path with no extension gets
// index.html, so a reload on a client-side route still lands on the app; a missing
// file — a stale tab asking for an old bundle — gets a real 404, not HTML.
func app() http.Handler {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // the embed directive guarantees dist/ exists at build time
	}
	files := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name != "" {
			if st, err := fs.Stat(sub, name); err != nil || st.IsDir() {
				if path.Ext(name) != "" {
					http.NotFound(w, r)
					return
				}
				r = r.Clone(r.Context())
				r.URL.Path = "/"
			}
		}
		files.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}

// Serve runs the server on ln until ctx is cancelled, then shuts it down. A
// cancelled context is the normal way out and returns nil.
func Serve(ctx context.Context, ln net.Listener, root string) error {
	srv := &http.Server{Handler: Handler(root), ReadHeaderTimeout: 10 * time.Second}
	done := make(chan error, 1)
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		done <- srv.Shutdown(sctx)
	}()
	if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return <-done
}
