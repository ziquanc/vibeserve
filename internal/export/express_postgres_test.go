package export

import (
	"strings"
	"testing"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

func TestGeneratePostgresSchema(t *testing.T) {
	schemas := []manifest.Schema{{
		Table: "users",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "name", Type: "TEXT", Required: true},
			{Name: "email", Type: "TEXT", Required: true, Unique: true},
			{Name: "score", Type: "REAL"},
			{Name: "active", Type: "BOOLEAN", Default: true},
			{Name: "created_at", Type: "DATETIME", Default: "NOW()"},
		},
	}, {
		Table: "posts",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "user_id", Type: "INTEGER", Required: true, References: "users(id)"},
			{Name: "title", Type: "TEXT", Required: true},
		},
	}}

	result := GeneratePostgresSchema(schemas)

	if !strings.Contains(result, "SERIAL PRIMARY KEY") {
		t.Error("INTEGER PRIMARY KEY AUTO should map to SERIAL PRIMARY KEY")
	}
	if !strings.Contains(result, "DOUBLE PRECISION") {
		t.Error("REAL should map to DOUBLE PRECISION")
	}
	if !strings.Contains(result, "BOOLEAN") {
		t.Error("BOOLEAN should stay BOOLEAN")
	}
	if !strings.Contains(result, "TIMESTAMP") {
		t.Error("DATETIME should map to TIMESTAMP")
	}
	if !strings.Contains(result, "NOT NULL") {
		t.Error("required columns should have NOT NULL")
	}
	if !strings.Contains(result, "UNIQUE") {
		t.Error("unique columns should have UNIQUE")
	}
	if !strings.Contains(result, "REFERENCES users(id)") {
		t.Error("foreign key references should be preserved")
	}
	if !strings.Contains(result, "DEFAULT NOW()") {
		t.Error("SQL expression defaults should be emitted raw")
	}
	if !strings.Contains(result, "DEFAULT true") {
		t.Error("boolean defaults should use true/false")
	}
}

func TestGeneratePostgresSeed(t *testing.T) {
	seeds := []manifest.Seed{{
		Table: "users",
		Rows: []map[string]any{
			{"id": float64(1), "name": "Alice", "email": "alice@example.com"},
			{"id": float64(2), "name": "Bob", "email": "bob@example.com"},
		},
	}}
	schemas := []manifest.Schema{{
		Table: "users",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "name", Type: "TEXT"},
			{Name: "email", Type: "TEXT"},
		},
	}}

	result := GeneratePostgresSeed(seeds, schemas)

	if !strings.Contains(result, "INSERT INTO users") {
		t.Error("should generate INSERT statements")
	}
	if !strings.Contains(result, "'Alice'") {
		t.Error("should contain string values in single quotes")
	}
	if !strings.Contains(result, "BEGIN") {
		t.Error("should wrap in transaction")
	}
	if !strings.Contains(result, "COMMIT") {
		t.Error("should wrap in transaction")
	}
	if !strings.Contains(result, "setval") {
		t.Error("should reset sequences after seeding auto-increment tables")
	}
}

func TestGeneratePostgresSeed_Empty(t *testing.T) {
	result := GeneratePostgresSeed(nil, nil)
	if !strings.Contains(result, "No seed data") {
		t.Error("empty seeds should return comment")
	}
}

func TestGeneratePostgresDatabaseJS(t *testing.T) {
	schemas := []manifest.Schema{{
		Table: "users",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "name", Type: "TEXT", Required: true},
			{Name: "email", Type: "TEXT", Required: true},
		},
	}}

	result := GeneratePostgresDatabaseJS(schemas)

	if !strings.Contains(result, "require('pg')") {
		t.Error("should use pg package")
	}
	if !strings.Contains(result, "DATABASE_URL") {
		t.Error("should connect via DATABASE_URL")
	}
	if !strings.Contains(result, "async function listUsers") {
		t.Error("list function should be async")
	}
	if !strings.Contains(result, "async function getUser") {
		t.Error("get function should be async")
	}
	if !strings.Contains(result, "async function createUser") {
		t.Error("create function should be async")
	}
	if !strings.Contains(result, "async function updateUser") {
		t.Error("update function should be async")
	}
	if !strings.Contains(result, "async function deleteUser") {
		t.Error("delete function should be async")
	}
	if !strings.Contains(result, "$1") {
		t.Error("should use $1 style parameterized queries")
	}
	if !strings.Contains(result, "RETURNING *") {
		t.Error("INSERT/UPDATE should use RETURNING *")
	}
	if !strings.Contains(result, "LIMIT") {
		t.Error("list function should support pagination")
	}
	if !strings.Contains(result, "closeDB") {
		t.Error("should export closeDB for graceful shutdown")
	}
}

func TestGeneratePostgresSchema_ForeignKeyIndexes(t *testing.T) {
	schemas := []manifest.Schema{{
		Table: "users",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "name", Type: "TEXT"},
		},
	}, {
		Table: "posts",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "user_id", Type: "INTEGER", Required: true, References: "users(id)"},
			{Name: "title", Type: "TEXT"},
		},
	}}

	result := GeneratePostgresSchema(schemas)

	if !strings.Contains(result, "CREATE INDEX") {
		t.Error("should generate indexes for foreign key columns")
	}
	if !strings.Contains(result, "idx_posts_user_id") {
		t.Error("index name should follow idx_table_column convention")
	}
}

