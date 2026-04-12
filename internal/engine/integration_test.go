package engine_test

import (
	"context"
	"encoding/json"
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
	"github.com/vibeserve/vibeserve/internal/snapshot"
	"github.com/vibeserve/vibeserve/internal/store"
)

// mockLLMProvider returns a fixed manifest when Generate is called.
type mockLLMProvider struct {
	result *manifest.Manifest
}

func (m *mockLLMProvider) Generate(_ context.Context, _ *manifest.Manifest, _ string, _ []llm.Message) (*manifest.Manifest, error) {
	return m.result, nil
}

func (m *mockLLMProvider) Chat(ctx context.Context, systemPrompt string, prompt string) (string, error) {
	return "", nil
}

// TestIntegration_EngineEvolvesManifest verifies the full Phase 2 pipeline:
// load v1 → apply prompt (mock LLM returns v2) → diff/migrate → HTTP routes updated.
func TestIntegration_EngineEvolvesManifest(t *testing.T) {
	// ── 1. Load v1 manifest ───────────────────────────────────────────────────
	v1, err := manifest.LoadFromFile("../../testdata/car_rental_manifest.json")
	if err != nil {
		t.Fatalf("load v1 manifest: %v", err)
	}
	if err := manifest.Validate(v1); err != nil {
		t.Fatalf("validate v1: %v", err)
	}

	// ── 2. Create a real file-based store in a temp dir ───────────────────────
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	vibeDir := filepath.Join(tmpDir, ".vibe")
	if err := os.MkdirAll(vibeDir, 0o755); err != nil {
		t.Fatalf("mkdir vibeDir: %v", err)
	}

	s, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	// ── 3. Apply v1 schemas and seeds ─────────────────────────────────────────
	if err := s.ApplySchemas(v1.Schemas); err != nil {
		t.Fatalf("apply v1 schemas: %v", err)
	}
	for _, seed := range v1.Seeds {
		if err := s.Seed(seed.Table, seed.Rows); err != nil {
			t.Fatalf("seed v1 %s: %v", seed.Table, err)
		}
	}

	// ── 4. Build the shared trie and scripts map (v1 state) ───────────────────
	trie := router.NewTrie()
	scripts := make(map[string]string)

	for _, sc := range v1.Scripts {
		scripts[sc.Name] = sc.Code
	}
	for _, r := range v1.Routes {
		trie.Insert(r.Method, r.Path, r.Script)
	}

	// ── 5. Load v2 manifest (the mock LLM will return this) ───────────────────
	v2, err := manifest.LoadFromFile("../../testdata/car_rental_v2_manifest.json")
	if err != nil {
		t.Fatalf("load v2 manifest: %v", err)
	}
	if err := manifest.Validate(v2); err != nil {
		t.Fatalf("validate v2: %v", err)
	}

	provider := &mockLLMProvider{result: v2}

	// ── 6. Create Engine with all components wired ────────────────────────────
	bus := engine.NewBus()
	storeOpener := func(dsn string) (engine.SchemaStore, error) {
		return store.New(dsn)
	}

	eng := engine.NewEngine(engine.EngineConfig{
		Bus:         bus,
		Store:       s,
		Trie:        trie,
		Scripts:     scripts,
		Provider:    provider,
		Manifest:    v1,
		VibeDir:     vibeDir,
		StoreOpener: storeOpener,
	})

	// ── 7. Apply the prompt (mock LLM returns v2) — now proposes a blueprint ──
	blueprintResult, err := eng.Apply(context.Background(), "add a customers table with loyalty tiers")
	if err != nil {
		t.Fatalf("engine.Apply: %v", err)
	}
	if blueprintResult.Blueprint == nil {
		t.Fatal("expected Blueprint to be set after Apply()")
	}

	// ── 8. Verify proposed changes include ADD_TABLE, ADD_ROUTE, ADD_SCRIPT ───
	var foundAddTable, foundAddRoute, foundAddScript bool
	for _, c := range blueprintResult.Blueprint.Changes {
		switch c.Type {
		case manifest.ChangeAddTable:
			if c.Table == "customers" {
				foundAddTable = true
			}
		case manifest.ChangeAddRoute:
			if c.Route != nil && c.Route.Path == "/customers" && c.Route.Method == "GET" {
				foundAddRoute = true
			}
		case manifest.ChangeAddScript:
			if c.Script != nil && c.Script.Name == "list_customers" {
				foundAddScript = true
			}
		}
	}

	if !foundAddTable {
		t.Error("expected ADD_TABLE change for customers")
	}
	if !foundAddRoute {
		t.Error("expected ADD_ROUTE change for GET /customers")
	}
	if !foundAddScript {
		t.Error("expected ADD_SCRIPT change for list_customers")
	}

	// ── 8b. Approve the blueprint to actually apply the changes ───────────────
	applyResult, err := eng.ApproveBlueprint(context.Background())
	if err != nil {
		t.Fatalf("engine.ApproveBlueprint: %v", err)
	}

	// ── 8c. Apply pending seeds (now a separate step) ─────────────────────────
	if len(applyResult.PendingSeeds) > 0 {
		if err := eng.ApplySeeds(applyResult.PendingSeeds); err != nil {
			t.Fatalf("engine.ApplySeeds: %v", err)
		}
	}

	// ── 9. Build HTTP handler using the (now-mutated) trie and scripts ─────────
	rt := runtime.New(s, bus)
	handler := router.NewHandler(trie, router.MapScriptResolver(scripts), rt, false)

	// ── 10. Verify GET /customers returns the seeded customers ────────────────
	req := httptest.NewRequest(http.MethodGet, "/customers", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /customers: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var customers []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&customers); err != nil {
		t.Fatalf("decode /customers response: %v", err)
	}
	if len(customers) != 3 {
		t.Errorf("expected 3 customers, got %d", len(customers))
	}

	// Verify loyalty_tier field is present
	foundGold := false
	for _, c := range customers {
		if c["loyalty_tier"] == "gold" {
			foundGold = true
			break
		}
	}
	if !foundGold {
		t.Error("expected at least one customer with loyalty_tier=gold")
	}

	// ── 11. Verify GET /vehicles still works ──────────────────────────────────
	req2 := httptest.NewRequest(http.MethodGet, "/vehicles", nil)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("GET /vehicles: expected 200, got %d: %s", rec2.Code, rec2.Body.String())
	}

	var vehicles []map[string]any
	if err := json.NewDecoder(rec2.Body).Decode(&vehicles); err != nil {
		t.Fatalf("decode /vehicles response: %v", err)
	}
	if len(vehicles) != 3 {
		t.Errorf("expected 3 vehicles, got %d", len(vehicles))
	}

	// ── 12. Verify a snapshot was created ─────────────────────────────────────
	snaps, err := snapshot.List(vibeDir)
	if err != nil {
		t.Fatalf("list snapshots: %v", err)
	}
	if len(snaps) == 0 {
		t.Error("expected at least one snapshot to have been created")
	}
}
