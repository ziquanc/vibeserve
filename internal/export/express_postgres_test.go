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
