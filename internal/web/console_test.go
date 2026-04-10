package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vibeserve/vibeserve/internal/engine"
	"github.com/vibeserve/vibeserve/internal/manifest"
	"github.com/vibeserve/vibeserve/internal/store"
)

// testManifest returns a minimal manifest for tests.
func testManifest() *manifest.Manifest {
	return &manifest.Manifest{
		Version:     "1.0",
		Name:        "test-api",
		Description: "Test API",
		Schemas: []manifest.Schema{
			{
				Table: "users",
				Columns: []manifest.Column{
					{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
					{Name: "name", Type: "TEXT", Required: true},
					{Name: "email", Type: "TEXT", Required: true, Unique: true},
				},
			},
		},
		Routes: []manifest.Route{
			{Path: "/users", Method: "GET", Script: "list_users.tengo", Description: "List all users"},
			{Path: "/users", Method: "POST", Script: "create_user.tengo", Description: "Create a user"},
			{Path: "/users/:id", Method: "GET", Script: "get_user.tengo", Description: "Get user by ID"},
		},
		Scripts: []manifest.Script{
			{Name: "list_users.tengo", Code: "result := db.query(\"SELECT * FROM users\", [])\nresponse.json(result)"},
			{Name: "create_user.tengo", Code: "body := request.body()\nrow := db.insert(\"users\", body)\nresponse.json(row, 201)"},
			{Name: "get_user.tengo", Code: "id := request.param(\"id\")\nrow := db.query_one(\"SELECT * FROM users WHERE id = ?\", [id])\nresponse.json(row)"},
		},
		Seeds: []manifest.Seed{
			{Table: "users", Rows: []map[string]any{{"name": "Alice", "email": "alice@example.com"}}},
		},
	}
}

// setupTestConsole creates an in-memory store + engine for testing.
func setupTestConsole(t *testing.T) (*Console, *http.ServeMux) {
	t.Helper()

	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	m := testManifest()
	if err := s.ApplySchemas(m.Schemas); err != nil {
		t.Fatalf("apply schemas: %v", err)
	}
	if err := s.Seed("users", m.Seeds[0].Rows); err != nil {
		t.Fatalf("seed: %v", err)
	}

	bus := engine.NewBus()
	eng := engine.NewEngine(engine.EngineConfig{
		Bus:      bus,
		Store:    s,
		Manifest: m,
		VibeDir:  "",
	})

	console := NewConsole(eng, s)
	mux := http.NewServeMux()
	console.RegisterRoutes(mux)

	return console, mux
}

func TestHandleManifest(t *testing.T) {
	_, mux := setupTestConsole(t)

	req := httptest.NewRequest(http.MethodGet, "/_api/manifest", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var m manifest.Manifest
	if err := json.NewDecoder(w.Body).Decode(&m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if m.Name != "test-api" {
		t.Errorf("expected name=test-api, got %q", m.Name)
	}
	if len(m.Routes) != 3 {
		t.Errorf("expected 3 routes, got %d", len(m.Routes))
	}
}

func TestHandleRoutes(t *testing.T) {
	_, mux := setupTestConsole(t)

	req := httptest.NewRequest(http.MethodGet, "/_api/routes", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var routes []map[string]string
	if err := json.NewDecoder(w.Body).Decode(&routes); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(routes) != 3 {
		t.Errorf("expected 3 routes, got %d", len(routes))
	}
}

func TestHandleTables(t *testing.T) {
	_, mux := setupTestConsole(t)

	req := httptest.NewRequest(http.MethodGet, "/_api/tables", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var tables []store.TableInfo
	if err := json.NewDecoder(w.Body).Decode(&tables); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	if tables[0].Name != "users" {
		t.Errorf("expected table name=users, got %q", tables[0].Name)
	}
	if tables[0].RowCount != 1 {
		t.Errorf("expected 1 row, got %d", tables[0].RowCount)
	}
	if len(tables[0].Columns) != 6 {
		t.Errorf("expected 6 columns (3 user + 3 timestamps), got %d", len(tables[0].Columns))
	}
}

func TestHandleTableRows(t *testing.T) {
	_, mux := setupTestConsole(t)

	req := httptest.NewRequest(http.MethodGet, "/_api/tables/users/rows", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var result struct {
		Rows   []map[string]any `json:"rows"`
		Total  int              `json:"total"`
		Limit  int              `json:"limit"`
		Offset int              `json:"offset"`
	}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("expected total=1, got %d", result.Total)
	}
	if len(result.Rows) != 1 {
		t.Errorf("expected 1 row, got %d", len(result.Rows))
	}
}

func TestHandleTableRowsPagination(t *testing.T) {
	_, mux := setupTestConsole(t)

	req := httptest.NewRequest(http.MethodGet, "/_api/tables/users/rows?limit=10&offset=0", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var result struct {
		Limit  int `json:"limit"`
		Offset int `json:"offset"`
	}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Limit != 10 {
		t.Errorf("expected limit=10, got %d", result.Limit)
	}
	if result.Offset != 0 {
		t.Errorf("expected offset=0, got %d", result.Offset)
	}
}

func TestHandleTableRowsInvalidName(t *testing.T) {
	_, mux := setupTestConsole(t)

	req := httptest.NewRequest(http.MethodGet, "/_api/tables/users%3BDROP%20TABLE/rows", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleScripts(t *testing.T) {
	_, mux := setupTestConsole(t)

	req := httptest.NewRequest(http.MethodGet, "/_api/scripts", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var scripts []map[string]string
	if err := json.NewDecoder(w.Body).Decode(&scripts); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(scripts) != 3 {
		t.Errorf("expected 3 scripts, got %d", len(scripts))
	}
}

func TestHandleScriptByName(t *testing.T) {
	_, mux := setupTestConsole(t)

	req := httptest.NewRequest(http.MethodGet, "/_api/scripts/list_users.tengo", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var script map[string]string
	if err := json.NewDecoder(w.Body).Decode(&script); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if script["name"] != "list_users.tengo" {
		t.Errorf("expected name=list_users.tengo, got %q", script["name"])
	}
}

func TestHandleScriptNotFound(t *testing.T) {
	_, mux := setupTestConsole(t)

	req := httptest.NewRequest(http.MethodGet, "/_api/scripts/nonexistent", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
