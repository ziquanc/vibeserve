package router

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/vibeserve/vibeserve/internal/runtime"
)

// NewHandler creates an http.Handler that routes requests through the trie,
// executes the matched Tengo script via the runtime, and writes a JSON response.
// If cors is true, CORS headers are added to all responses and OPTIONS
// preflight requests return 204 No Content.
func NewHandler(trie *Trie, scripts map[string]string, rt *runtime.Runtime, cors bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers on every response when enabled.
		if cors {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		}

		// Handle CORS preflight.
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// Look up route.
		script, pathParams, found := trie.Search(r.Method, r.URL.Path)
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "route not found"})
			return
		}

		// Resolve script code.
		code, ok := scripts[script]
		if !ok {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "script not found: " + script})
			return
		}

		// Parse request body for methods that carry a body.
		var body map[string]any
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch:
			raw, err := io.ReadAll(r.Body)
			if err == nil && len(raw) > 0 {
				_ = json.Unmarshal(raw, &body)
			}
		}

		// Parse query params (single value per key).
		queryParams := make(map[string]string)
		for k, vs := range r.URL.Query() {
			if len(vs) > 0 {
				queryParams[k] = vs[0]
			}
		}

		// Parse headers with lowercase keys.
		headers := make(map[string]string)
		for k, vs := range r.Header {
			if len(vs) > 0 {
				headers[strings.ToLower(k)] = vs[0]
			}
		}

		// Build request context.
		rc := &runtime.RequestContext{
			PathParams:  pathParams,
			QueryParams: queryParams,
			Body:        body,
			Headers:     headers,
			Method:      r.Method,
		}

		// Execute the script.
		statusCode, respBody, respHeaders, err := rt.Execute(code, rc)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}

		// Apply any custom headers from the script.
		for k, v := range respHeaders {
			w.Header().Set(k, v)
		}

		writeJSON(w, statusCode, respBody)
	})
}

// writeJSON serialises v as JSON and writes it to w with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
