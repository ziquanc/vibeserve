package web

import (
	"encoding/json"
	"io/fs"
	"net/http"

	"github.com/vibeserve/vibeserve/internal/cloud"
	"github.com/vibeserve/vibeserve/internal/manifest"
)

// NewConsoleMux builds the combined HTTP handler that routes:
//   - /_console/                  → embedded static files (index.html, etc.)
//   - /_console                   → redirect to /_console/
//   - /_api/                      → Console REST API handlers
//   - /_api/blueprint             → Blueprint preview API
//   - /_api/openapi.yaml          → Generated OpenAPI spec
//   - /_blueprint                 → Blueprint preview HTML page
//   - /_swagger                   → Swagger UI page
//   - /_ws                        → WebSocket hub
//   - /.well-known/vibeserve.json → Public manifest descriptor for AI agents
//   - /                           → existing API handler (trie-based)
func NewConsoleMux(apiHandler http.Handler, console *Console, wsHub *WSHub, blueprint *BlueprintHandler, m *manifest.Manifest) http.Handler {
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
	mux.HandleFunc("GET /_blueprint/diagram", blueprint.HandleDiagram)
	mux.HandleFunc("GET /_swagger", serveStaticPage("swagger.html"))

	// .well-known descriptor — for AI agents and directory crawlers
	mux.HandleFunc("GET /.well-known/vibeserve.json", func(w http.ResponseWriter, r *http.Request) {
		if m == nil {
			http.Error(w, "no manifest loaded", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=60")
		_ = json.NewEncoder(w).Encode(cloud.GenerateWellKnown(m))
	})

	// Fall through to the existing trie-based API handler
	mux.Handle("/", apiHandler)

	return mux
}
