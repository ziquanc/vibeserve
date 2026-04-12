package export

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

// RunNext generates a Next.js App Router admin panel from the manifest.
func (e *Exporter) RunNext() error {
	m := e.manifest

	// Stage 0: Infer missing foreign keys from _id column patterns.
	m.Schemas = manifest.InferForeignKeys(m.Schemas)

	// Stage 1: Validate manifest.
	if err := manifest.Validate(m); err != nil {
		return fmt.Errorf("manifest validation: %w", err)
	}

	// Stage 2: Create directory structure.
	dirs := []string{
		filepath.Join(e.outDir, "src", "app"),
		filepath.Join(e.outDir, "src", "lib"),
		filepath.Join(e.outDir, "src", "components", "ui"),
	}
	// Per-resource page directories
	for _, schema := range m.Schemas {
		dirs = append(dirs, filepath.Join(e.outDir, "src", "app", schema.Table))
		dirs = append(dirs, filepath.Join(e.outDir, "src", "app", schema.Table, "new"))
		dirs = append(dirs, filepath.Join(e.outDir, "src", "app", schema.Table, "[id]"))
		dirs = append(dirs, filepath.Join(e.outDir, "src", "app", schema.Table, "[id]", "edit"))
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create dir %s: %w", dir, err)
		}
	}

	// Stage 3: Scaffold files.
	scaffoldFiles := map[string]string{
		"package.json":       GenerateNextPackageJSON(m.Name),
		"next.config.ts":     GenerateNextConfig(),
		"tsconfig.json":      GenerateNextTSConfig(),
		"tailwind.config.ts": GenerateNextTailwindConfig(),
		"postcss.config.mjs": GenerateNextPostCSS(),
		".env.local":         GenerateNextEnvLocal(),
		".gitignore":         GenerateNextGitignore(),
	}
	for name, content := range scaffoldFiles {
		if err := writeFile(filepath.Join(e.outDir, name), content); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}

	// Stage 4: Global CSS.
	if err := writeFile(filepath.Join(e.outDir, "src", "app", "globals.css"), GenerateNextGlobalCSS()); err != nil {
		return fmt.Errorf("write globals.css: %w", err)
	}

	// Stage 5: TypeScript types (reuse existing generator).
	typesTS := GenerateTypeScript(m.Schemas)
	if err := writeFile(filepath.Join(e.outDir, "src", "lib", "types.ts"), typesTS); err != nil {
		return fmt.Errorf("write types.ts: %w", err)
	}

	// Stage 6: API client.
	apiTS := GenerateNextAPIClient(m)
	if err := writeFile(filepath.Join(e.outDir, "src", "lib", "api.ts"), apiTS); err != nil {
		return fmt.Errorf("write api.ts: %w", err)
	}

	// Stage 7: UI components.
	uiComponents := GenerateNextUIComponents()
	for name, content := range uiComponents {
		if err := writeFile(filepath.Join(e.outDir, "src", name), content); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}

	// Stage 8: Layout + sidebar + dashboard.
	if err := writeFile(filepath.Join(e.outDir, "src", "app", "layout.tsx"), GenerateNextLayout(m)); err != nil {
		return fmt.Errorf("write layout.tsx: %w", err)
	}
	if err := writeFile(filepath.Join(e.outDir, "src", "components", "sidebar.tsx"), GenerateNextSidebar(m)); err != nil {
		return fmt.Errorf("write sidebar.tsx: %w", err)
	}
	if err := writeFile(filepath.Join(e.outDir, "src", "app", "page.tsx"), GenerateNextDashboard(m)); err != nil {
		return fmt.Errorf("write page.tsx: %w", err)
	}

	// Stage 9: Per-resource CRUD pages.
	for _, schema := range m.Schemas {
		pages := GenerateNextResourcePages(schema, schema.Table)
		for name, content := range pages {
			if err := writeFile(filepath.Join(e.outDir, "src", "app", name), content); err != nil {
				return fmt.Errorf("write %s: %w", name, err)
			}
		}
	}

	return nil
}
