package server

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// distFS holds the built Svelte SPA. The real frontend lands in a later phase;
// for now dist/ contains only a placeholder index.html so this compiles.
//
//go:embed all:dist
var distFS embed.FS

// spaHandler serves the embedded SPA with an HTML5 fallback: a request for an
// existing static asset is served directly; any other path returns index.html
// so the client-side router can take over. /api paths never reach here — they're
// matched by the router first.
func spaHandler() http.HandlerFunc {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic("server: embedded dist subtree missing: " + err.Error())
	}
	fileServer := http.FileServer(http.FS(sub))

	return func(w http.ResponseWriter, r *http.Request) {
		upath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if upath == "" {
			upath = "index.html"
		}
		if _, err := fs.Stat(sub, upath); err != nil {
			// Unknown path: hand the SPA its entrypoint for client routing.
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			fileServer.ServeHTTP(w, r2)
			return
		}
		fileServer.ServeHTTP(w, r)
	}
}
