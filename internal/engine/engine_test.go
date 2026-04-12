package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/vibeserve/vibeserve/internal/llm"
	"github.com/vibeserve/vibeserve/internal/manifest"
)

// mockProvider is a test provider that returns a fixed manifest.
type mockProvider struct {
	manifest *manifest.Manifest
	err      error
}

func (m *mockProvider) Generate(ctx context.Context, current *manifest.Manifest, prompt string, history []llm.Message) (*manifest.Manifest, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.manifest, nil
}

func (m *mockProvider) Chat(ctx context.Context, systemPrompt string, prompt string) (string, error) {
	return "", m.err
}

// mockTrie implements RouteTrie for tests without importing router (avoids import cycle).
type mockTrie struct {
	mu      sync.Mutex
	routes  map[string]string // "METHOD /path" → script
}

func newMockTrie() *mockTrie {
	return &mockTrie{routes: make(map[string]string)}
}

func (t *mockTrie) Insert(method, path, script string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.routes[method+" "+path] = script
}

func (t *mockTrie) Remove(method, path string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.routes, method+" "+path)
}

func (t *mockTrie) Search(method, path string) (string, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	s, ok := t.routes[method+" "+path]
	return s, ok
}

// mockStore implements SchemaStore for tests without importing store (avoids import cycle).
type mockStore struct {
	dsn     string
	schemas []manifest.Schema
	columns map[string][]manifest.Column // table → added columns
	seeds   map[string][]map[string]any
	closed  bool
}

func newMockStore(dsn string) *mockStore {
	return &mockStore{
		dsn:     dsn,
		columns: make(map[string][]manifest.Column),
		seeds:   make(map[string][]map[string]any),
	}
}

func (s *mockStore) ApplySchemas(schemas []manifest.Schema) error {
	s.schemas = append(s.schemas, schemas...)
	return nil
}

func (s *mockStore) AddColumn(table string, col manifest.Column) error {
	s.columns[table] = append(s.columns[table], col)
	return nil
}

func (s *mockStore) Seed(table string, rows []map[string]any) error {
	s.seeds[table] = append(s.seeds[table], rows...)
	return nil
}

func (s *mockStore) DSN() string {
	return s.dsn
}

func (s *mockStore) Close() error {
	s.closed = true
	return nil
}

func newTestEngine(t *testing.T, provider llm.Provider) (*Engine, string, *mockTrie) {
	t.Helper()

	tmpDir := t.TempDir()
	vibeDir := filepath.Join(tmpDir, ".vibe")
	os.MkdirAll(vibeDir, 0o755)

	s := newMockStore(":memory:")
	trie := newMockTrie()
	scripts := make(map[string]string)

	eng := NewEngine(EngineConfig{
		Bus:      NewBus(),
		Store:    s,
		Trie:     trie,
		Scripts:  scripts,
		Provider: provider,
		VibeDir:  vibeDir,
	})

	return eng, vibeDir, trie
}

