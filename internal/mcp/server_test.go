package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"

	"github.com/vibeserve/vibeserve/internal/engine"
	"github.com/vibeserve/vibeserve/internal/manifest"
	"github.com/vibeserve/vibeserve/internal/router"
	"github.com/vibeserve/vibeserve/internal/store"
)

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
