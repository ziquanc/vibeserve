package manifest

import (
	"strings"
	"testing"
)

func TestGenerateMermaidER(t *testing.T) {
	schemas := []Schema{
		{Table: "users", Columns: []Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "name", Type: "TEXT", Required: true},
			{Name: "email", Type: "TEXT", Required: true, Unique: true},
		}},
		{Table: "posts", Columns: []Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "user_id", Type: "INTEGER", Required: true, References: "users.id"},
			{Name: "title", Type: "TEXT", Required: true},
		}},
	}

	result := GenerateMermaidER(schemas)

	if !strings.Contains(result, "erDiagram") {
		t.Error("should start with erDiagram")
	}
	if !strings.Contains(result, "users {") {
		t.Error("should contain users entity")
	}
	if !strings.Contains(result, "posts {") {
		t.Error("should contain posts entity")
	}
	if !strings.Contains(result, "PK") {
		t.Error("should mark primary keys")
	}
	if !strings.Contains(result, "FK") {
		t.Error("should mark foreign keys")
	}
	if !strings.Contains(result, "UK") {
		t.Error("should mark unique keys")
	}
	if !strings.Contains(result, "users ||--o{ posts") {
		t.Error("should draw relationship from users to posts")
	}
}

func TestGenerateMermaidER_ExcludesTimestamps(t *testing.T) {
	schemas := []Schema{{
		Table: "users",
		Columns: []Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "name", Type: "TEXT"},
			{Name: "created_at", Type: "DATETIME"},
			{Name: "updated_at", Type: "DATETIME"},
			{Name: "deleted_at", Type: "DATETIME"},
		},
	}}

	result := GenerateMermaidER(schemas)

	if strings.Contains(result, "created_at") {
		t.Error("should exclude timestamp columns for cleaner diagram")
	}
	if strings.Contains(result, "updated_at") {
		t.Error("should exclude timestamp columns")
	}
	if strings.Contains(result, "deleted_at") {
		t.Error("should exclude timestamp columns")
	}
}

func TestGenerateMermaidER_Empty(t *testing.T) {
	result := GenerateMermaidER(nil)
	if !strings.Contains(result, "erDiagram") {
		t.Error("should return valid mermaid even with no schemas")
	}
}
