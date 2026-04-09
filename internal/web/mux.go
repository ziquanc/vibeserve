package web

import (
	"io/fs"
	"net/http"
)

// NewConsoleMux builds the combined HTTP handler that routes:
//   - /_console/     → embedded static files (index.html, etc.)
//   - /_console      → redirect to /_console/
//   - /_api/         → Console REST API handlers
//   - /_api/blueprint → Blueprint preview API
//   - /_blueprint    → Blueprint preview HTML page
//   - /_ws           → WebSocket hub
//   - /              → existing API handler (trie-based)
func NewConsoleMux(apiHandler http.Handler, console *Console, wsHub *WSHub, blueprint *BlueprintHandler) http.Handler {
	mux := http.NewServeMux()

	// Serve embedded static files under /_console/
	subFS, _ := fs.Sub(StaticFS, "static")
	mux.Handle("/_console/", http.StripPrefix("/_console/", http.FileServer(http.FS(subFS))))

	// Redirect /_console (no trailing slash) → /_console/
	mux.HandleFunc("/_console", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/_console/", http.StatusMovedPermanently)
	})

	// Register console REST API routes
	console.RegisterRoutes(mux)

	// WebSocket hub
	mux.HandleFunc("/_ws", wsHub.HandleWS)

	// Blueprint preview
	mux.HandleFunc("GET /_api/blueprint", blueprint.HandleBlueprint)
	mux.HandleFunc("GET /_api/openapi.yaml", blueprint.HandleOpenAPI)
	serveStaticPage := func(filename string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			data, err := StaticFS.ReadFile("static/" + filename)
			if err != nil {
				http.Error(w, "page not found", 404)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(data)
		}
	}
	mux.HandleFunc("GET /_blueprint", serveStaticPage("blueprint.html"))
	mux.HandleFunc("GET /_swagger", serveStaticPage("swagger.html"))

	// Fall through to the existing trie-based API handler
	mux.Handle("/", apiHandler)

	return mux
}
