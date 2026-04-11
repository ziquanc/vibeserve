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

func TestGenerateDatabaseJS_SoftDelete(t *testing.T) {
	schemas := []manifest.Schema{{
		Table: "users",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "name", Type: "TEXT"},
		},
	}}
	result := GenerateDatabaseJS(schemas)

	// list should filter out soft-deleted rows
	if !strings.Contains(result, "deleted_at IS NULL") {
		t.Error("listXxx should filter WHERE deleted_at IS NULL")
	}

	// delete should soft delete, not hard delete
	if strings.Contains(result, "DELETE FROM") {
		t.Error("deleteXxx should soft delete (SET deleted_at), not hard DELETE")
	}
	if !strings.Contains(result, "SET deleted_at") {
		t.Error("deleteXxx should SET deleted_at for soft delete")
	}
}

func TestGenerateDatabaseJS_UpdateTimestamp(t *testing.T) {
	schemas := []manifest.Schema{{
		Table: "users",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "name", Type: "TEXT"},
		},
	}}
	result := GenerateDatabaseJS(schemas)

	if !strings.Contains(result, "updated_at = CURRENT_TIMESTAMP") {
		t.Error("updateXxx should set updated_at to CURRENT_TIMESTAMP")
	}
}

func TestGenerateDatabaseJS_TimestampColumns(t *testing.T) {
	schemas := []manifest.Schema{{
		Table: "users",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "name", Type: "TEXT"},
		},
	}}
	result := GenerateDatabaseJS(schemas)

	if !strings.Contains(result, "created_at") {
		t.Error("CREATE TABLE should include created_at")
	}
	if !strings.Contains(result, "updated_at") {
		t.Error("CREATE TABLE should include updated_at")
	}
	if !strings.Contains(result, "deleted_at") {
		t.Error("CREATE TABLE should include deleted_at")
	}
}

func TestGenerateDatabaseJS_ListFiltering(t *testing.T) {
	schemas := []manifest.Schema{{
		Table: "users",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "name", Type: "TEXT"},
			{Name: "email", Type: "TEXT"},
			{Name: "age", Type: "INTEGER"},
		},
	}}
	result := GenerateDatabaseJS(schemas)

	// Should accept options object
	if !strings.Contains(result, "function listUsers(options = {})") {
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

	// Should support search parameter
	if !strings.Contains(result, "search") {
		t.Error("list function should support search parameter")
	}

	// Should use LIKE for text column search
	if !strings.Contains(result, "LIKE") {
		t.Error("search should use LIKE for text columns")
	}

	// Should only search TEXT columns (name, email) not INTEGER columns (age)
	if !strings.Contains(result, "name LIKE") {
		t.Error("search should include TEXT column 'name'")
	}
	if !strings.Contains(result, "email LIKE") {
		t.Error("search should include TEXT column 'email'")
	}
	if strings.Contains(result, "age LIKE") {
		t.Error("search should NOT include INTEGER column 'age'")
	}

	// Should validate column names for sort/filter
	if !strings.Contains(result, "validColumns") {
		t.Error("should validate column names for sort/filter")
	}

	// Should include all non-timestamp columns in validColumns
	if !strings.Contains(result, "'id'") {
		t.Error("validColumns should include 'id'")
	}
	if !strings.Contains(result, "'name'") {
		t.Error("validColumns should include 'name'")
	}
	if !strings.Contains(result, "'age'") {
		t.Error("validColumns should include 'age'")
	}

	// Should support ORDER BY with validated column
	if !strings.Contains(result, "ORDER BY") {
		t.Error("should include ORDER BY clause")
	}

	// Should support field filters
	if !strings.Contains(result, "Object.entries(filters)") {
		t.Error("should support field-specific filters via Object.entries")
	}
}

func TestGenerateDatabaseJS_CreateExcludesTimestamps(t *testing.T) {
	schemas := []manifest.Schema{{
		Table: "users",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "name", Type: "TEXT"},
			{Name: "email", Type: "TEXT"},
		},
	}}
	result := GenerateDatabaseJS(schemas)

	// The INSERT in createUser should NOT include timestamp columns as params
	// It should only have name and email
	if strings.Contains(result, "data.created_at") {
		t.Error("createXxx should not include created_at in INSERT params")
	}
	if strings.Contains(result, "data.updated_at") {
		t.Error("createXxx should not include updated_at in INSERT params")
	}
	if strings.Contains(result, "data.deleted_at") {
		t.Error("createXxx should not include deleted_at in INSERT params")
	}
}
