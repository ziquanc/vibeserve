package manifest

import (
	"strings"
	"testing"
)

func TestDetectAuth(t *testing.T) {
	// With password_hash — should detect
	schemas := []Schema{{Table: "users", Columns: []Column{{Name: "id", Type: "INTEGER"}, {Name: "password_hash", Type: "TEXT"}}}}
	if !DetectAuth(schemas) {
		t.Error("should detect auth")
	}

	// Without password_hash — should not detect
	schemas2 := []Schema{{Table: "users", Columns: []Column{{Name: "id", Type: "INTEGER"}, {Name: "name", Type: "TEXT"}}}}
	if DetectAuth(schemas2) {
		t.Error("should not detect without password_hash")
	}

	// No users table — should not detect
	schemas3 := []Schema{{Table: "products", Columns: []Column{{Name: "id", Type: "INTEGER"}}}}
	if DetectAuth(schemas3) {
		t.Error("should not detect without users table")
	}
}

func TestDetectAuthCaseInsensitive(t *testing.T) {
	schemas := []Schema{{Table: "Users", Columns: []Column{{Name: "id", Type: "INTEGER"}, {Name: "password", Type: "TEXT"}}}}
	if !DetectAuth(schemas) {
		t.Error("should detect auth with case-insensitive table name and password column")
	}
}

func TestGenerateAuthTables(t *testing.T) {
	tables := GenerateAuthTables()
	if len(tables) != 2 {
		t.Fatalf("expected 2 tables, got %d", len(tables))
	}
	if tables[0].Table != "refresh_tokens" {
		t.Error("first table should be refresh_tokens")
	}
	if tables[1].Table != "password_resets" {
		t.Error("second table should be password_resets")
	}
}

func TestGenerateAuthRoutes(t *testing.T) {
	routes, scripts := GenerateAuthRoutes("users")
	if len(routes) != 9 {
		t.Errorf("expected 9 routes, got %d", len(routes))
	}
	if len(scripts) != 9 {
		t.Errorf("expected 9 scripts, got %d", len(scripts))
	}

	// Check key routes exist
	paths := map[string]bool{}
	for _, r := range routes {
		paths[r.Method+" "+r.Path] = true
	}
	expected := []string{"POST /auth/register", "POST /auth/login", "POST /auth/refresh", "GET /me", "PUT /me"}
	for _, e := range expected {
		if !paths[e] {
			t.Errorf("missing route: %s", e)
		}
	}

	// Check register script has hash_password
	for _, s := range scripts {
		if s.Name == "auth_register" {
			if !strings.Contains(s.Code, "hash_password") {
				t.Error("register should hash password")
			}
			if !strings.Contains(s.Code, "generate_tokens") {
				t.Error("register should generate tokens")
			}
		}
	}
}
