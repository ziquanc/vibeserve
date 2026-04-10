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
	handler := router.NewProxyHandler(trie, scripts, rt, true, proxyEng.HandleUnknownRequest)

	// Step 1: First request to GET /products should trigger auto-generation
	req := httptest.NewRequest(http.MethodGet, "/products", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 after auto-generation, got %d: %s", resp.StatusCode, w.Body.String())
	}

	if provider.calls != 1 {
		t.Errorf("expected 1 LLM call, got %d", provider.calls)
	}

	// Verify the X-VibeServe-Generated header
	if resp.Header.Get("X-VibeServe-Generated") != "true" {
		t.Error("expected X-VibeServe-Generated: true header")
	}

	// Step 2: Second request to GET /products should NOT trigger LLM again
	provider.calls = 0
	req2 := httptest.NewRequest(http.MethodGet, "/products", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	resp2 := w2.Result()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("expected 200 on second request, got %d", resp2.StatusCode)
	}

	if provider.calls != 0 {
		t.Errorf("expected 0 LLM calls on cached route, got %d", provider.calls)
	}

	// Step 3: POST /products should also work (route was registered by generation)
	body := map[string]any{"name": "Widget", "price": 9.99}
	bodyBytes, _ := json.Marshal(body)
	req3 := httptest.NewRequest(http.MethodPost, "/products", bytes.NewReader(bodyBytes))
	req3.Header.Set("Content-Type", "application/json")
	w3 := httptest.NewRecorder()
	handler.ServeHTTP(w3, req3)

	resp3 := w3.Result()
	if resp3.StatusCode != http.StatusCreated {
		t.Errorf("expected 201 for POST, got %d: %s", resp3.StatusCode, w3.Body.String())
	}

	// Verify the product was actually created
	var created map[string]any
	if err := json.NewDecoder(resp3.Body).Decode(&created); err != nil {
		t.Fatalf("decode created product: %v", err)
	}
	if created["name"] != "Widget" {
		t.Errorf("expected name=Widget, got %v", created["name"])
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

	pe := engine.NewProxyEngine(eng)

	// Test POST with body
	prompt := pe.BuildPromptFromRequest("POST", "/users", map[string]any{
		"name":  "John",
		"email": "john@test.com",
		"age":   float64(25),
	}, nil)

	if prompt == "" {
		t.Error("expected non-empty prompt")
	}

	// Test GET with query params
	prompt2 := pe.BuildPromptFromRequest("GET", "/products", nil, map[string]string{"category": "electronics"})
	if prompt2 == "" {
		t.Error("expected non-empty prompt for GET with query params")
	}

	// Test DELETE
	prompt3 := pe.BuildPromptFromRequest("DELETE", "/users/:id", nil, nil)
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
