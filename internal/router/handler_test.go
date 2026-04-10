package router

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vibeserve/vibeserve/internal/engine"
	"github.com/vibeserve/vibeserve/internal/manifest"
	"github.com/vibeserve/vibeserve/internal/runtime"
	"github.com/vibeserve/vibeserve/internal/store"
)

// setupHandler creates an in-memory store with an "items" table, seeds 1 item,
// registers 3 routes, and returns an http.Handler ready for testing.
func setupHandler(t *testing.T, cors bool) http.Handler {
	t.Helper()

	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	schema := manifest.Schema{
		Table: "items",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "title", Type: "TEXT"},
		},
	}
	if err := s.ApplySchemas([]manifest.Schema{schema}); err != nil {
		t.Fatalf("apply schemas: %v", err)
	}
	if err := s.Seed("items", []map[string]any{
		{"title": "First Item"},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	scripts := map[string]string{
		"list_items": `
result := db.query("SELECT * FROM items", [])
response.json(result)
`,
		"create_item": `
body := request.body()
row := db.insert("items", body)
response.json(row, 201)
`,
		"get_item": `
id := request.param("id")
row := db.query_one("SELECT * FROM items WHERE id = ?", [id])
if is_undefined(row) {
  response.fail(404, "not found")
} else {
  response.json(row)
}
`,
	}

	tr := NewTrie()
	tr.Insert("GET", "/items", "list_items")
	tr.Insert("POST", "/items", "create_item")
	tr.Insert("GET", "/items/:id", "get_item")

	rt := runtime.New(s, engine.NewBus())

	return NewHandler(tr, scripts, rt, cors)
}

func TestHandlerListItems(t *testing.T) {
	handler := setupHandler(t, false)

	req := httptest.NewRequest(http.MethodGet, "/items", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	var body []any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body) != 1 {
		t.Errorf("expected 1 item, got %d", len(body))
	}
}

func TestHandlerCreateItem(t *testing.T) {
	handler := setupHandler(t, false)

	payload := map[string]any{"title": "New Item"}
	b, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/items", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected 201, got %d", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["title"] != "New Item" {
		t.Errorf("expected title=New Item, got %v", body["title"])
	}
}

func TestHandlerGetItemNotFound(t *testing.T) {
	handler := setupHandler(t, false)

	req := httptest.NewRequest(http.MethodGet, "/items/999", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestHandlerRouteNotFound(t *testing.T) {
	handler := setupHandler(t, false)

	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if _, ok := body["error"]; !ok {
		t.Error("expected error field in 404 response")
	}
}

func TestHandlerCORSPreflight(t *testing.T) {
	handler := setupHandler(t, true)

	req := httptest.NewRequest(http.MethodOptions, "/items", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204 for OPTIONS, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Error("expected Access-Control-Allow-Origin: * header")
	}
	if resp.Header.Get("Access-Control-Allow-Methods") == "" {
		t.Error("expected Access-Control-Allow-Methods header")
	}
	if resp.Header.Get("Access-Control-Allow-Headers") == "" {
		t.Error("expected Access-Control-Allow-Headers header")
	}
}

func TestHandlerCORSHeaders(t *testing.T) {
	handler := setupHandler(t, true)

	req := httptest.NewRequest(http.MethodGet, "/items", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Error("expected Access-Control-Allow-Origin: * on regular response")
	}
}

// --- Proxy mode tests ---

func TestHandlerProxyModeUnmatchedRoute(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	scripts := map[string]string{
		"list_items": `
result := db.query("SELECT * FROM items", [])
response.json(result)
`,
	}

	tr := NewTrie()
	rt := runtime.New(s, engine.NewBus())

	// Create a mock proxy handler that registers a route and returns the retry signal.
	called := false
	proxyFn := func(ctx context.Context, method, path string, body map[string]any, queryParams map[string]string, headers map[string]string) (int, map[string]any, map[string]string, error) {
		called = true
		if method != "GET" || path != "/users" {
			t.Errorf("unexpected proxy call: %s %s", method, path)
		}

		// Simulate registering the route (as the real ProxyEngine would do)
		scripts["list_users"] = `result := db.query("SELECT * FROM users", [])
response.json(result)`
		tr.Insert("GET", "/users", "list_users")

		// Return retry signal
		return 0, map[string]any{"__vibeserve_retry__": true}, map[string]string{"X-VibeServe-Generated": "true"}, nil
	}

	handler := NewProxyHandler(tr, scripts, rt, false, proxyFn)

	// First request to /users should trigger proxy
	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if !called {
		t.Error("expected proxy handler to be called")
	}

	// Since there's no users table, the script will error, but the proxy mechanism worked
	// The important thing is the proxy was invoked, not a 404

	// Second request to /users should NOT trigger proxy (route is now registered)
	called = false
	req2 := httptest.NewRequest(http.MethodGet, "/users", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	if called {
		t.Error("expected proxy handler NOT to be called on second request")
	}
}

func TestHandlerProxyModeSystemPathsSkipped(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	tr := NewTrie()
	scripts := map[string]string{}
	rt := runtime.New(s, engine.NewBus())

	proxyCalled := false
	proxyFn := func(ctx context.Context, method, path string, body map[string]any, queryParams map[string]string, headers map[string]string) (int, map[string]any, map[string]string, error) {
		proxyCalled = true
		return 200, map[string]any{"ok": true}, nil, nil
	}

	handler := NewProxyHandler(tr, scripts, rt, false, proxyFn)

	systemPaths := []string{"/_console", "/_api/something", "/_blueprint", "/_swagger", "/_ws"}
	for _, path := range systemPaths {
		proxyCalled = false
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if proxyCalled {
			t.Errorf("expected proxy NOT to be called for system path %s", path)
		}
	}
}

func TestIsSystemPath(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"/_console", true},
		{"/_console/foo", true},
		{"/_api/something", true},
		{"/_blueprint", true},
		{"/_swagger", true},
		{"/_ws", true},
		{"/users", false},
		{"/items/123", false},
		{"/api/v1/users", false},
	}
	for _, tt := range tests {
		result := isSystemPath(tt.path)
		if result != tt.expected {
			t.Errorf("isSystemPath(%q) = %v, want %v", tt.path, result, tt.expected)
		}
	}
}

func TestHandlerNoProxyReturns404(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	tr := NewTrie()
	scripts := map[string]string{}
	rt := runtime.New(s, engine.NewBus())

	// No proxy handler — should return 404
	handler := NewProxyHandler(tr, scripts, rt, false, nil)

	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 without proxy, got %d", resp.StatusCode)
	}
}
