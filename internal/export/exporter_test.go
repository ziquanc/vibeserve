package export

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

func TestExporter_Run_CreatesAllFiles(t *testing.T) {
	m := &manifest.Manifest{
		Version:     "1.0",
		Name:        "test-api",
		Description: "A test API",
		Schemas: []manifest.Schema{
			{
				Table: "items",
				Columns: []manifest.Column{
					{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
					{Name: "name", Type: "TEXT", Required: true},
				},
			},
		},
		Routes: []manifest.Route{
			{Path: "/items", Method: "GET", Script: "list_items", ResponseType: "array"},
		},
		Scripts: []manifest.Script{
			{Name: "list_items", Code: "result := db.query(\"SELECT * FROM items\", [])\nresponse.json(result)"},
		},
	}

	outDir := t.TempDir()
	target := filepath.Join(outDir, "test-api")

	exp := NewExporter(m, target)
	err := exp.Run()
	if err != nil {
		t.Fatalf("Exporter.Run() failed: %v", err)
	}

	expectedFiles := []string{
		"cmd/api/main.go",
		"internal/model/models.go",
		"internal/repository/store.go",
		"internal/repository/sqlite.go",
		"internal/handler/handlers.go",
		"go.mod",
		"Dockerfile",
		"README.md",
		"openapi.yaml",
	}

	for _, f := range expectedFiles {
		path := filepath.Join(target, f)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected file %s to exist", f)
		}
	}
}

func TestExporter_DefaultOutputDir(t *testing.T) {
	m := &manifest.Manifest{
		Name:    "My Cool API",
		Version: "1.0",
	}

	dir := DefaultOutputDir(m)
	if dir != "vibe-export-my-cool-api" {
		t.Errorf("DefaultOutputDir = %q, want %q", dir, "vibe-export-my-cool-api")
	}
}

func TestExporter_ModuleReplace(t *testing.T) {
	m := &manifest.Manifest{
		Version: "1.0",
		Name:    "test-api",
		Schemas: []manifest.Schema{
			{Table: "items", Columns: []manifest.Column{{Name: "id", Type: "INTEGER", Primary: true, Auto: true}}},
		},
		Routes: []manifest.Route{
			{Path: "/items", Method: "GET", Script: "list_items", ResponseType: "array"},
		},
		Scripts: []manifest.Script{
			{Name: "list_items", Code: "result := db.query(\"SELECT * FROM items\", [])\nresponse.json(result)"},
		},
	}

	outDir := t.TempDir()
	target := filepath.Join(outDir, "test-api")

	exp := NewExporter(m, target)
	err := exp.Run()
	if err != nil {
		t.Fatalf("Run() failed: %v", err)
	}

	// Verify {{MODULE}} placeholder was replaced
	filesToCheck := []string{
		"internal/repository/store.go",
		"internal/repository/sqlite.go",
		"internal/handler/handlers.go",
	}
	for _, f := range filesToCheck {
		data, _ := os.ReadFile(filepath.Join(target, f))
		if strings.Contains(string(data), "{{MODULE}}") {
			t.Errorf("%s still contains {{MODULE}} placeholder", f)
		}
	}
}
