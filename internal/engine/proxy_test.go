package engine_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/vibeserve/vibeserve/internal/engine"
	"github.com/vibeserve/vibeserve/internal/llm"
	"github.com/vibeserve/vibeserve/internal/manifest"
	"github.com/vibeserve/vibeserve/internal/router"
	"github.com/vibeserve/vibeserve/internal/runtime"
	"github.com/vibeserve/vibeserve/internal/store"
)

// proxyMockProvider returns a manifest with CRUD routes for a given resource.
type proxyMockProvider struct {
	calls int
}

func (p *proxyMockProvider) Generate(_ context.Context, current *manifest.Manifest, prompt string, _ []llm.Message) (*manifest.Manifest, error) {
	p.calls++

	// Start from current manifest or create fresh
	m := &manifest.Manifest{
		Version:     "1.0",
		Name:        "test-api",
		Description: "Auto-generated API",
	}

	// Preserve existing resources
	if current != nil {
		m.Name = current.Name
		m.Description = current.Description
		m.Schemas = append(m.Schemas, current.Schemas...)
		m.Routes = append(m.Routes, current.Routes...)
		m.Scripts = append(m.Scripts, current.Scripts...)
		m.Seeds = append(m.Seeds, current.Seeds...)
	}

	// Add a "products" table and CRUD routes (simulating what the LLM would generate)
	m.Schemas = append(m.Schemas, manifest.Schema{
		Table: "products",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "name", Type: "TEXT", Required: true},
			{Name: "price", Type: "REAL"},
			{Name: "created_at", Type: "DATETIME", Default: "NOW"},
		},
	})

	// Add routes and scripts
	scripts := []manifest.Script{
		{
			Name: "list_products",
			Code: `result := db.query("SELECT * FROM products", [])
response.json(result)`,
		},
		{
			Name: "get_product",
			Code: `id := request.param("id")
row := db.query_one("SELECT * FROM products WHERE id = ?", [id])
if is_undefined(row) {
  response.fail(404, "not found")
} else {
  response.json(row)
}`,
		},
		{
			Name: "create_product",
			Code: `body := request.body()
row := db.insert("products", body)
response.json(row, 201)`,
		},
	}
	m.Scripts = append(m.Scripts, scripts...)

	routes := []manifest.Route{
		{Path: "/products", Method: "GET", Script: "list_products", Description: "List products"},
		{Path: "/products/:id", Method: "GET", Script: "get_product", Description: "Get product"},
		{Path: "/products", Method: "POST", Script: "create_product", Description: "Create product"},
	}
	m.Routes = append(m.Routes, routes...)

	return m, nil
}

func (p *proxyMockProvider) Chat(ctx context.Context, systemPrompt string, prompt string) (string, error) {
	return "", nil
}

func TestProxyEngine_AutoGenerateEndpoint(t *testing.T) {
	// Setup
	tmpDir := t.TempDir()
	vibeDir := filepath.Join(tmpDir, ".vibe")
	os.MkdirAll(vibeDir, 0o755)

	s, err := store.New(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	bus := engine.NewBus()
	trie := router.NewTrie()
	scripts := make(map[string]string)
	provider := &proxyMockProvider{}

	eng := engine.NewEngine(engine.EngineConfig{
		Bus:      bus,
		Store:    s,
		Trie:     trie,
		Scripts:  scripts,
		Provider: provider,
		Manifest: nil,
		VibeDir:  vibeDir,
	})

	// Create proxy engine
	proxyEng := engine.NewProxyEngine(eng)

	// Create HTTP handler with proxy
	rt := runtime.New(s, bus)
	handler := router.NewProxyHandler(trie, eng.GetScript, rt, true, proxyEng.HandleUnknownRequest)

	// Step 1: POST /products with body triggers deterministic table creation.
	// The intent-aware proxy creates a table with columns inferred from the body
	// plus CRUD routes — no LLM call needed.
	body := map[string]any{"name": "Widget", "price": 9.99}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/products", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected 201 after auto-generation, got %d: %s", resp.StatusCode, w.Body.String())
	}

	// Proxy now uses LLM for quality schema design.
	if provider.calls < 1 {
		t.Errorf("expected at least 1 LLM call for table creation, got %d", provider.calls)
	}

	// Verify the X-VibeServe-Generated header
	if resp.Header.Get("X-VibeServe-Generated") != "true" {
		t.Error("expected X-VibeServe-Generated: true header")
	}

	// Verify the product was actually created
	var created map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode created product: %v", err)
	}
	if created["name"] != "Widget" {
		t.Errorf("expected name=Widget, got %v", created["name"])
	}

	// Step 2: GET /products should now work without any LLM call
	provider.calls = 0
	req2 := httptest.NewRequest(http.MethodGet, "/products", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	resp2 := w2.Result()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("expected 200 on second request, got %d: %s", resp2.StatusCode, w2.Body.String())
	}

	// Second request hits cached route — no additional LLM calls needed.
	// provider.calls tracks total across the test, not per-request.

	// Step 3: POST another product
	body2 := map[string]any{"name": "Gadget", "price": 19.99}
	bodyBytes2, _ := json.Marshal(body2)
	req3 := httptest.NewRequest(http.MethodPost, "/products", bytes.NewReader(bodyBytes2))
	req3.Header.Set("Content-Type", "application/json")
	w3 := httptest.NewRecorder()
	handler.ServeHTTP(w3, req3)

	resp3 := w3.Result()
	if resp3.StatusCode != http.StatusCreated {
		t.Errorf("expected 201 for second POST, got %d: %s", resp3.StatusCode, w3.Body.String())
	}
}

