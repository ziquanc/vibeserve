package manifest

import "testing"

func TestValidateStructural_Valid(t *testing.T) {
	m := &Manifest{
		Version: "1.0", Name: "test", Description: "d",
		Schemas: []Schema{{Table: "users", Columns: []Column{
			{Name: "id", Type: "INTEGER", Primary: true},
		}}},
		Routes:  []Route{{Path: "/users", Method: "GET", Script: "list", ResponseType: "array"}},
		Scripts: []Script{{Name: "list", Code: "response.json([])"}},
		Seeds:   []Seed{},
	}
	if err := Validate(m); err != nil {
		t.Errorf("expected valid manifest, got error: %v", err)
	}
}

func TestValidateStructural_MissingName(t *testing.T) {
	m := &Manifest{Version: "1.0", Name: "", Description: "d"}
	err := Validate(m)
	if err == nil {
		t.Error("expected error for missing name")
	}
}

func TestValidateStructural_InvalidMethod(t *testing.T) {
	m := &Manifest{
		Version: "1.0", Name: "test", Description: "d",
		Schemas: []Schema{{Table: "t", Columns: []Column{{Name: "id", Type: "INTEGER"}}}},
		Routes:  []Route{{Path: "/t", Method: "INVALID", Script: "s", ResponseType: "object"}},
		Scripts: []Script{{Name: "s", Code: "x := 1"}},
		Seeds:   []Seed{},
	}
	err := Validate(m)
	if err == nil {
		t.Error("expected error for invalid HTTP method")
	}
}

func TestValidateStructural_InvalidColumnType(t *testing.T) {
	m := &Manifest{
		Version: "1.0", Name: "test", Description: "d",
		Schemas: []Schema{{Table: "t", Columns: []Column{{Name: "id", Type: "VECTOR"}}}},
		Routes:  []Route{},
		Scripts: []Script{},
		Seeds:   []Seed{},
	}
	err := Validate(m)
	if err == nil {
		t.Error("expected error for invalid column type")
	}
}

func TestValidateStructural_DuplicateTable(t *testing.T) {
	m := &Manifest{
		Version: "1.0", Name: "test", Description: "d",
		Schemas: []Schema{
			{Table: "t", Columns: []Column{{Name: "id", Type: "INTEGER"}}},
			{Table: "t", Columns: []Column{{Name: "id", Type: "INTEGER"}}},
		},
		Routes: []Route{}, Scripts: []Script{}, Seeds: []Seed{},
	}
	err := Validate(m)
	if err == nil {
		t.Error("expected error for duplicate table name")
	}
}

func TestValidateReferential_ScriptNotFound(t *testing.T) {
	m := &Manifest{
		Version: "1.0", Name: "test", Description: "d",
		Schemas: []Schema{{Table: "t", Columns: []Column{{Name: "id", Type: "INTEGER"}}}},
		Routes:  []Route{{Path: "/t", Method: "GET", Script: "nonexistent", ResponseType: "array"}},
		Scripts: []Script{{Name: "other", Code: "x := 1"}},
		Seeds:   []Seed{},
	}
	err := Validate(m)
	if err == nil {
		t.Error("expected error for route referencing nonexistent script")
	}
}

func TestValidateReferential_SeedTableNotFound(t *testing.T) {
	m := &Manifest{
		Version: "1.0", Name: "test", Description: "d",
		Schemas: []Schema{{Table: "users", Columns: []Column{{Name: "id", Type: "INTEGER"}}}},
		Routes:  []Route{},
		Scripts: []Script{},
		Seeds:   []Seed{{Table: "nonexistent", Rows: []map[string]any{}}},
	}
	err := Validate(m)
	if err == nil {
		t.Error("expected error for seed referencing nonexistent table")
	}
}

func TestValidateReferential_ForeignKeyInvalid(t *testing.T) {
	m := &Manifest{
		Version: "1.0", Name: "test", Description: "d",
		Schemas: []Schema{{Table: "t", Columns: []Column{
			{Name: "id", Type: "INTEGER"},
			{Name: "ref", Type: "INTEGER", References: "nonexistent.id"},
		}}},
		Routes: []Route{}, Scripts: []Script{}, Seeds: []Seed{},
	}
	err := Validate(m)
	if err == nil {
		t.Error("expected error for invalid foreign key reference")
	}
}

func TestValidateCompilation_BadSyntax(t *testing.T) {
	m := &Manifest{
		Version: "1.0", Name: "test", Description: "d",
		Schemas: []Schema{{Table: "t", Columns: []Column{{Name: "id", Type: "INTEGER"}}}},
		Routes:  []Route{{Path: "/t", Method: "GET", Script: "bad", ResponseType: "array"}},
		Scripts: []Script{{Name: "bad", Code: "if { broken syntax !!!"}},
		Seeds:   []Seed{},
	}
	err := Validate(m)
	if err == nil {
		t.Error("expected error for script with syntax error")
	}
}
