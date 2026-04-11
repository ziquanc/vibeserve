package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/vibeserve/vibeserve/internal/engine"
	"github.com/vibeserve/vibeserve/internal/llm"
	"github.com/vibeserve/vibeserve/internal/manifest"
	"github.com/vibeserve/vibeserve/internal/router"
	"github.com/vibeserve/vibeserve/internal/store"
)

func setArgs(req *mcplib.CallToolRequest, args map[string]any) {
	data, _ := json.Marshal(args)
	json.Unmarshal(data, &req.Params.Arguments)
}

func setupTestServer(t *testing.T) *Server {
	t.Helper()
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}

	m := &manifest.Manifest{
		Version: "1.0",
		Name:    "test-api",
		Schemas: []manifest.Schema{{
			Table: "users",
			Columns: []manifest.Column{
				{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
				{Name: "name", Type: "TEXT", Required: true},
				{Name: "email", Type: "TEXT", Required: true, Unique: true},
			},
		}},
		Routes: []manifest.Route{
			{Path: "/users", Method: "GET", Description: "List users", Script: "list_users"},
			{Path: "/users/:id", Method: "GET", Description: "Get user", Script: "get_user"},
			{Path: "/users", Method: "POST", Description: "Create user", Script: "create_user"},
		},
		Scripts: []manifest.Script{
			{Name: "list_users", Code: "response.json([])"},
			{Name: "get_user", Code: "response.json({})"},
			{Name: "create_user", Code: "response.json({})"},
		},
	}

	if err := s.ApplySchemas(m.Schemas); err != nil {
		t.Fatal(err)
	}

	trie := router.NewTrie()
	scripts := make(map[string]string)
	for _, sc := range m.Scripts {
		scripts[sc.Name] = sc.Code
	}
	for _, r := range m.Routes {
		trie.Insert(r.Method, r.Path, r.Script)
	}

	bus := engine.NewBus()
	eng := engine.NewEngine(engine.EngineConfig{
		Bus:      bus,
		Store:    s,
		Trie:     trie,
		Scripts:  scripts,
		Manifest: m,
		VibeDir:  t.TempDir(),
	})

	return &Server{eng: eng, store: s, vibeDir: t.TempDir()}
}

func TestListRoutes(t *testing.T) {
	srv := setupTestServer(t)
	result, err := srv.handleListRoutes(context.Background(), mcplib.CallToolRequest{})
	if err != nil {
		t.Fatal(err)
	}
	text := result.Content[0].(mcplib.TextContent).Text
	if !strings.Contains(text, "GET") {
		t.Error("should contain GET method")
	}
	if !strings.Contains(text, "/users") {
		t.Error("should contain /users path")
	}
	if !strings.Contains(text, "3 total") {
		t.Error("should show route count")
	}
}

func TestListTables(t *testing.T) {
	srv := setupTestServer(t)
	result, err := srv.handleListTables(context.Background(), mcplib.CallToolRequest{})
	if err != nil {
		t.Fatal(err)
	}
	text := result.Content[0].(mcplib.TextContent).Text
	if !strings.Contains(text, "users") {
		t.Error("should contain users table")
	}
	if !strings.Contains(text, "name") {
		t.Error("should contain column names")
	}
}

func TestQueryData(t *testing.T) {
	srv := setupTestServer(t)

	// Insert a row first
	_, err := srv.store.Insert("users", map[string]any{"name": "Alice", "email": "alice@test.com"})
	if err != nil {
		t.Fatal(err)
	}

	req := mcplib.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"sql": "SELECT * FROM users",
	}

	result, err := srv.handleQueryData(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	text := result.Content[0].(mcplib.TextContent).Text
	if !strings.Contains(text, "Alice") {
		t.Error("query result should contain Alice")
	}
}

func TestQueryData_RejectsNonSelect(t *testing.T) {
	srv := setupTestServer(t)

	req := mcplib.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"sql": "DELETE FROM users WHERE id = 1",
	}

	result, err := srv.handleQueryData(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	text := result.Content[0].(mcplib.TextContent).Text
	if !strings.Contains(text, "Only SELECT") {
		t.Error("should reject non-SELECT queries")
	}
	if result.IsError != true {
		t.Error("should be marked as error")
	}
}

