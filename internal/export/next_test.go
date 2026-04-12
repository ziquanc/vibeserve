package export

import (
	"os"
	"path/filepath"
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

func TestNextLayout(t *testing.T) {
	m := testNextManifest()
	result := GenerateNextLayout(m)
	if !strings.Contains(result, "Sidebar") {
		t.Error("layout should include Sidebar")
	}
	if !strings.Contains(result, "test-api") {
		t.Error("layout should use manifest name as title")
	}
	if !strings.Contains(result, "export const metadata") {
		t.Error("layout should export metadata")
	}
	if !strings.Contains(result, "RootLayout") {
		t.Error("layout should export RootLayout")
	}
}

func TestNextLayoutDescriptionFallback(t *testing.T) {
	m := testNextManifest()
	// No description set — should fallback to "Admin Panel"
	result := GenerateNextLayout(m)
	if !strings.Contains(result, "Admin Panel") {
		t.Error("layout should fallback to 'Admin Panel' when no description")
	}

	// With description
	m.Description = "My Custom API"
	result = GenerateNextLayout(m)
	if !strings.Contains(result, "My Custom API") {
		t.Error("layout should use manifest description when set")
	}
	if strings.Contains(result, "Admin Panel") {
		t.Error("layout should not use fallback when description is set")
	}
}

func TestNextSidebar(t *testing.T) {
	m := testNextManifest()
	result := GenerateNextSidebar(m)
	if !strings.Contains(result, "/users") {
		t.Error("sidebar should have link to /users")
	}
	if !strings.Contains(result, "/posts") {
		t.Error("sidebar should have link to /posts")
	}
	if !strings.Contains(result, "lucide-react") {
		t.Error("should import icons from lucide-react")
	}
	if !strings.Contains(result, "usePathname") {
		t.Error("should use usePathname for active state")
	}
	if !strings.Contains(result, "'use client'") {
		t.Error("should be a client component")
	}
	if !strings.Contains(result, "test-api") {
		t.Error("should display the API name")
	}
	if !strings.Contains(result, "LayoutDashboard") {
		t.Error("should import LayoutDashboard for dashboard link")
	}
	if !strings.Contains(result, "Users") {
		t.Error("should have Users icon for users table")
	}
	if !strings.Contains(result, "FileText") {
		t.Error("should have FileText icon for posts table")
	}
}

func TestNextDashboard(t *testing.T) {
	m := testNextManifest()
	result := GenerateNextDashboard(m)
	if !strings.Contains(result, "Users") {
		t.Error("dashboard should show Users card")
	}
	if !strings.Contains(result, "Posts") {
		t.Error("dashboard should show Posts card")
	}
	if !strings.Contains(result, "Card") {
		t.Error("dashboard should use Card component")
	}
	if !strings.Contains(result, "'use client'") {
		t.Error("dashboard should be a client component")
	}
	if !strings.Contains(result, "test-api") {
		t.Error("dashboard should display API name")
	}
	if !strings.Contains(result, "3 columns") {
		t.Error("dashboard should show column count for users (3 columns)")
	}
	if !strings.Contains(result, "4 columns") {
		t.Error("dashboard should show column count for posts (4 columns)")
	}
	if !strings.Contains(result, "lucide-react") {
		t.Error("dashboard should import icons from lucide-react")
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

func TestRunNext(t *testing.T) {
	m := testNextManifest()
	outDir := filepath.Join(t.TempDir(), "next-export")

	exp := NewExporter(m, outDir)
	if err := exp.RunNext(); err != nil {
		t.Fatalf("RunNext failed: %v", err)
	}

	// Check key files exist
	expectedFiles := []string{
		"package.json",
		"next.config.ts",
		"tsconfig.json",
		".env.local",
		"src/app/layout.tsx",
		"src/app/page.tsx",
		"src/app/globals.css",
		"src/lib/api.ts",
		"src/lib/types.ts",
		"src/lib/utils.ts",
		"src/components/sidebar.tsx",
		"src/components/ui/button.tsx",
		"src/components/ui/table.tsx",
		"src/app/users/page.tsx",
		"src/app/users/new/page.tsx",
		"src/app/users/[id]/page.tsx",
		"src/app/users/[id]/edit/page.tsx",
		"src/app/posts/page.tsx",
	}

	for _, f := range expectedFiles {
		path := filepath.Join(outDir, f)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("missing file: %s", f)
		}
	}
}