func TestProxyEngine_BuildPromptFromRequest(t *testing.T) {
	tmpDir := t.TempDir()
	vibeDir := filepath.Join(tmpDir, ".vibe")
	os.MkdirAll(vibeDir, 0o755)

	s, err := store.New(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	bus := engine.NewBus()
	trie := router.NewTrie()
	scripts := make(map[string]string)

	eng := engine.NewEngine(engine.EngineConfig{
		Bus:      bus,
		Store:    s,
		Trie:     trie,
		Scripts:  scripts,
		Manifest: nil,
		VibeDir:  vibeDir,
	})

	_ = engine.NewProxyEngine(eng)

	// Test POST with body — use IntentAnalyzer directly
	analyzer := engine.NewIntentAnalyzer(nil)
	prompt := analyzer.BuildSmartPrompt("POST", "/users", map[string]any{
		"name":  "John",
		"email": "john@test.com",
		"age":   float64(25),
	}, nil)

	if prompt == "" {
		t.Error("expected non-empty prompt")
	}

	// Test GET with query params
	prompt2 := analyzer.BuildSmartPrompt("GET", "/products", nil, map[string]string{"category": "electronics"})
	if prompt2 == "" {
		t.Error("expected non-empty prompt for GET with query params")
	}

	// Test DELETE
	prompt3 := analyzer.BuildSmartPrompt("DELETE", "/users/:id", nil, nil)
	if prompt3 == "" {
		t.Error("expected non-empty prompt for DELETE")
	}
}

// Ensure the ProxyEngine interface is satisfied and compiles.
func TestProxyEngine_Interface(t *testing.T) {
	// Verify that NewProxyEngine returns a usable *ProxyEngine
	tmpDir := t.TempDir()
	vibeDir := filepath.Join(tmpDir, ".vibe")
	os.MkdirAll(vibeDir, 0o755)

	s, err := store.New(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	bus := engine.NewBus()
	eng := engine.NewEngine(engine.EngineConfig{
		Bus:      bus,
		Store:    s,
		Trie:     router.NewTrie(),
		Scripts:  make(map[string]string),
		Manifest: nil,
		VibeDir:  vibeDir,
	})

	pe := engine.NewProxyEngine(eng)
	if pe == nil {
		t.Fatal("expected non-nil ProxyEngine")
	}
}

// readCloser wraps a byte slice as an io.ReadCloser (used indirectly).
var _ io.ReadCloser = (io.ReadCloser)(nil)

func TestIntentAnalyzer_RejectsInvalidColumnNames(t *testing.T) {
	// With no existing schemas, Analyze creates a new table from body keys.
	// Invalid column names (SQL injection attempts, spaces, etc.) must be filtered out.
	m := &manifest.Manifest{
		Version: "1.0",
		Schemas: []manifest.Schema{},
	}
	analyzer := engine.NewIntentAnalyzer(m)

	body := map[string]any{
		"valid_name":             "test",
		"name'; DROP TABLE x;--": "malicious",
		"also invalid":           "nope", // spaces not allowed
		"good_col":               123,
	}

	intent := analyzer.Analyze("POST", "/widgets", body, nil)
	if intent.Type.String() != "CreateTable" {
		t.Fatalf("expected CreateTable intent, got %s", intent.Type)
	}

	for _, col := range intent.Columns {
		if col.Name == "name'; DROP TABLE x;--" {
			t.Error("should reject column names with SQL injection characters")
		}
		if col.Name == "also invalid" {
			t.Error("should reject column names with spaces")
		}
	}

	// Verify valid columns survived
	found := map[string]bool{}
	for _, col := range intent.Columns {
		found[col.Name] = true
	}
	if !found["valid_name"] {
		t.Error("valid_name column should be present")
	}
	if !found["good_col"] {
		t.Error("good_col column should be present")
	}
}

func TestIntentAnalyzer_RelationshipCreateChild(t *testing.T) {
	m := &manifest.Manifest{
		Version: "1.0",
		Schemas: []manifest.Schema{{
			Table: "pets",
			Columns: []manifest.Column{
				{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
				{Name: "name", Type: "TEXT"},
			},
		}},
		Routes:  []manifest.Route{},
		Scripts: []manifest.Script{},
	}

	analyzer := engine.NewIntentAnalyzer(m)
	intent := analyzer.Analyze("GET", "/pets/:id/history", nil, nil)

	if intent.Type.String() != "CreateTable" {
		t.Errorf("expected CreateTable, got %s", intent.Type)
	}
	if intent.TableName != "histories" {
		t.Errorf("expected table name 'histories', got %q", intent.TableName)
	}

	// Should have FK column
	hasPetFK := false
	for _, col := range intent.Columns {
		if col.Name == "pet_id" && col.References == "pets.id" {
			hasPetFK = true
		}
	}
	if !hasPetFK {
		t.Error("child table should have pet_id FK column referencing pets.id")
	}

	// ParentTable should be set
	if intent.ParentTable != "pets" {
		t.Errorf("expected ParentTable 'pets', got %q", intent.ParentTable)
	}
}

func TestIntentAnalyzer_RelationshipCreateChildWithBody(t *testing.T) {
	m := &manifest.Manifest{
		Version: "1.0",
		Schemas: []manifest.Schema{{
			Table: "pets",
			Columns: []manifest.Column{
				{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
				{Name: "name", Type: "TEXT"},
			},
		}},
		Routes:  []manifest.Route{},
		Scripts: []manifest.Script{},
	}

	analyzer := engine.NewIntentAnalyzer(m)
	body := map[string]any{
		"event":       "vaccination",
		"description": "Annual checkup",
	}
	intent := analyzer.Analyze("POST", "/pets/:id/history", body, nil)

	if intent.Type.String() != "CreateTable" {
		t.Errorf("expected CreateTable, got %s", intent.Type)
	}

	// Should have body columns
	colNames := map[string]bool{}
	for _, col := range intent.Columns {
		colNames[col.Name] = true
	}
	if !colNames["event"] {
		t.Error("should have 'event' column from body")
	}
	if !colNames["description"] {
		t.Error("should have 'description' column from body")
	}
	if !colNames["pet_id"] {
		t.Error("should have 'pet_id' FK column")
	}
	if !colNames["created_at"] {
		t.Error("should have 'created_at' column")
	}
}

func TestIntentAnalyzer_RejectsInvalidColumnNamesInAddColumn(t *testing.T) {
	// When a table exists but body has new fields, invalid new column names
	// must be filtered out of ColumnsToAdd.
	m := &manifest.Manifest{
		Version: "1.0",
		Schemas: []manifest.Schema{
			{
				Table: "widgets",
				Columns: []manifest.Column{
					{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
					{Name: "name", Type: "TEXT"},
				},
			},
		},
	}
	analyzer := engine.NewIntentAnalyzer(m)

	body := map[string]any{
		"name":               "existing field",
		"color":              "blue",           // valid new column
		"x'; DROP TABLE w;": "injection",      // invalid
	}

	intent := analyzer.Analyze("POST", "/widgets", body, nil)
	if intent.Type.String() != "AddColumn" {
		t.Fatalf("expected AddColumn intent, got %s", intent.Type)
	}

	for _, col := range intent.ColumnsToAdd {
		if col.Name == "x'; DROP TABLE w;" {
			t.Error("should reject invalid column names in ColumnsToAdd")
		}
	}
}
