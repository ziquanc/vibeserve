package manifest

import (
	"testing"
)

// helper: find the first change of a given type in the slice.
func findChange(changes []Change, ct ChangeType) *Change {
	for i := range changes {
		if changes[i].Type == ct {
			return &changes[i]
		}
	}
	return nil
}

// helper: count changes of a given type.
func countChanges(changes []Change, ct ChangeType) int {
	n := 0
	for _, c := range changes {
		if c.Type == ct {
			n++
		}
	}
	return n
}

// ── Test 1 ────────────────────────────────────────────────────────────────────

func TestDiff_NilOld_AllAdditions(t *testing.T) {
	newM := &Manifest{
		Schemas: []Schema{
			{Table: "users", Columns: []Column{{Name: "id", Type: "int", Primary: true}}},
		},
		Routes: []Route{
			{Method: "GET", Path: "/users", Script: "getUsers"},
		},
		Scripts: []Script{
			{Name: "getUsers", Code: "SELECT * FROM users"},
		},
		Seeds: []Seed{
			{Table: "users", Rows: []map[string]any{{"id": 1}}},
		},
	}

	changes := Diff(nil, newM)

	if findChange(changes, ChangeAddTable) == nil {
		t.Error("expected ADD_TABLE change")
	}
	if findChange(changes, ChangeAddRoute) == nil {
		t.Error("expected ADD_ROUTE change")
	}
	if findChange(changes, ChangeAddScript) == nil {
		t.Error("expected ADD_SCRIPT change")
	}
	if findChange(changes, ChangeAddSeed) == nil {
		t.Error("expected ADD_SEED change")
	}
}

// ── Test 2 ────────────────────────────────────────────────────────────────────

func TestDiff_AddNewTable(t *testing.T) {
	oldM := &Manifest{}
	newM := &Manifest{
		Schemas: []Schema{
			{Table: "orders", Columns: []Column{{Name: "id", Type: "int"}}},
		},
	}

	changes := Diff(oldM, newM)

	c := findChange(changes, ChangeAddTable)
	if c == nil {
		t.Fatal("expected ADD_TABLE change")
	}
	if c.Table != "orders" {
		t.Errorf("expected table %q, got %q", "orders", c.Table)
	}
	if c.Schema == nil || c.Schema.Table != "orders" {
		t.Error("expected Schema to be set on ADD_TABLE change")
	}
}

// ── Test 3 ────────────────────────────────────────────────────────────────────

func TestDiff_AddColumn(t *testing.T) {
	oldM := &Manifest{
		Schemas: []Schema{
			{Table: "users", Columns: []Column{{Name: "id", Type: "int"}}},
		},
	}
	newM := &Manifest{
		Schemas: []Schema{
			{Table: "users", Columns: []Column{
				{Name: "id", Type: "int"},
				{Name: "email", Type: "text"},
			}},
		},
	}

	changes := Diff(oldM, newM)

	c := findChange(changes, ChangeAddColumn)
	if c == nil {
		t.Fatal("expected ADD_COLUMN change")
	}
	if c.Table != "users" {
		t.Errorf("expected table %q, got %q", "users", c.Table)
	}
	if c.Column == nil || c.Column.Name != "email" {
		t.Error("expected Column.Name to be 'email'")
	}
}

// ── Test 4 ────────────────────────────────────────────────────────────────────

func TestDiff_DropColumn_Warning(t *testing.T) {
	oldM := &Manifest{
		Schemas: []Schema{
			{Table: "users", Columns: []Column{
				{Name: "id", Type: "int"},
				{Name: "phone", Type: "text"},
			}},
		},
	}
	newM := &Manifest{
		Schemas: []Schema{
			{Table: "users", Columns: []Column{
				{Name: "id", Type: "int"},
			}},
		},
	}

	changes := Diff(oldM, newM)

	c := findChange(changes, ChangeDropColumn)
	if c == nil {
		t.Fatal("expected DROP_COLUMN change")
	}
	if c.Column == nil || c.Column.Name != "phone" {
		t.Error("expected Column.Name to be 'phone'")
	}
}

