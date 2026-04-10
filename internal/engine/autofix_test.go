package engine_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/vibeserve/vibeserve/internal/engine"
	"github.com/vibeserve/vibeserve/internal/llm"
	"github.com/vibeserve/vibeserve/internal/manifest"
	"github.com/vibeserve/vibeserve/internal/router"
	"github.com/vibeserve/vibeserve/internal/store"
)

// autoFixMockProvider returns a broken manifest on the first call and a fixed
// one on subsequent calls. This simulates an LLM that generates bad Tengo
// scripts initially but fixes them when asked.
type autoFixMockProvider struct {
	calls   int
	broken  *manifest.Manifest
	fixed   *manifest.Manifest
}

func (p *autoFixMockProvider) Generate(_ context.Context, _ *manifest.Manifest, prompt string, _ []llm.Message) (*manifest.Manifest, error) {
	p.calls++
	// If the prompt contains "compilation errors" or "Fix them", return the fixed version
	if p.calls > 1 {
		return p.fixed, nil
	}
	return p.broken, nil
}

func (p *autoFixMockProvider) Chat(ctx context.Context, systemPrompt string, prompt string) (string, error) {
	return "", nil
}

// TestAutoFix_CompilationErrors verifies that when the LLM generates scripts
// with Tengo compilation errors, the engine auto-fixes them by sending the
// errors back to the LLM and retrying.
func TestAutoFix_CompilationErrors(t *testing.T) {
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

	// Broken manifest: scripts have "return" outside function and use "strlen"
	broken := &manifest.Manifest{
		Version:     "1.0",
		Name:        "todo-api",
		Description: "A todo API",
		Schemas: []manifest.Schema{
			{
				Table: "todos",
				Columns: []manifest.Column{
					{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
					{Name: "title", Type: "TEXT", Required: true},
					{Name: "done", Type: "INTEGER"},
					{Name: "created_at", Type: "DATETIME", Default: "NOW"},
				},
			},
		},
		Scripts: []manifest.Script{
			{
				Name: "list_todos",
				Code: `result := db.query("SELECT * FROM todos", [])
return result`,
			},
			{
				Name: "create_todo",
				Code: `body := request.body()
l := strlen(body.title)
result := db.insert("todos", body)
response.json(result, 201)`,
			},
		},
		Routes: []manifest.Route{
			{Path: "/todos", Method: "GET", Script: "list_todos", Description: "List todos"},
			{Path: "/todos", Method: "POST", Script: "create_todo", Description: "Create todo"},
		},
	}

	// Fixed manifest: scripts use proper Tengo syntax
	fixed := &manifest.Manifest{
		Version:     "1.0",
		Name:        "todo-api",
		Description: "A todo API",
		Schemas: []manifest.Schema{
			{
				Table: "todos",
				Columns: []manifest.Column{
					{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
					{Name: "title", Type: "TEXT", Required: true},
					{Name: "done", Type: "INTEGER"},
					{Name: "created_at", Type: "DATETIME", Default: "NOW"},
				},
			},
		},
		Scripts: []manifest.Script{
			{
				Name: "list_todos",
				Code: `result := db.query("SELECT * FROM todos", [])
response.json(result)`,
			},
			{
				Name: "create_todo",
				Code: `body := request.body()
result := db.insert("todos", body)
response.json(result, 201)`,
			},
		},
		Routes: []manifest.Route{
			{Path: "/todos", Method: "GET", Script: "list_todos", Description: "List todos"},
			{Path: "/todos", Method: "POST", Script: "create_todo", Description: "Create todo"},
		},
	}

	provider := &autoFixMockProvider{broken: broken, fixed: fixed}

	eng := engine.NewEngine(engine.EngineConfig{
		Bus:      bus,
		Store:    s,
		Trie:     trie,
		Scripts:  scripts,
		Provider: provider,
		Manifest: nil,
		VibeDir:  vibeDir,
	})

	// Track events
	var autoFixStarted, autoFixCompleted int
	bus.Subscribe(engine.EventAutoFixStarted, func(e engine.Event) {
		autoFixStarted++
	})
	bus.Subscribe(engine.EventAutoFixCompleted, func(e engine.Event) {
		autoFixCompleted++
		info := e.Data.(engine.AutoFixInfo)
		if !info.Fixed {
			t.Errorf("expected auto-fix to succeed, but it reported failure")
		}
	})

	// Verify the broken manifest actually fails validation
	if err := manifest.Validate(broken); err == nil {
		t.Fatal("expected broken manifest to fail validation, but it passed")
	}

	// Apply via the engine — it should auto-fix the compilation errors
	result, err := eng.ApplyAutoApprove(context.Background(), "create a todo API")
	if err != nil {
		t.Fatalf("ApplyAutoApprove failed: %v", err)
	}

	if len(result.Changes) == 0 {
		t.Error("expected changes after auto-fix, got 0")
	}

	// The LLM should have been called twice: once for initial generation,
	// once for auto-fix
	if provider.calls < 2 {
		t.Errorf("expected at least 2 LLM calls (generate + fix), got %d", provider.calls)
	}

	if autoFixStarted != 1 {
		t.Errorf("expected 1 auto-fix start event, got %d", autoFixStarted)
	}
	if autoFixCompleted != 1 {
		t.Errorf("expected 1 auto-fix completed event, got %d", autoFixCompleted)
	}
}

// TestAutoFix_NoProvider verifies that without an LLM provider, auto-fix
// doesn't panic and the validation error is returned as-is.
func TestAutoFix_NoProvider(t *testing.T) {
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
		Provider: nil, // no provider
		Manifest: nil,
		VibeDir:  vibeDir,
	})

	broken := &manifest.Manifest{
		Version: "1.0",
		Name:    "test",
		Scripts: []manifest.Script{
			{Name: "bad", Code: "return 1"},
		},
		Routes: []manifest.Route{
			{Path: "/test", Method: "GET", Script: "bad"},
		},
	}

	result, err := eng.ApplyManifestDirect(context.Background(), "test", broken)
	if err == nil {
		t.Fatal("expected error for broken manifest without provider, got nil")
	}
	if result != nil {
		t.Error("expected nil result on failure")
	}
}
