package export

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

// Exporter orchestrates the 7-stage export pipeline, writing a complete Go
// project to disk from a VibeServe manifest.
type Exporter struct {
	manifest *manifest.Manifest
	outDir   string
	vibeDir  string
}

// NewExporter creates an Exporter that will write the generated project to outDir.
// vibeDir defaults to ".vibe" and is the directory where state.db lives.
func NewExporter(m *manifest.Manifest, outDir string) *Exporter {
	return &Exporter{
		manifest: m,
		outDir:   outDir,
		vibeDir:  ".vibe",
	}
}

// SetVibeDir overrides the default ".vibe" directory (useful for testing).
func (e *Exporter) SetVibeDir(dir string) {
	e.vibeDir = dir
}

// DefaultOutputDir returns "vibe-export-" + Slugify(m.Name).
func DefaultOutputDir(m *manifest.Manifest) string {
	return "vibe-export-" + Slugify(m.Name)
}

// Run executes the export pipeline:
//  1. Validate manifest
//  2. Create directory structure
//  3. Generate models → internal/model/models.go
//  4. Analyze route usage → generate store interface + sqlite impl → internal/repository/
//  5. Generate handlers → internal/handler/handlers.go
//  6. Generate scaffold: main.go, go.mod, Dockerfile, README
//  7. Generate OpenAPI → openapi.yaml
//  8. Copy state.db from vibeDir if it exists
//
// All {{MODULE}} placeholders in generated Go files are replaced with the
// slugified manifest name (which is also the go.mod module name).
func (e *Exporter) Run() error {
	m := e.manifest

	// Stage 1: Validate manifest.
	if err := manifest.Validate(m); err != nil {
		return fmt.Errorf("manifest validation: %w", err)
	}

	// Derive the module name used throughout.
	moduleName := Slugify(m.Name)

	// Stage 2: Create directory structure.
	dirs := []string{
		filepath.Join(e.outDir, "cmd", "api"),
		filepath.Join(e.outDir, "internal", "model"),
		filepath.Join(e.outDir, "internal", "repository"),
		filepath.Join(e.outDir, "internal", "handler"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create dir %s: %w", dir, err)
		}
	}

	// Stage 3: Generate models.
	modelsContent := GenerateModels(m.Schemas)
	if err := writeFile(filepath.Join(e.outDir, "internal", "model", "models.go"), modelsContent); err != nil {
		return fmt.Errorf("write models.go: %w", err)
	}

	// Stage 4: Analyze route usage → store interface + sqlite impl.
	usage := AnalyzeRouteUsage(m.Routes, m.Scripts)
	storeContent := replaceModule(GenerateStoreInterface(m.Schemas, usage), moduleName)
	sqliteContent := replaceModule(GenerateSQLiteStore(m.Schemas, usage), moduleName)
	if err := writeFile(filepath.Join(e.outDir, "internal", "repository", "store.go"), storeContent); err != nil {
		return fmt.Errorf("write store.go: %w", err)
	}
	if err := writeFile(filepath.Join(e.outDir, "internal", "repository", "sqlite.go"), sqliteContent); err != nil {
		return fmt.Errorf("write sqlite.go: %w", err)
	}

	// Stage 5: Generate handlers.
	handlersContent := replaceModule(GenerateHandlers(m.Schemas, m.Routes, m.Scripts), moduleName)
	if err := writeFile(filepath.Join(e.outDir, "internal", "handler", "handlers.go"), handlersContent); err != nil {
		return fmt.Errorf("write handlers.go: %w", err)
	}

	// Stage 6: Generate scaffold files.
	mainContent := GenerateMain(moduleName, m.Routes)
	if err := writeFile(filepath.Join(e.outDir, "cmd", "api", "main.go"), mainContent); err != nil {
		return fmt.Errorf("write main.go: %w", err)
	}

	goModContent := GenerateGoMod(moduleName)
	if err := writeFile(filepath.Join(e.outDir, "go.mod"), goModContent); err != nil {
		return fmt.Errorf("write go.mod: %w", err)
	}

	dockerfileContent := GenerateDockerfile(moduleName)
	if err := writeFile(filepath.Join(e.outDir, "Dockerfile"), dockerfileContent); err != nil {
		return fmt.Errorf("write Dockerfile: %w", err)
	}

	readmeContent := GenerateREADME(m)
	if err := writeFile(filepath.Join(e.outDir, "README.md"), readmeContent); err != nil {
		return fmt.Errorf("write README.md: %w", err)
	}

	// Stage 7: Generate OpenAPI spec.
	openAPIContent := GenerateOpenAPI(m)
	if err := writeFile(filepath.Join(e.outDir, "openapi.yaml"), openAPIContent); err != nil {
		return fmt.Errorf("write openapi.yaml: %w", err)
	}

	// Stage 8: Copy state.db from vibeDir if it exists.
	stateDBSrc := filepath.Join(e.vibeDir, "state.db")
	if data, err := os.ReadFile(stateDBSrc); err == nil {
		if err := os.WriteFile(filepath.Join(e.outDir, "state.db"), data, 0o644); err != nil {
			return fmt.Errorf("copy state.db: %w", err)
		}
	}

	return nil
}

// replaceModule replaces all occurrences of "{{MODULE}}" with the given module name.
func replaceModule(src, moduleName string) string {
	return strings.ReplaceAll(src, "{{MODULE}}", moduleName)
}

// writeFile writes content to path, creating or truncating the file as needed.
func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}
