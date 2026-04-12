package export

import (
	"strings"
	"testing"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

func testNextManifest() *manifest.Manifest {
	return &manifest.Manifest{
		Version: "1.0",
		Name:    "test-api",
		Schemas: []manifest.Schema{
			{Table: "users", Columns: []manifest.Column{
				{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
				{Name: "name", Type: "TEXT", Required: true},
				{Name: "email", Type: "TEXT", Required: true, Unique: true},
			}},
			{Table: "posts", Columns: []manifest.Column{
				{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
				{Name: "user_id", Type: "INTEGER", Required: true, References: "users.id"},
				{Name: "title", Type: "TEXT", Required: true},
				{Name: "body", Type: "TEXT"},
			}},
		},
	}
}

func TestNextPackageJSON(t *testing.T) {
	result := GenerateNextPackageJSON("test-api")
	for _, dep := range []string{"next", "react", "tailwindcss", "typescript", "lucide-react"} {
		if !strings.Contains(result, dep) {
			t.Errorf("package.json should include %s", dep)
		}
	}
	if !strings.Contains(result, "test-api") {
		t.Error("should use slugified name")
	}
}

func TestNextConfig(t *testing.T) {
	result := GenerateNextConfig()
	if !strings.Contains(result, "rewrites") {
		t.Error("should include API proxy rewrites")
	}
	if !strings.Contains(result, "API_URL") {
		t.Error("should reference API_URL env variable")
	}
	if !strings.Contains(result, "/api/:path*") {
		t.Error("should proxy /api/* paths")
	}
}

func TestNextTSConfig(t *testing.T) {
	result := GenerateNextTSConfig()
	if !strings.Contains(result, "@/*") {
		t.Error("should have @/* path alias")
	}
}

func TestNextGlobalCSS(t *testing.T) {
	result := GenerateNextGlobalCSS()
	if !strings.Contains(result, "tailwindcss") {
		t.Error("should import tailwindcss")
	}
}

func TestNextAPIClient(t *testing.T) {
	m := testNextManifest()
	result := GenerateNextAPIClient(m)

	// Should have fetchAPI helper
	if !strings.Contains(result, "async function fetchAPI") {
		t.Error("should have fetchAPI helper")
	}

	// Should have CRUD for users
	if !strings.Contains(result, "listUsers") {
		t.Error("should generate listUsers")
	}
	if !strings.Contains(result, "getUser") {
		t.Error("should generate getUser")
	}
	if !strings.Contains(result, "createUser") {
		t.Error("should generate createUser")
	}
	if !strings.Contains(result, "updateUser") {
		t.Error("should generate updateUser")
	}
	if !strings.Contains(result, "deleteUser") {
		t.Error("should generate deleteUser")
	}

	// Should have CRUD for posts
	if !strings.Contains(result, "listPosts") {
		t.Error("should generate listPosts")
	}

	// Should use /api/ prefix
	if !strings.Contains(result, "/api") {
		t.Error("should use /api prefix for proxy")
	}

	// Should import types
	if !strings.Contains(result, "import type") {
		t.Error("should import types")
	}
	if !strings.Contains(result, "User") {
		t.Error("should import User type")
	}
	if !strings.Contains(result, "CreateUserInput") {
		t.Error("should import CreateUserInput")
	}
	if !strings.Contains(result, "ListOptions") {
		t.Error("should import ListOptions")
	}

	// Should support search/sort
	if !strings.Contains(result, "URLSearchParams") {
		t.Error("should use URLSearchParams for query building")
	}
}

func TestNextUIComponents(t *testing.T) {
	components := GenerateNextUIComponents()

	expected := []string{
		"lib/utils.ts",
		"components/ui/button.tsx",
		"components/ui/input.tsx",
		"components/ui/label.tsx",
		"components/ui/card.tsx",
		"components/ui/table.tsx",
		"components/ui/badge.tsx",
	}

	for _, name := range expected {
		content, ok := components[name]
		if !ok {
			t.Errorf("missing component: %s", name)
			continue
		}
		if len(content) < 50 {
			t.Errorf("component %s seems too short (%d bytes)", name, len(content))
		}
	}

	// Verify utils has cn function
	if !strings.Contains(components["lib/utils.ts"], "export function cn") {
		t.Error("utils.ts should export cn function")
	}

	// Verify button uses CVA
	if !strings.Contains(components["components/ui/button.tsx"], "cva") {
		t.Error("button should use class-variance-authority")
	}

	// Verify table has all parts
	tableContent := components["components/ui/table.tsx"]
	for _, part := range []string{"Table", "TableHeader", "TableBody", "TableRow", "TableHead", "TableCell"} {
		if !strings.Contains(tableContent, part) {
			t.Errorf("table.tsx should export %s", part)
		}
	}
}
