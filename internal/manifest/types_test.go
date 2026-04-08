package manifest

import (
	"encoding/json"
	"testing"
)

func TestParseManifest(t *testing.T) {
	raw := `{
		"version": "1.0",
		"name": "test-api",
		"description": "A test API",
		"schemas": [
			{
				"table": "users",
				"columns": [
					{"name": "id", "type": "INTEGER", "primary": true, "auto": true},
					{"name": "name", "type": "TEXT", "required": true},
					{"name": "active", "type": "BOOLEAN", "default": true}
				]
			}
		],
		"routes": [
			{
				"path": "/users",
				"method": "GET",
				"description": "List users",
				"script": "list_users",
				"response_type": "array"
			},
			{
				"path": "/users",
				"method": "POST",
				"description": "Create user",
				"script": "create_user",
				"request_body": {"name": "TEXT", "active": "BOOLEAN"},
				"response_type": "object"
			}
		],
		"scripts": [
			{"name": "list_users", "code": "result := db.query(\"SELECT * FROM users\", [])"},
			{"name": "create_user", "code": "body := request.body()\ndb.insert(\"users\", body)"}
		],
		"seeds": [
			{
				"table": "users",
				"rows": [
					{"name": "Alice", "active": true},
					{"name": "Bob", "active": false}
				]
			}
		]
	}`

	var m Manifest
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("failed to parse manifest: %v", err)
	}

	if m.Name != "test-api" {
		t.Errorf("expected name 'test-api', got %q", m.Name)
	}
	if len(m.Schemas) != 1 {
		t.Fatalf("expected 1 schema, got %d", len(m.Schemas))
	}
	if m.Schemas[0].Table != "users" {
		t.Errorf("expected table 'users', got %q", m.Schemas[0].Table)
	}
	if len(m.Schemas[0].Columns) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(m.Schemas[0].Columns))
	}
	if !m.Schemas[0].Columns[0].Primary {
		t.Error("expected first column to be primary")
	}
	if len(m.Routes) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(m.Routes))
	}
	if m.Routes[0].Script != "list_users" {
		t.Errorf("expected script 'list_users', got %q", m.Routes[0].Script)
	}
	if len(m.Scripts) != 2 {
		t.Fatalf("expected 2 scripts, got %d", len(m.Scripts))
	}
	if len(m.Seeds) != 1 {
		t.Fatalf("expected 1 seed, got %d", len(m.Seeds))
	}
	if len(m.Seeds[0].Rows) != 2 {
		t.Fatalf("expected 2 seed rows, got %d", len(m.Seeds[0].Rows))
	}
}

func TestParseColumnDefaults(t *testing.T) {
	raw := `{
		"version": "1.0", "name": "t", "description": "",
		"schemas": [{"table": "t", "columns": [
			{"name": "a", "type": "TEXT", "default": "hello"},
			{"name": "b", "type": "INTEGER", "default": 42},
			{"name": "c", "type": "BOOLEAN", "default": true},
			{"name": "d", "type": "DATETIME", "default": "NOW"},
			{"name": "e", "type": "TEXT", "unique": true},
			{"name": "f", "type": "INTEGER", "references": "users.id"}
		]}],
		"routes": [], "scripts": [], "seeds": []
	}`

	var m Manifest
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	cols := m.Schemas[0].Columns
	if cols[0].Default == nil {
		t.Error("expected default for column a")
	}
	if cols[4].Unique != true {
		t.Error("expected column e to be unique")
	}
	if cols[5].References != "users.id" {
		t.Errorf("expected references 'users.id', got %q", cols[5].References)
	}
}

func TestManifestRoundTrip(t *testing.T) {
	m := Manifest{
		Version:     "1.0",
		Name:        "roundtrip",
		Description: "test",
		Schemas: []Schema{{
			Table: "items",
			Columns: []Column{
				{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
				{Name: "title", Type: "TEXT", Required: true},
			},
		}},
		Routes: []Route{{
			Path:         "/items",
			Method:       "GET",
			Description:  "list",
			Script:       "list_items",
			ResponseType: "array",
		}},
		Scripts: []Script{{Name: "list_items", Code: "response.json([])"}},
		Seeds:   []Seed{},
	}

	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var m2 Manifest
	if err := json.Unmarshal(data, &m2); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if m2.Name != m.Name || len(m2.Schemas) != len(m.Schemas) {
		t.Error("round-trip mismatch")
	}
}