// ── Test 5 ────────────────────────────────────────────────────────────────────

func TestDiff_AddRoute(t *testing.T) {
	oldM := &Manifest{}
	newM := &Manifest{
		Routes: []Route{
			{Method: "POST", Path: "/items", Script: "createItem"},
		},
	}

	changes := Diff(oldM, newM)

	c := findChange(changes, ChangeAddRoute)
	if c == nil {
		t.Fatal("expected ADD_ROUTE change")
	}
	if c.Route == nil || c.Route.Path != "/items" || c.Route.Method != "POST" {
		t.Error("expected Route to be set correctly")
	}
}

// ── Test 6 ────────────────────────────────────────────────────────────────────

func TestDiff_UpdateRoute(t *testing.T) {
	oldM := &Manifest{
		Routes: []Route{
			{Method: "GET", Path: "/users", Script: "getUsersV1"},
		},
	}
	newM := &Manifest{
		Routes: []Route{
			{Method: "GET", Path: "/users", Script: "getUsersV2"},
		},
	}

	changes := Diff(oldM, newM)

	c := findChange(changes, ChangeUpdateRoute)
	if c == nil {
		t.Fatal("expected UPDATE_ROUTE change")
	}
	if c.Route == nil || c.Route.Script != "getUsersV2" {
		t.Error("expected Route.Script to be 'getUsersV2'")
	}
}

// ── Test 7 ────────────────────────────────────────────────────────────────────

func TestDiff_RemoveRoute(t *testing.T) {
	oldM := &Manifest{
		Routes: []Route{
			{Method: "DELETE", Path: "/users/:id", Script: "deleteUser"},
		},
	}
	newM := &Manifest{}

	changes := Diff(oldM, newM)

	c := findChange(changes, ChangeRemoveRoute)
	if c == nil {
		t.Fatal("expected REMOVE_ROUTE change")
	}
	if c.Route == nil || c.Route.Path != "/users/:id" {
		t.Error("expected Route.Path to be '/users/:id'")
	}
}

// ── Test 8 ────────────────────────────────────────────────────────────────────

func TestDiff_AddScript(t *testing.T) {
	oldM := &Manifest{}
	newM := &Manifest{
		Scripts: []Script{
			{Name: "myScript", Code: "SELECT 1"},
		},
	}

	changes := Diff(oldM, newM)

	c := findChange(changes, ChangeAddScript)
	if c == nil {
		t.Fatal("expected ADD_SCRIPT change")
	}
	if c.Script == nil || c.Script.Name != "myScript" {
		t.Error("expected Script.Name to be 'myScript'")
	}
}

// ── Test 9 ────────────────────────────────────────────────────────────────────

func TestDiff_UpdateScript(t *testing.T) {
	oldM := &Manifest{
		Scripts: []Script{
			{Name: "calc", Code: "SELECT 1"},
		},
	}
	newM := &Manifest{
		Scripts: []Script{
			{Name: "calc", Code: "SELECT 2"},
		},
	}

	changes := Diff(oldM, newM)

	c := findChange(changes, ChangeUpdateScript)
	if c == nil {
		t.Fatal("expected UPDATE_SCRIPT change")
	}
	if c.Script == nil || c.Script.Code != "SELECT 2" {
		t.Error("expected Script.Code to be 'SELECT 2'")
	}
}

// ── Test 10 ───────────────────────────────────────────────────────────────────

func TestDiff_NoChanges(t *testing.T) {
	m := &Manifest{
		Schemas: []Schema{
			{Table: "users", Columns: []Column{{Name: "id", Type: "int"}}},
		},
		Routes: []Route{
			{Method: "GET", Path: "/users", Script: "getUsers"},
		},
		Scripts: []Script{
			{Name: "getUsers", Code: "SELECT * FROM users"},
		},
		Seeds: []Seed{
			{Table: "users", Rows: []map[string]any{{"id": 1}}},
		},
	}

	changes := Diff(m, m)

	if len(changes) != 0 {
		t.Errorf("expected 0 changes for identical manifests, got %d: %v", len(changes), changes)
	}
}
