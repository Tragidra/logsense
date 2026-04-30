// Package web embeds the built Vue Dashboard into the binary.
//
// The Vite output at ./dist/ is bundled via go:embed and exposed through Handler(), which serves static assets
// and falls back to index.html for unknown paths so SPA client-side routing keeps working on direct loads.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// distRoot returns dist/ as a sub-filesystem so paths like "/assets/foo.js" resolve to dist/assets/foo.js.
func distRoot() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}

// Handler serves the embedded Dashboard. Extension-less paths that don't match a file fall back to index.html
// so the SPA router can handle them.
func Handler() http.Handler {
	root := distRoot()
	fileServer := http.FileServer(http.FS(root))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqPath := strings.TrimPrefix(r.URL.Path, "/")
		if reqPath == "" {
			reqPath = "index.html"
		}
		if _, err := fs.Stat(root, reqPath); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}
		if path.Ext(reqPath) != "" {
			http.NotFound(w, r)
			return
		}
		// SPA navigation — let the client router resolve it.
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		fileServer.ServeHTTP(w, r2)
	})
}