func TestInsertData(t *testing.T) {
	srv := setupTestServer(t)

	req := mcplib.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"table": "users",
		"data": map[string]any{
			"name":  "Bob",
			"email": "bob@test.com",
		},
	}

	result, err := srv.handleInsertData(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	text := result.Content[0].(mcplib.TextContent).Text
	if !strings.Contains(text, "Bob") {
		t.Error("insert result should contain Bob")
	}
}

func TestGetAPIStatus(t *testing.T) {
	srv := setupTestServer(t)
	result, err := srv.handleGetAPIStatus(context.Background(), mcplib.CallToolRequest{})
	if err != nil {
		t.Fatal(err)
	}
	text := result.Content[0].(mcplib.TextContent).Text
	var status map[string]any
	if err := json.Unmarshal([]byte(text), &status); err != nil {
		t.Fatalf("should return valid JSON: %v", err)
	}
	if status["tables"].(float64) != 1 {
		t.Error("should have 1 table")
	}
	if status["routes"].(float64) != 3 {
		t.Error("should have 3 routes")
	}
}

func TestCreateAPI_NoProvider(t *testing.T) {
	srv := setupTestServer(t)
	// setupTestServer creates srv with provider = nil

	req := mcplib.CallToolRequest{}
	setArgs(&req, map[string]any{
		"description": "Create a pet API",
	})

	result, err := srv.handleCreateAPI(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	if result.IsError != true {
		t.Error("should return error when no LLM provider configured")
	}
	text := result.Content[0].(mcplib.TextContent).Text
	if !strings.Contains(text, "LLM provider") {
		t.Error("error should mention LLM provider")
	}
}

func TestCreateAPI_NoDescription(t *testing.T) {
	srv := setupTestServer(t)
	srv.provider = &fakeProvider{}

	req := mcplib.CallToolRequest{}
	setArgs(&req, map[string]any{})

	result, err := srv.handleCreateAPI(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	if result.IsError != true {
		t.Error("should return error when description is missing")
	}
	text := result.Content[0].(mcplib.TextContent).Text
	if !strings.Contains(text, "description") {
		t.Error("error should mention description parameter")
	}
}

func TestAddFeature_NoProvider(t *testing.T) {
	srv := setupTestServer(t)

	req := mcplib.CallToolRequest{}
	setArgs(&req, map[string]any{
		"description": "Add comments",
	})

	result, err := srv.handleAddFeature(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	if result.IsError != true {
		t.Error("should return error when no LLM provider configured")
	}
	text := result.Content[0].(mcplib.TextContent).Text
	if !strings.Contains(text, "LLM provider") {
		t.Error("error should mention LLM provider")
	}
}

func TestAddFeature_NoManifest(t *testing.T) {
	// Create server with no manifest
	s, _ := store.New(":memory:")
	bus := engine.NewBus()
	eng := engine.NewEngine(engine.EngineConfig{
		Bus:     bus,
		Store:   s,
		Trie:    router.NewTrie(),
		Scripts: make(map[string]string),
		VibeDir: t.TempDir(),
	})
	srv := &Server{eng: eng, store: s, provider: &fakeProvider{}, vibeDir: t.TempDir()}

	req := mcplib.CallToolRequest{}
	setArgs(&req, map[string]any{
		"description": "Add comments",
	})

	result, err := srv.handleAddFeature(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	if result.IsError != true {
		t.Error("should return error when no manifest exists")
	}
	text := result.Content[0].(mcplib.TextContent).Text
	if !strings.Contains(text, "create_api") {
		t.Error("error should mention create_api")
	}
}

func TestUndo(t *testing.T) {
	srv := setupTestServer(t)

	result, err := srv.handleUndo(context.Background(), mcplib.CallToolRequest{})
	if err != nil {
		t.Fatal(err)
	}

	// May succeed or fail (no snapshots) — just shouldn't panic
	text := result.Content[0].(mcplib.TextContent).Text
	if text == "" {
		t.Error("should return a message")
	}
}

func TestExportProject(t *testing.T) {
	srv := setupTestServer(t)

	outDir := filepath.Join(t.TempDir(), "exported")
	req := mcplib.CallToolRequest{}
	setArgs(&req, map[string]any{
		"format":     "express",
		"output_dir": outDir,
	})

	result, err := srv.handleExportProject(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	text := result.Content[0].(mcplib.TextContent).Text
	if !strings.Contains(text, outDir) {
		t.Error("result should contain output directory")
	}

	// Verify files were created
	if _, err := os.Stat(filepath.Join(outDir, "package.json")); os.IsNotExist(err) {
		t.Error("package.json should exist in exported directory")
	}
	if _, err := os.Stat(filepath.Join(outDir, "server.js")); os.IsNotExist(err) {
		t.Error("server.js should exist in exported directory")
	}
}

func TestExportProject_NoManifest(t *testing.T) {
	s, _ := store.New(":memory:")
	bus := engine.NewBus()
	eng := engine.NewEngine(engine.EngineConfig{
		Bus:     bus,
		Store:   s,
		Trie:    router.NewTrie(),
		Scripts: make(map[string]string),
		VibeDir: t.TempDir(),
	})
	srv := &Server{eng: eng, store: s, vibeDir: t.TempDir()}

	req := mcplib.CallToolRequest{}
	setArgs(&req, map[string]any{"format": "express"})

	result, err := srv.handleExportProject(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	if result.IsError != true {
		t.Error("should return error when no manifest exists")
	}
}

// fakeProvider is a minimal LLM provider for tests that check validation gates
// before the LLM is actually called.
type fakeProvider struct{}

func (f *fakeProvider) Generate(ctx context.Context, current *manifest.Manifest, prompt string, history []llm.Message) (*manifest.Manifest, error) {
	return nil, fmt.Errorf("fakeProvider: not implemented")
}

func TestAllToolsRegistered(t *testing.T) {
	srv := setupTestServer(t)

	s := mcpserver.NewMCPServer("test", "1.0.0",
		mcpserver.WithToolCapabilities(true),
	)
	srv.registerTools(s)

	// Verify all 9 handlers can be called without panicking.
	ctx := context.Background()
	emptyReq := mcplib.CallToolRequest{}

	// Inspection tools — should succeed with test data
	if result, err := srv.handleListRoutes(ctx, emptyReq); err != nil || result == nil {
		t.Error("list_routes should work")
	}
	if result, err := srv.handleListTables(ctx, emptyReq); err != nil || result == nil {
		t.Error("list_tables should work")
	}
	if result, err := srv.handleGetAPIStatus(ctx, emptyReq); err != nil || result == nil {
		t.Error("get_api_status should work")
	}

	// Data tools — missing args should return error result, not panic
	if result, err := srv.handleQueryData(ctx, emptyReq); err != nil || result == nil {
		t.Error("query_data should not panic on empty request")
	}
	if result, err := srv.handleInsertData(ctx, emptyReq); err != nil || result == nil {
		t.Error("insert_data should not panic on empty request")
	}

	// LLM tools — no provider should return error result, not panic
	if result, err := srv.handleCreateAPI(ctx, emptyReq); err != nil || result == nil {
		t.Error("create_api should not panic on empty request")
	}
	if result, err := srv.handleAddFeature(ctx, emptyReq); err != nil || result == nil {
		t.Error("add_feature should not panic on empty request")
	}

	// Undo — should return a result (may be error)
	if result, err := srv.handleUndo(ctx, emptyReq); err != nil || result == nil {
		t.Error("undo should not panic")
	}

	// Export — empty args should handle gracefully
	if result, err := srv.handleExportProject(ctx, emptyReq); err != nil || result == nil {
		t.Error("export_project should not panic on empty request")
	}
}