func TestGeneratePostgresSchema_DeletedAtIndex(t *testing.T) {
	schemas := []manifest.Schema{{
		Table: "users",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "name", Type: "TEXT"},
		},
	}}

	result := GeneratePostgresSchema(schemas)

	if !strings.Contains(result, "idx_users_deleted_at") {
		t.Error("should generate index on deleted_at for soft delete filtering")
	}
}

func TestGeneratePostgresSchema_TimestampColumns(t *testing.T) {
	schemas := []manifest.Schema{{
		Table: "users",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "name", Type: "TEXT"},
		},
	}}

	result := GeneratePostgresSchema(schemas)

	if !strings.Contains(result, "created_at") {
		t.Error("schema should include created_at")
	}
	if !strings.Contains(result, "NOW()") {
		t.Error("timestamp defaults should use NOW() for PostgreSQL")
	}
}

func TestGeneratePostgresDatabaseJS_SoftDelete(t *testing.T) {
	schemas := []manifest.Schema{{
		Table: "users",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "name", Type: "TEXT"},
		},
	}}

	result := GeneratePostgresDatabaseJS(schemas)

	if !strings.Contains(result, "deleted_at IS NULL") {
		t.Error("list should filter WHERE deleted_at IS NULL")
	}
	if strings.Contains(result, "DELETE FROM") {
		t.Error("delete should soft delete, not hard DELETE")
	}
	if !strings.Contains(result, "updated_at = NOW()") {
		t.Error("update should set updated_at = NOW()")
	}
}

func TestGeneratePostgresSeed_ExcludesTimestampColumns(t *testing.T) {
	seeds := []manifest.Seed{{
		Table: "users",
		Rows: []map[string]any{
			{"id": float64(1), "name": "Alice"},
		},
	}}
	schemas := []manifest.Schema{{
		Table: "users",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "name", Type: "TEXT"},
		},
	}}

	result := GeneratePostgresSeed(seeds, schemas)

	// Should generate INSERT for the actual data
	if !strings.Contains(result, "INSERT INTO users") {
		t.Error("should generate INSERT statement")
	}

	// The INSERT should have id and name only, not timestamp columns
	if strings.Contains(result, "created_at") {
		t.Error("seed INSERT should not include auto-injected timestamp columns")
	}
	if strings.Contains(result, "updated_at") {
		t.Error("seed INSERT should not include auto-injected timestamp columns")
	}
	if strings.Contains(result, "deleted_at") {
		t.Error("seed INSERT should not include auto-injected timestamp columns")
	}
}

func TestGeneratePostgresDatabaseJS_ListFiltering(t *testing.T) {
	schemas := []manifest.Schema{{
		Table: "users",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "name", Type: "TEXT", Required: true},
			{Name: "email", Type: "TEXT", Required: true},
			{Name: "age", Type: "INTEGER"},
		},
	}}
	result := GeneratePostgresDatabaseJS(schemas)

	// Should accept options object
	if !strings.Contains(result, "async function listUsers(options = {})") {
		t.Error("list function should accept options object parameter")
	}

	// Should support sort parameter
	if !strings.Contains(result, "sort = 'id'") {
		t.Error("list function should support sort parameter with 'id' default")
	}

	// Should support order parameter
	if !strings.Contains(result, "order = 'asc'") {
		t.Error("list function should support order parameter with 'asc' default")
	}

	// Should use ILIKE for PostgreSQL case-insensitive search
	if !strings.Contains(result, "ILIKE") {
		t.Error("PostgreSQL search should use ILIKE for case-insensitive matching")
	}

	// Should only search TEXT columns (name, email) not INTEGER columns (age)
	if !strings.Contains(result, "name ILIKE") {
		t.Error("search should include TEXT column 'name'")
	}
	if !strings.Contains(result, "email ILIKE") {
		t.Error("search should include TEXT column 'email'")
	}
	if strings.Contains(result, "age ILIKE") {
		t.Error("search should NOT include INTEGER column 'age'")
	}

	// Should validate column names
	if !strings.Contains(result, "validColumns") {
		t.Error("should validate column names for sort/filter")
	}

	// Should support field filters
	if !strings.Contains(result, "Object.entries(filters)") {
		t.Error("should support field-specific filters via Object.entries")
	}

	// Should use numbered $N params
	if !strings.Contains(result, "$") {
		t.Error("should use $N numbered params for PostgreSQL")
	}
}

func TestFormatPostgresValue(t *testing.T) {
	tests := []struct {
		input    any
		expected string
	}{
		{nil, "NULL"},
		{"hello", "'hello'"},
		{"it's", "'it''s'"},
		{float64(42), "42"},
		{float64(3.14), "3.14"},
		{true, "true"},
		{false, "false"},
	}
	for _, tt := range tests {
		got := formatPostgresValue(tt.input)
		if got != tt.expected {
			t.Errorf("formatPostgresValue(%v) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
