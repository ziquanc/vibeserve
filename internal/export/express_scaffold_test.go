package export

import (
	"strings"
	"testing"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

func TestGenerateServerJS_JWTStartupValidation(t *testing.T) {
	m := &manifest.Manifest{Name: "test", Version: "1.0"}
	result := GenerateServerJS(m)
	if !strings.Contains(result, "JWT_SECRET") || !strings.Contains(result, "process.exit") {
		t.Error("server.js should validate JWT_SECRET on startup and exit if missing")
	}
}

func TestGenerateServerJS_CORSDefault(t *testing.T) {
	m := &manifest.Manifest{Name: "test", Version: "1.0"}
	result := GenerateServerJS(m)
	if strings.Contains(result, "CORS_ORIGIN || '*'") {
		t.Error("CORS should not default to *")
	}
	if !strings.Contains(result, "localhost") {
		t.Error("CORS should default to localhost")
	}
}

func TestGenerateGitignore(t *testing.T) {
	result := GenerateGitignore()
	for _, entry := range []string{"node_modules", ".env", "*.db"} {
		if !strings.Contains(result, entry) {
			t.Errorf(".gitignore should contain %q", entry)
		}
	}
}

func TestGenerateExpressDockerfile_Postgres(t *testing.T) {
	result := GenerateExpressDockerfile("postgres")
	if strings.Contains(result, "DATABASE_PATH") {
		t.Error("postgres Dockerfile should not have DATABASE_PATH")
	}
	if strings.Contains(result, "mkdir -p src/data") {
		t.Error("postgres Dockerfile should not create src/data")
	}
}

func TestGenerateEnvExample_Postgres(t *testing.T) {
	result := GenerateEnvExample("postgres")
	if !strings.Contains(result, "DATABASE_URL") {
		t.Error("postgres env should have DATABASE_URL")
	}
	if strings.Contains(result, "DATABASE_PATH") {
		t.Error("postgres env should not have DATABASE_PATH")
	}
}

func TestGeneratePackageJSON_Postgres(t *testing.T) {
	m := &manifest.Manifest{Name: "test-api", Version: "1.0"}
	result := GeneratePackageJSON(m, "postgres")
	if !strings.Contains(result, "\"pg\"") {
		t.Error("should include pg dependency")
	}
	if strings.Contains(result, "better-sqlite3") {
		t.Error("should not include better-sqlite3")
	}
}
