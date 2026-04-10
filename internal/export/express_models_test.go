package export

import (
	"strings"
	"testing"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

func TestGenerateDatabaseJS_PKReturnUsesActualPKName(t *testing.T) {
	schemas := []manifest.Schema{{
		Table: "orders",
		Columns: []manifest.Column{
			{Name: "order_id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "total", Type: "REAL"},
		},
	}}
	result := GenerateDatabaseJS(schemas)

	if strings.Contains(result, "{ id: result.lastInsertRowid") {
		t.Error("createXxx still uses hardcoded 'id' instead of actual PK column name")
	}
	if !strings.Contains(result, "{ order_id: result.lastInsertRowid") {
		t.Error("createXxx should use actual PK column name 'order_id'")
	}
}

func TestGenerateDatabaseJS_DefaultEscaping(t *testing.T) {
	schemas := []manifest.Schema{{
		Table: "users",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "status", Type: "TEXT", Default: "it's active"},
		},
	}}
	result := GenerateDatabaseJS(schemas)

	if strings.Contains(result, "DEFAULT 'it's active'") {
		t.Error("single quotes in DEFAULT value are not escaped")
	}
	if !strings.Contains(result, "DEFAULT 'it''s active'") {
		t.Error("single quotes in DEFAULT should be doubled for SQL escaping")
	}
}

func TestGenerateDatabaseJS_ListPagination(t *testing.T) {
	schemas := []manifest.Schema{{
		Table: "users",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "name", Type: "TEXT"},
		},
	}}
	result := GenerateDatabaseJS(schemas)

	if !strings.Contains(result, "LIMIT") {
		t.Error("listXxx should support pagination with LIMIT")
	}
	if !strings.Contains(result, "OFFSET") {
		t.Error("listXxx should support pagination with OFFSET")
	}
}
