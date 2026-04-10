package manifest

import (
	"testing"
)

func TestInjectTimestamps_AddsAllThree(t *testing.T) {
	schema := Schema{
		Table: "users",
		Columns: []Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "name", Type: "TEXT"},
		},
	}

	result := InjectTimestamps(schema)

	if len(result.Columns) != 5 {
		t.Fatalf("expected 5 columns, got %d", len(result.Columns))
	}

	names := map[string]bool{}
	for _, c := range result.Columns {
		names[c.Name] = true
	}
	for _, expected := range []string{"created_at", "updated_at", "deleted_at"} {
		if !names[expected] {
			t.Errorf("missing column %q", expected)
		}
	}
}

func TestInjectTimestamps_SkipsExisting(t *testing.T) {
	schema := Schema{
		Table: "users",
		Columns: []Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "created_at", Type: "DATETIME"},
			{Name: "updated_at", Type: "DATETIME"},
			{Name: "deleted_at", Type: "DATETIME"},
		},
	}

	result := InjectTimestamps(schema)

	if len(result.Columns) != 4 {
		t.Fatalf("should not duplicate — expected 4 columns, got %d", len(result.Columns))
	}
}

func TestInjectTimestamps_PartialExisting(t *testing.T) {
	schema := Schema{
		Table: "users",
		Columns: []Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "created_at", Type: "DATETIME"},
		},
	}

	result := InjectTimestamps(schema)

	if len(result.Columns) != 4 {
		t.Fatalf("expected 4 columns (id + created_at + 2 new), got %d", len(result.Columns))
	}
}

func TestInjectTimestamps_PreservesOriginal(t *testing.T) {
	schema := Schema{
		Table: "users",
		Columns: []Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
		},
	}

	result := InjectTimestamps(schema)

	if len(schema.Columns) != 1 {
		t.Error("InjectTimestamps should not mutate the original schema")
	}
	if len(result.Columns) != 4 {
		t.Fatalf("expected 4 columns in result, got %d", len(result.Columns))
	}
}
