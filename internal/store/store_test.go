package store

import (
	"testing"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

func testSchemas() []manifest.Schema {
	return []manifest.Schema{{
		Table: "users",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "name", Type: "TEXT", Required: true},
			{Name: "active", Type: "BOOLEAN", Default: true},
			{Name: "score", Type: "REAL"},
		},
	}}
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.ApplySchemas(testSchemas()); err != nil {
		t.Fatalf("ApplySchemas: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestApplySchemas(t *testing.T) {
	s := newTestStore(t)
	// Insert a row to verify the table exists and schema is correct
	row, err := s.Insert("users", map[string]any{"name": "Alice"})
	if err != nil {
		t.Fatalf("Insert after ApplySchemas: %v", err)
	}
	if row["name"] != "Alice" {
		t.Errorf("expected name Alice, got %v", row["name"])
	}
}

func TestApplySchemas_Idempotent(t *testing.T) {
	s := newTestStore(t)
	// Applying schemas again should not error (IF NOT EXISTS)
	if err := s.ApplySchemas(testSchemas()); err != nil {
		t.Fatalf("ApplySchemas second call: %v", err)
	}
}

func TestInsertAndQuery(t *testing.T) {
	s := newTestStore(t)

	_, err := s.Insert("users", map[string]any{"name": "Alice", "score": 9.5})
	if err != nil {
		t.Fatalf("Insert Alice: %v", err)
	}
	_, err = s.Insert("users", map[string]any{"name": "Bob", "score": 7.0})
	if err != nil {
		t.Fatalf("Insert Bob: %v", err)
	}

	rows, err := s.Query("SELECT * FROM users ORDER BY id", nil)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0]["name"] != "Alice" {
		t.Errorf("expected Alice, got %v", rows[0]["name"])
	}
	if rows[1]["name"] != "Bob" {
		t.Errorf("expected Bob, got %v", rows[1]["name"])
	}
}

func TestInsertReturnsFullRow(t *testing.T) {
	s := newTestStore(t)
	row, err := s.Insert("users", map[string]any{"name": "Charlie", "score": 5.5})
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if row["id"] == nil {
		t.Error("expected id to be set after insert")
	}
	if row["name"] != "Charlie" {
		t.Errorf("expected name Charlie, got %v", row["name"])
	}
	if row["score"] != 5.5 {
		t.Errorf("expected score 5.5, got %v", row["score"])
	}
}

func TestQueryOne_Found(t *testing.T) {
	s := newTestStore(t)
	inserted, err := s.Insert("users", map[string]any{"name": "Dana"})
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	id := inserted["id"]

	row, err := s.QueryOne("SELECT * FROM users WHERE id = ?", []any{id})
	if err != nil {
		t.Fatalf("QueryOne: %v", err)
	}
	if row == nil {
		t.Fatal("expected row, got nil")
	}
	if row["name"] != "Dana" {
		t.Errorf("expected Dana, got %v", row["name"])
	}
}

func TestQueryOne_NotFound(t *testing.T) {
	s := newTestStore(t)
	row, err := s.QueryOne("SELECT * FROM users WHERE id = ?", []any{99999})
	if err != nil {
		t.Fatalf("QueryOne not found: %v", err)
	}
	if row != nil {
		t.Errorf("expected nil row for not found, got %v", row)
	}
}

func TestUpdate(t *testing.T) {
	s := newTestStore(t)
	inserted, err := s.Insert("users", map[string]any{"name": "Eve"})
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	id := inserted["id"]

	updated, err := s.Update("users", id, map[string]any{"name": "Eve Updated", "score": 8.0})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated["name"] != "Eve Updated" {
		t.Errorf("expected Eve Updated, got %v", updated["name"])
	}
	if updated["score"] != 8.0 {
		t.Errorf("expected score 8.0, got %v", updated["score"])
	}
}

func TestDelete_Exists(t *testing.T) {
	s := newTestStore(t)
	inserted, err := s.Insert("users", map[string]any{"name": "Frank"})
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	id := inserted["id"]

	deleted, err := s.Delete("users", id)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !deleted {
		t.Error("expected Delete to return true")
	}

	row, err := s.QueryOne("SELECT * FROM users WHERE id = ?", []any{id})
	if err != nil {
		t.Fatalf("QueryOne after delete: %v", err)
	}
	if row != nil {
		t.Error("expected row to be gone after delete")
	}
}

