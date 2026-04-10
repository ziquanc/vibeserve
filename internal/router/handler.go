package router

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/vibeserve/vibeserve/internal/runtime"
)

// ProxyHandler is called when a request hits an unmatched route in proxy mode.
// It receives the request details and returns (statusCode, body, headers, error).
type ProxyHandler func(ctx context.Context, method, path string, body map[string]any, queryParams map[string]string, headers map[string]string) (int, map[string]any, map[string]string, error)

// NewHandler creates an http.Handler that routes requests through the trie,
// executes the matched Tengo script via the runtime, and writes a JSON response.
// If cors is true, CORS headers are added to all responses and OPTIONS
// preflight requests return 204 No Content.
func NewHandler(trie *Trie, scripts map[string]string, rt *runtime.Runtime, cors bool) http.Handler {
	return NewProxyHandler(trie, scripts, rt, cors, nil)
}

// NewProxyHandler creates an http.Handler like NewHandler, but when proxyFn is
// non-nil and no route matches, it calls proxyFn instead of returning 404.
// After proxyFn generates the route, the handler retries the trie lookup and
// executes the newly registered script.
func NewProxyHandler(trie *Trie, scripts map[string]string, rt *runtime.Runtime, cors bool, proxyFn ProxyHandler) http.Handler {
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
		if !found && proxyFn != nil && !isSystemPath(r.URL.Path) {
			// Proxy mode: intercept unmatched requests.
			handleProxyRequest(w, r, trie, scripts, rt, proxyFn)
			return
		}

		if !found {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "route not found"})
			return
		}

		executeAndRespond(w, r, scripts, rt, script, pathParams)
	})
}

// isSystemPath returns true for internal VibeServe paths that should not trigger proxy generation.
func isSystemPath(path string) bool {
	return strings.HasPrefix(path, "/_console") ||
		strings.HasPrefix(path, "/_api") ||
		strings.HasPrefix(path, "/_blueprint") ||
		strings.HasPrefix(path, "/_swagger") ||
		strings.HasPrefix(path, "/_ws")
}

// handleProxyRequest processes a request through the proxy engine.
func handleProxyRequest(w http.ResponseWriter, r *http.Request, trie *Trie, scripts map[string]string, rt *runtime.Runtime, proxyFn ProxyHandler) {
	// Parse request body for methods that carry a body.
	var body map[string]any
	var rawBody []byte
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB limit
		var err error
		rawBody, err = io.ReadAll(r.Body)
		if err == nil && len(rawBody) > 0 {
			_ = json.Unmarshal(rawBody, &body)
		}
		// Restore body so retry can read it again.
		r.Body = io.NopCloser(bytes.NewReader(rawBody))
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

	// Call the proxy handler to generate the route.
	status, respBody, respHeaders, err := proxyFn(r.Context(), r.Method, r.URL.Path, body, queryParams, headers)
	if err != nil {
		log.Printf("[proxy] error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "proxy generation failed", "detail": err.Error()})
		return
	}

	// Check if the proxy wants us to retry with the newly registered route.
	if retry, ok := respBody["__vibeserve_retry__"]; ok && retry == true {
		// Retry the trie lookup — the route should now be registered.
		script, pathParams, found := trie.Search(r.Method, r.URL.Path)
		if !found {
			log.Printf("[proxy] route still not found after generation: %s %s", r.Method, r.URL.Path)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "generated route not found after registration"})
			return
		}

		// Apply proxy headers.
		for k, v := range respHeaders {
			w.Header().Set(k, v)
		}

		executeAndRespond(w, r, scripts, rt, script, pathParams)
		return
	}

	// Apply any custom headers from the proxy.
	for k, v := range respHeaders {
		w.Header().Set(k, v)
	}

	writeJSON(w, status, respBody)
}

// executeAndRespond resolves a script, executes it, and writes the response.
func executeAndRespond(w http.ResponseWriter, r *http.Request, scripts map[string]string, rt *runtime.Runtime, scriptName string, pathParams map[string]string) {
	// Resolve script code.
	code, ok := scripts[scriptName]
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "script not found: " + scriptName})
		return
	}

	// Parse request body for methods that carry a body.
	var body map[string]any
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB limit
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
}

// writeJSON serialises v as JSON and writes it to w with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