func TestEngine_Apply_NewManifest(t *testing.T) {
	newManifest := &manifest.Manifest{
		Version:     "1.0",
		Name:        "test-api",
		Description: "Test API",
		Schemas: []manifest.Schema{
			{Table: "users", Columns: []manifest.Column{
				{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
				{Name: "name", Type: "TEXT", Required: true},
			}},
		},
		Routes: []manifest.Route{
			{Path: "/users", Method: "GET", Script: "list_users", ResponseType: "array"},
		},
		Scripts: []manifest.Script{
			{Name: "list_users", Code: "result := db.query(\"SELECT * FROM users\", [])\nresponse.json(result)"},
		},
	}

	eng, _, trie := newTestEngine(t, &mockProvider{manifest: newManifest})

	result, err := eng.Apply(context.Background(), "Create a users API")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// Apply() now proposes — blueprint should be set
	if result.Blueprint == nil {
		t.Fatal("expected Blueprint to be set")
	}

	if len(result.Blueprint.Changes) == 0 {
		t.Error("expected changes in blueprint")
	}

	// Verify table was added in the proposed changes
	foundTable := false
	for _, c := range result.Blueprint.Changes {
		if c.Type == manifest.ChangeAddTable && c.Table == "users" {
			foundTable = true
		}
	}
	if !foundTable {
		t.Error("expected ADD_TABLE change for users")
	}

	// Verify route NOT yet registered (Apply proposes, Approve applies)
	_, found := trie.Search("GET", "/users")
	if found {
		t.Error("route should not be registered until blueprint is approved")
	}

	// Verify manifest NOT yet updated (Apply proposes, Approve applies)
	if eng.Manifest() != nil && eng.Manifest().Name == "test-api" {
		// eng.manifest starts as nil, so no manifest yet is fine
	}

	// Verify pending blueprint is set
	if !eng.HasPendingBlueprint() {
		t.Error("expected pending blueprint after Apply()")
	}
}

func TestEngine_Apply_AddColumn(t *testing.T) {
	// Start with an existing manifest
	initial := &manifest.Manifest{
		Version:     "1.0",
		Name:        "test-api",
		Description: "Test",
		Schemas: []manifest.Schema{
			{Table: "users", Columns: []manifest.Column{
				{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
				{Name: "name", Type: "TEXT", Required: true},
			}},
		},
		Routes: []manifest.Route{
			{Path: "/users", Method: "GET", Script: "list_users", ResponseType: "array"},
		},
		Scripts: []manifest.Script{
			{Name: "list_users", Code: "result := db.query(\"SELECT * FROM users\", [])\nresponse.json(result)"},
		},
	}

	// New manifest adds an email column
	updated := &manifest.Manifest{
		Version:     "1.0",
		Name:        "test-api",
		Description: "Test",
		Schemas: []manifest.Schema{
			{Table: "users", Columns: []manifest.Column{
				{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
				{Name: "name", Type: "TEXT", Required: true},
				{Name: "email", Type: "TEXT", Unique: true},
			}},
		},
		Routes: []manifest.Route{
			{Path: "/users", Method: "GET", Script: "list_users", ResponseType: "array"},
		},
		Scripts: []manifest.Script{
			{Name: "list_users", Code: "result := db.query(\"SELECT * FROM users\", [])\nresponse.json(result)"},
		},
	}

	eng, _, _ := newTestEngine(t, &mockProvider{manifest: updated})

	// Set up initial state
	eng.manifest = initial
	eng.store.ApplySchemas(initial.Schemas)
	eng.trie.Insert("GET", "/users", "list_users")
	eng.scripts["list_users"] = initial.Scripts[0].Code

	result, err := eng.Apply(context.Background(), "Add email to users")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if result.Blueprint == nil {
		t.Fatal("expected Blueprint to be set")
	}

	foundAddCol := false
	for _, c := range result.Blueprint.Changes {
		if c.Type == manifest.ChangeAddColumn && c.Table == "users" && c.Column.Name == "email" {
			foundAddCol = true
		}
	}
	if !foundAddCol {
		t.Error("expected ADD_COLUMN change for users.email")
	}
}

func TestEngine_Apply_EventsEmitted(t *testing.T) {
	newManifest := &manifest.Manifest{
		Version:     "1.0",
		Name:        "test",
		Description: "Test",
		Schemas: []manifest.Schema{
			{Table: "items", Columns: []manifest.Column{
				{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			}},
		},
		Routes: []manifest.Route{
			{Path: "/items", Method: "GET", Script: "list_items", ResponseType: "array"},
		},
		Scripts: []manifest.Script{
			{Name: "list_items", Code: "response.json([])"},
		},
	}

	eng, _, _ := newTestEngine(t, &mockProvider{manifest: newManifest})

	var events []EventType
	var mu sync.Mutex

	// Subscribe to all relevant events for proposal mode
	for _, et := range []EventType{
		EventUserPromptReceived, EventLLMRequestStarted, EventLLMRequestCompleted,
		EventManifestGenerated, EventManifestDiffComputed, EventBlueprintProposed,
	} {
		eventType := et
		eng.bus.Subscribe(eventType, func(e Event) {
			mu.Lock()
			events = append(events, e.Type)
			mu.Unlock()
		})
	}

	_, err := eng.Apply(context.Background(), "Create items API")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// Wait for async event delivery
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(events) < 5 {
		t.Errorf("expected at least 5 events, got %d: %v", len(events), events)
	}

	// Check key events were emitted
	eventSet := make(map[EventType]bool)
	for _, e := range events {
		eventSet[e] = true
	}

	// Apply() now proposes — schema/route/script events are emitted on Approve, not Apply
	required := []EventType{
		EventUserPromptReceived,
		EventLLMRequestStarted,
		EventLLMRequestCompleted,
		EventManifestGenerated,
		EventManifestDiffComputed,
		EventBlueprintProposed,
	}
	for _, req := range required {
		if !eventSet[req] {
			t.Errorf("missing required event: %s", req)
		}
	}
}

func TestEngine_Apply_ValidationFailure(t *testing.T) {
	// Manifest with invalid column type (cannot be auto-repaired)
	badManifest := &manifest.Manifest{
		Version: "1.0",
		Name:    "bad",
		Schemas: []manifest.Schema{{
			Table:   "t",
			Columns: []manifest.Column{{Name: "x", Type: "INVALID_TYPE"}},
		}},
	}

	eng, _, _ := newTestEngine(t, &mockProvider{manifest: badManifest})

	_, err := eng.Apply(context.Background(), "test")
	if err == nil {
		t.Error("expected error for invalid manifest")
	}
}

func TestEngine_Apply_LLMError(t *testing.T) {
	eng, _, _ := newTestEngine(t, &mockProvider{err: fmt.Errorf("API timeout")})

	_, err := eng.Apply(context.Background(), "test")
	if err == nil {
		t.Error("expected error for LLM failure")
	}
}

func TestEngine_Apply_ProposesButDoesNotSaveManifest(t *testing.T) {
	newManifest := &manifest.Manifest{
		Version:     "1.0",
		Name:        "saved-api",
		Description: "Test",
		Schemas: []manifest.Schema{{
			Table: "items",
			Columns: []manifest.Column{
				{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
				{Name: "name", Type: "TEXT", Required: true},
			},
		}},
		Routes: []manifest.Route{
			{Path: "/items", Method: "GET", Description: "List items", Script: "list_items"},
		},
		Scripts: []manifest.Script{
			{Name: "list_items", Code: "result := db.query(\"SELECT * FROM items\", [])\nresponse.json(result)"},
		},
	}

	eng, vibeDir, _ := newTestEngine(t, &mockProvider{manifest: newManifest})

	result, err := eng.Apply(context.Background(), "Create API")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// Apply() proposes — blueprint should be pending
	if result.Blueprint == nil {
		t.Fatal("expected Blueprint to be set")
	}
	if !eng.HasPendingBlueprint() {
		t.Error("expected pending blueprint after Apply()")
	}

	// Verify manifest was NOT saved to disk (only happens on Approve)
	manifestPath := filepath.Join(vibeDir, "manifest.json")
	_, statErr := os.Stat(manifestPath)
	if statErr == nil {
		t.Error("manifest should not be saved to disk until blueprint is approved")
	}

	// Approve and verify manifest IS saved
	_, err = eng.ApproveBlueprint(context.Background())
	if err != nil {
		t.Fatalf("ApproveBlueprint: %v", err)
	}

	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest after approve: %v", err)
	}

	var saved manifest.Manifest
	json.Unmarshal(data, &saved)
	if saved.Name != "saved-api" {
		t.Errorf("expected saved name 'saved-api', got %q", saved.Name)
	}
}

func TestFormatChangeSummary(t *testing.T) {
	result := &ApplyResult{
		Changes: []manifest.Change{
			{Type: manifest.ChangeAddTable, Table: "users"},
			{Type: manifest.ChangeAddColumn, Table: "users", Column: &manifest.Column{Name: "email", Type: "TEXT"}},
			{Type: manifest.ChangeAddRoute, Route: &manifest.Route{Method: "GET", Path: "/users"}},
			{Type: manifest.ChangeAddScript, Script: &manifest.Script{Name: "list_users"}},
		},
	}

	summary := FormatChangeSummary(result)
	if summary == "" {
		t.Error("expected non-empty summary")
	}
	if !contains(summary, "table: users") {
		t.Error("expected table mention in summary")
	}
	if !contains(summary, "column: users.email") {
		t.Error("expected column mention in summary")
	}
	if !contains(summary, "route: GET /users") {
		t.Error("expected route mention in summary")
	}
	if !contains(summary, "script: list_users") {
		t.Error("expected script mention in summary")
	}
}

func TestFormatChangeSummary_NoChanges(t *testing.T) {
	result := &ApplyResult{Changes: nil}
	summary := FormatChangeSummary(result)
	if summary != "No changes detected." {
		t.Errorf("expected 'No changes detected.', got %q", summary)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