func TestDelete_NotExists(t *testing.T) {
	s := newTestStore(t)
	deleted, err := s.Delete("users", 99999)
	if err != nil {
		t.Fatalf("Delete non-existent: %v", err)
	}
	if deleted {
		t.Error("expected Delete to return false for non-existent row")
	}
}

func TestSeed(t *testing.T) {
	s := newTestStore(t)
	err := s.Seed("users", []map[string]any{
		{"name": "Grace", "score": 10.0},
		{"name": "Hank", "score": 6.5},
	})
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}

	count, err := s.Count("users")
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 seeded rows, got %d", count)
	}
}

func TestCount(t *testing.T) {
	s := newTestStore(t)

	count, err := s.Count("users")
	if err != nil {
		t.Fatalf("Count empty: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0, got %d", count)
	}

	s.Insert("users", map[string]any{"name": "Ivan"})
	s.Insert("users", map[string]any{"name": "Judy"})

	count, err = s.Count("users")
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2, got %d", count)
	}
}

func TestBooleanTypeMapping(t *testing.T) {
	s := newTestStore(t)

	// Insert with explicit active=true
	row, err := s.Insert("users", map[string]any{"name": "Karl", "active": true})
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}

	// Check that the returned row has bool, not int64
	active, ok := row["active"]
	if !ok {
		t.Fatal("active field missing from insert result")
	}
	if _, isBool := active.(bool); !isBool {
		t.Errorf("expected bool for active, got %T (%v)", active, active)
	}
	if active.(bool) != true {
		t.Errorf("expected active=true, got %v", active)
	}

	// Also verify via Query
	rows, err := s.Query("SELECT * FROM users WHERE name = ?", []any{"Karl"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	qActive := rows[0]["active"]
	if _, isBool := qActive.(bool); !isBool {
		t.Errorf("Query: expected bool for active, got %T (%v)", qActive, qActive)
	}
}

func TestBooleanTypeMapping_FalseValue(t *testing.T) {
	s := newTestStore(t)

	row, err := s.Insert("users", map[string]any{"name": "Lena", "active": false})
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	active, ok := row["active"]
	if !ok {
		t.Fatal("active field missing")
	}
	if _, isBool := active.(bool); !isBool {
		t.Errorf("expected bool, got %T (%v)", active, active)
	}
	if active.(bool) != false {
		t.Errorf("expected false, got %v", active)
	}
}

func TestQuery_WithParams(t *testing.T) {
	s := newTestStore(t)
	s.Insert("users", map[string]any{"name": "Mike", "score": 5.0})
	s.Insert("users", map[string]any{"name": "Nina", "score": 9.0})

	rows, err := s.Query("SELECT * FROM users WHERE score > ?", []any{7.0})
	if err != nil {
		t.Fatalf("Query with param: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0]["name"] != "Nina" {
		t.Errorf("expected Nina, got %v", rows[0]["name"])
	}
}

func TestAddColumn(t *testing.T) {
	s := newTestStore(t)
	s.Insert("users", map[string]any{"name": "Alice"})

	col := manifest.Column{Name: "email", Type: "TEXT"}
	if err := s.AddColumn("users", col); err != nil {
		t.Fatalf("AddColumn: %v", err)
	}

	row, err := s.Insert("users", map[string]any{"name": "Bob", "email": "bob@test.com"})
	if err != nil {
		t.Fatalf("Insert after AddColumn: %v", err)
	}
	if row["email"] != "bob@test.com" {
		t.Errorf("expected email 'bob@test.com', got %v", row["email"])
	}

	oldRow, _ := s.QueryOne("SELECT * FROM users WHERE name = ?", []any{"Alice"})
	if oldRow["email"] != nil {
		t.Errorf("expected nil email for old row, got %v", oldRow["email"])
	}
}

func TestAddColumn_Boolean(t *testing.T) {
	s := newTestStore(t)
	col := manifest.Column{Name: "verified", Type: "BOOLEAN", Default: false}
	if err := s.AddColumn("users", col); err != nil {
		t.Fatalf("AddColumn: %v", err)
	}
	row, _ := s.Insert("users", map[string]any{"name": "Charlie", "verified": true})
	if v, ok := row["verified"].(bool); !ok || !v {
		t.Errorf("expected verified=true (bool), got %T: %v", row["verified"], row["verified"])
	}
}

func TestStoreDSN(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()
	if s.DSN() != ":memory:" {
		t.Errorf("expected ':memory:', got %q", s.DSN())
	}
}
