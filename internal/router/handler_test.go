package router

import (
	"bytes"
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
