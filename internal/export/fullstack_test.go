package export

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunFullstack(t *testing.T) {
	m := testNextManifest()
	outDir := filepath.Join(t.TempDir(), "fullstack-export")

	exp := NewExporter(m, outDir)
	if err := exp.RunFullstack(); err != nil {
		t.Fatalf("RunFullstack failed: %v", err)
	}

	expectedFiles := []string{
		"package.json",
		"README.md",
		".gitignore",
		"backend/package.json",
		"backend/server.js",
		"backend/src/models/database.js",
		"frontend/package.json",
		"frontend/next.config.ts",
		"frontend/src/app/layout.tsx",
		"frontend/src/lib/api.ts",
		"frontend/src/lib/types.ts",
	}

	for _, f := range expectedFiles {
		path := filepath.Join(outDir, f)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("missing file: %s", f)
		}
	}
}
