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

func TestExporter_CarRentalManifest(t *testing.T) {
	// Load the real test manifest
	m, err := manifest.LoadFromFile("../../testdata/car_rental_manifest.json")
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	outDir := t.TempDir()
	target := filepath.Join(outDir, "car-rental")

	exp := NewExporter(m, target)
	err = exp.Run()
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	// Verify models contain Vehicle and Booking structs
	modelsBytes, _ := os.ReadFile(filepath.Join(target, "internal/model/models.go"))
	models := string(modelsBytes)
	if !strings.Contains(models, "type Vehicle struct") {
		t.Error("missing Vehicle struct")
	}
	if !strings.Contains(models, "type Booking struct") {
		t.Error("missing Booking struct")
	}
	if !strings.Contains(models, "float64") {
		t.Error("missing float64 for REAL columns")
	}

	// Verify repository has usage-driven methods
	storeBytes, _ := os.ReadFile(filepath.Join(target, "internal/repository/store.go"))
	store := string(storeBytes)
	if !strings.Contains(store, "ListVehicle") {
		t.Error("missing ListVehicle* in store interface")
	}
	if !strings.Contains(store, "GetVehicle") {
		t.Error("missing GetVehicle in store interface")
	}
	if !strings.Contains(store, "CreateBooking") {
		t.Error("missing CreateBooking in store interface")
	}

	// Verify handlers reference chi and store
	handlersBytes, _ := os.ReadFile(filepath.Join(target, "internal/handler/handlers.go"))
	handlers := string(handlersBytes)
	if !strings.Contains(handlers, "func (h *Handler)") {
		t.Error("missing handler methods")
	}
	if !strings.Contains(handlers, "chi.URLParam") {
		t.Error("missing chi.URLParam in get_vehicle handler")
	}

	// Verify main.go has all routes registered
	mainBytes, _ := os.ReadFile(filepath.Join(target, "cmd/api/main.go"))
	mainCode := string(mainBytes)
	if !strings.Contains(mainCode, "/vehicles") {
		t.Error("missing /vehicles route in main.go")
	}
	if !strings.Contains(mainCode, "/bookings") {
		t.Error("missing /bookings route in main.go")
	}

	// Verify OpenAPI spec
	openapiBytes, _ := os.ReadFile(filepath.Join(target, "openapi.yaml"))
	openapi := string(openapiBytes)
	if !strings.Contains(openapi, "openapi:") {
		t.Error("missing openapi version")
	}
	if !strings.Contains(openapi, "/vehicles:") {
		t.Error("missing /vehicles path in OpenAPI")
	}

	// Verify README
	readmeBytes, _ := os.ReadFile(filepath.Join(target, "README.md"))
	readme := string(readmeBytes)
	if !strings.Contains(readme, "malaysia-car-rental") && !strings.Contains(readme, "Car") && !strings.Contains(readme, "car") {
		t.Error("missing project name in README")
	}

	// Verify no {{MODULE}} placeholders remain
	allFiles := []string{
		"internal/model/models.go",
		"internal/repository/store.go",
		"internal/repository/sqlite.go",
		"internal/handler/handlers.go",
		"cmd/api/main.go",
	}
	for _, f := range allFiles {
		data, _ := os.ReadFile(filepath.Join(target, f))
		if strings.Contains(string(data), "{{MODULE}}") {
			t.Errorf("%s still contains {{MODULE}} placeholder", f)
		}
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
