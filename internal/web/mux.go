package web

import (
	"io/fs"
	"net/http"
)

// NewConsoleMux builds the combined HTTP handler that routes:
//   - /_console/  → embedded static files (index.html, etc.)
//   - /_console   → redirect to /_console/
//   - /_api/      → Console REST API handlers
//   - /_ws        → WebSocket hub
//   - /           → existing API handler (trie-based)
func NewConsoleMux(apiHandler http.Handler, console *Console, wsHub *WSHub) http.Handler {
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

	// Fall through to the existing trie-based API handler
	mux.Handle("/", apiHandler)

	return mux
}
