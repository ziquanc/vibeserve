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
