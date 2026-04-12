package apitest

import (
	"testing"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

func TestBuildTestData(t *testing.T) {
	schema := manifest.Schema{
		Table: "users",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "name", Type: "TEXT", Required: true},
			{Name: "age", Type: "INTEGER"},
			{Name: "score", Type: "REAL"},
			{Name: "active", Type: "BOOLEAN"},
			{Name: "created_at", Type: "DATETIME"},
			{Name: "updated_at", Type: "DATETIME"},
			{Name: "deleted_at", Type: "DATETIME"},
		},
	}

	data := buildTestData(schema)

	if _, ok := data["id"]; ok {
		t.Error("should not include auto PK")
	}
	if _, ok := data["created_at"]; ok {
		t.Error("should not include timestamps")
	}
	if data["name"] == nil {
		t.Error("should include name")
	}
	if data["age"] == nil {
		t.Error("should include age")
	}
	if _, ok := data["score"]; !ok {
		t.Error("should include score")
	}
	if _, ok := data["active"]; !ok {
		t.Error("should include active")
	}
}

func TestReplacePathParam(t *testing.T) {
	tests := []struct {
		path     string
		value    string
		expected string
	}{
		{"/users/:id", "42", "/users/42"},
		{"/users/:id/posts/:postId", "42", "/users/42/posts/42"},
		{"/users", "42", "/users"},
	}
	for _, tt := range tests {
		got := replacePathParam(tt.path, tt.value)
		if got != tt.expected {
			t.Errorf("replacePathParam(%q, %q) = %q, want %q", tt.path, tt.value, got, tt.expected)
		}
	}
}

func TestBuildUpdateData(t *testing.T) {
	schema := manifest.Schema{
		Table: "users",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "name", Type: "TEXT"},
			{Name: "age", Type: "INTEGER"},
			{Name: "created_at", Type: "DATETIME"},
		},
	}

	data := buildUpdateData(schema)

	if _, ok := data["id"]; ok {
		t.Error("should not include PK")
	}
	if _, ok := data["created_at"]; ok {
		t.Error("should not include timestamps")
	}
	if data["name"] != "updated_name" {
		t.Errorf("name should be 'updated_name', got %v", data["name"])
	}
}
