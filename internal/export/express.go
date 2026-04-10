package export

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

// RunExpress generates a production-ready Express.js project from the manifest.
// It writes the complete project to e.outDir with the following structure:
//
//	project/
//	  package.json
//	  server.js
//	  .env.example
//	  Dockerfile
//	  README.md
//	  openapi.yaml
//	  src/
//	    middleware/
//	      auth.js
//	      validate.js
//	    routes/
//	      index.js
//	      {resource}.js
//	    models/
//	      database.js
//	    data/
//	      state.db
func (e *Exporter) RunExpress() error {
	m := e.manifest

	// Stage 1: Validate manifest.
	if err := manifest.Validate(m); err != nil {
		return fmt.Errorf("manifest validation: %w", err)
	}

	// Stage 2: Create directory structure.
	dirs := []string{
		filepath.Join(e.outDir, "src", "middleware"),
		filepath.Join(e.outDir, "src", "routes"),
		filepath.Join(e.outDir, "src", "models"),
		filepath.Join(e.outDir, "src", "data"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create dir %s: %w", dir, err)
		}
	}

	// Stage 3: Generate scaffold files.
	packageJSON := GeneratePackageJSON(m, "sqlite")
	if err := writeFile(filepath.Join(e.outDir, "package.json"), packageJSON); err != nil {
		return fmt.Errorf("write package.json: %w", err)
	}

	serverJS := GenerateServerJS(m)
	if err := writeFile(filepath.Join(e.outDir, "server.js"), serverJS); err != nil {
		return fmt.Errorf("write server.js: %w", err)
	}

	envExample := GenerateEnvExample("sqlite")
	if err := writeFile(filepath.Join(e.outDir, ".env.example"), envExample); err != nil {
		return fmt.Errorf("write .env.example: %w", err)
	}

	gitignore := GenerateGitignore()
	if err := writeFile(filepath.Join(e.outDir, ".gitignore"), gitignore); err != nil {
		return fmt.Errorf("write .gitignore: %w", err)
	}

	dockerfile := GenerateExpressDockerfile("sqlite")
	if err := writeFile(filepath.Join(e.outDir, "Dockerfile"), dockerfile); err != nil {
		return fmt.Errorf("write Dockerfile: %w", err)
	}

	readme := GenerateExpressREADME(m)
	if err := writeFile(filepath.Join(e.outDir, "README.md"), readme); err != nil {
		return fmt.Errorf("write README.md: %w", err)
	}

	// Stage 4: Generate OpenAPI spec (reuse existing generator).
	openAPIContent := GenerateOpenAPI(m)
	if err := writeFile(filepath.Join(e.outDir, "openapi.yaml"), openAPIContent); err != nil {
		return fmt.Errorf("write openapi.yaml: %w", err)
	}

	// Stage 5: Generate database model.
	databaseJS := GenerateDatabaseJS(m.Schemas)
	if err := writeFile(filepath.Join(e.outDir, "src", "models", "database.js"), databaseJS); err != nil {
		return fmt.Errorf("write database.js: %w", err)
	}

	// Stage 6: Generate middleware.
	authJS := GenerateAuthMiddleware()
	if err := writeFile(filepath.Join(e.outDir, "src", "middleware", "auth.js"), authJS); err != nil {
		return fmt.Errorf("write auth.js: %w", err)
	}

	validateJS := GenerateValidateMiddleware()
	if err := writeFile(filepath.Join(e.outDir, "src", "middleware", "validate.js"), validateJS); err != nil {
		return fmt.Errorf("write validate.js: %w", err)
	}

	// Stage 7: Generate per-resource route files and index.
	routeFiles := GenerateExpressRoutes(m.Schemas, m.Routes, m.Scripts)
	for filename, content := range routeFiles {
		if err := writeFile(filepath.Join(e.outDir, "src", "routes", filename), content); err != nil {
			return fmt.Errorf("write routes/%s: %w", filename, err)
		}
	}

	// Stage 8: Copy state.db from vibeDir if it exists.
	stateDBSrc := filepath.Join(e.vibeDir, "state.db")
	if data, err := os.ReadFile(stateDBSrc); err == nil {
		if err := os.WriteFile(filepath.Join(e.outDir, "src", "data", "state.db"), data, 0o644); err != nil {
			return fmt.Errorf("copy state.db: %w", err)
		}
	}

	return nil
}
