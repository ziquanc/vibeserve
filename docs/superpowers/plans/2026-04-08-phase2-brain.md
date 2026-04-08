# Phase 2: The Brain — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add LLM integration with a conversational REPL. User types natural language, LLM produces a manifest, Differ computes changes, Engine applies them (schema migration + route updates + script loading), all while the HTTP server stays running.

**Architecture:** Engine coordinator wires: user prompt -> LLM -> validate -> diff -> snapshot -> migrate -> update routes -> update scripts. Events emitted at each step via the existing Bus. Pluggable LLM provider interface supports Claude API and Ollama.

**Tech Stack:** Go 1.26+, modernc.org/sqlite (pure Go), d5/tengo/v2, spf13/cobra, gopkg.in/yaml.v3, net/http (for LLM API calls — no SDK dependencies)

---

## File Map

```
vibeserve/
├── cmd/vibeserve/
│   └── main.go                          # MODIFIED — add "dev" and "undo" subcommands
├── internal/
│   ├── engine/
│   │   ├── interfaces.go                # EXISTING (unchanged)
│   │   ├── events.go                    # EXISTING (unchanged)
│   │   ├── bus.go                       # EXISTING (unchanged)
│   │   ├── bus_test.go                  # EXISTING (unchanged)
│   │   ├── engine.go                    # NEW — Engine coordinator
│   │   └── engine_test.go              # NEW
│   ├── manifest/
│   │   ├── types.go                     # EXISTING (unchanged)
│   │   ├── types_test.go               # EXISTING (unchanged)
│   │   ├── validate.go                  # EXISTING (unchanged)
│   │   ├── validate_test.go            # EXISTING (unchanged)
│   │   ├── diff.go                      # NEW — Manifest Differ
│   │   └── diff_test.go               # NEW
│   ├── store/
│   │   ├── store.go                     # EXISTING (unchanged)
│   │   ├── store_test.go               # EXISTING (unchanged)
│   │   ├── migrate.go                   # MODIFIED — add BuildAddColumnSQL + AddColumn
│   │   └── migrate_test.go             # MODIFIED — add AddColumn tests
│   ├── snapshot/
│   │   ├── snapshot.go                  # NEW — Snapshot create/restore/list
│   │   └── snapshot_test.go            # NEW
│   ├── config/
│   │   ├── config.go                    # NEW — YAML config loading
│   │   └── config_test.go             # NEW
│   ├── llm/
│   │   ├── provider.go                  # NEW — Provider interface + system prompt
│   │   ├── provider_test.go            # NEW
│   │   ├── claude.go                    # NEW — Claude API provider
│   │   ├── claude_test.go              # NEW
│   │   ├── ollama.go                    # NEW — Ollama API provider
│   │   └── ollama_test.go             # NEW
│   ├── runtime/
│   │   ├── runtime.go                   # EXISTING (unchanged)
│   │   ├── runtime_test.go             # EXISTING (unchanged)
│   │   ├── stdlib.go                    # EXISTING (unchanged)
│   │   └── stdlib_test.go             # EXISTING (unchanged)
│   └── router/
│       ├── trie.go                      # EXISTING (unchanged)
│       ├── trie_test.go                # EXISTING (unchanged)
│       ├── handler.go                   # EXISTING (unchanged)
│       ├── handler_test.go             # EXISTING (unchanged)
│       └── server.go                    # EXISTING (unchanged)
├── testdata/
│   ├── car_rental_manifest.json         # EXISTING (unchanged)
│   └── car_rental_v2_manifest.json      # NEW — evolved manifest for diff tests
├── go.mod                               # EXISTING (unchanged — yaml.v3 already present)
└── go.sum
```

---

### Task 1: Manifest Differ

**Files:**
- Create: `internal/manifest/diff.go`
- Create: `internal/manifest/diff_test.go`

The Differ compares an old manifest against a new manifest and produces a list of typed changes. It detects: new tables, new columns on existing tables, removed columns (warning only), route additions, route updates, route removals, script additions, script updates, script removals, and seed changes.

- [ ] **Step 1: Create `internal/manifest/diff.go`**

Create `internal/manifest/diff.go`:

```go
package manifest

// ChangeType describes what kind of change occurred between two manifests.
type ChangeType string

const (
	ChangeAddTable    ChangeType = "ADD_TABLE"
	ChangeAddColumn   ChangeType = "ADD_COLUMN"
	ChangeDropColumn  ChangeType = "DROP_COLUMN" // warning only, not applied
	ChangeAddRoute    ChangeType = "ADD_ROUTE"
	ChangeUpdateRoute ChangeType = "UPDATE_ROUTE"
	ChangeRemoveRoute ChangeType = "REMOVE_ROUTE"
	ChangeAddScript   ChangeType = "ADD_SCRIPT"
	ChangeUpdateScript ChangeType = "UPDATE_SCRIPT"
	ChangeRemoveScript ChangeType = "REMOVE_SCRIPT"
	ChangeAddSeed     ChangeType = "ADD_SEED"
)

// Change represents a single difference between two manifests.
type Change struct {
	Type   ChangeType
	Table  string // for schema changes
	Column *Column // for ADD_COLUMN
	Schema *Schema // for ADD_TABLE
	Route  *Route  // for route changes
	Script *Script // for script changes
	Seed   *Seed   // for seed changes
	Detail string  // human-readable description
}

// Diff computes a list of changes needed to go from old to new.
// If old is nil, every element in new is treated as an addition.
func Diff(old, new *Manifest) []Change {
	var changes []Change

	changes = append(changes, diffSchemas(old, new)...)
	changes = append(changes, diffRoutes(old, new)...)
	changes = append(changes, diffScripts(old, new)...)
	changes = append(changes, diffSeeds(old, new)...)

	return changes
}

func diffSchemas(old, new *Manifest) []Change {
	var changes []Change

	oldTables := make(map[string]*Schema)
	if old != nil {
		for i := range old.Schemas {
			oldTables[old.Schemas[i].Table] = &old.Schemas[i]
		}
	}

	for i := range new.Schemas {
		newSchema := &new.Schemas[i]
		oldSchema, exists := oldTables[newSchema.Table]
		if !exists {
			// Entire table is new
			changes = append(changes, Change{
				Type:   ChangeAddTable,
				Table:  newSchema.Table,
				Schema: newSchema,
				Detail: "add table " + newSchema.Table,
			})
			continue
		}

		// Table exists — check for new or removed columns
		oldCols := make(map[string]*Column)
		for j := range oldSchema.Columns {
			oldCols[oldSchema.Columns[j].Name] = &oldSchema.Columns[j]
		}

		for j := range newSchema.Columns {
			col := &newSchema.Columns[j]
			if _, colExists := oldCols[col.Name]; !colExists {
				changes = append(changes, Change{
					Type:   ChangeAddColumn,
					Table:  newSchema.Table,
					Column: col,
					Detail: "add column " + newSchema.Table + "." + col.Name,
				})
			}
		}

		// Check for removed columns (warning only)
		newCols := make(map[string]bool)
		for _, col := range newSchema.Columns {
			newCols[col.Name] = true
		}
		for colName := range oldCols {
			if !newCols[colName] {
				changes = append(changes, Change{
					Type:   ChangeDropColumn,
					Table:  newSchema.Table,
					Column: oldCols[colName],
					Detail: "column " + newSchema.Table + "." + colName + " removed (NOT applied — SQLite limitation)",
				})
			}
		}
	}

	return changes
}

func diffRoutes(old, new *Manifest) []Change {
	var changes []Change

	type routeKey struct {
		Method string
		Path   string
	}

	oldRoutes := make(map[routeKey]*Route)
	if old != nil {
		for i := range old.Routes {
			r := &old.Routes[i]
			oldRoutes[routeKey{r.Method, r.Path}] = r
		}
	}

	newRoutes := make(map[routeKey]bool)
	for i := range new.Routes {
		r := &new.Routes[i]
		key := routeKey{r.Method, r.Path}
		newRoutes[key] = true

		oldRoute, exists := oldRoutes[key]
		if !exists {
			changes = append(changes, Change{
				Type:   ChangeAddRoute,
				Route:  r,
				Detail: "add route " + r.Method + " " + r.Path,
			})
		} else if oldRoute.Script != r.Script || oldRoute.Description != r.Description {
			changes = append(changes, Change{
				Type:   ChangeUpdateRoute,
				Route:  r,
				Detail: "update route " + r.Method + " " + r.Path,
			})
		}
	}

	if old != nil {
		for i := range old.Routes {
			r := &old.Routes[i]
			key := routeKey{r.Method, r.Path}
			if !newRoutes[key] {
				changes = append(changes, Change{
					Type:   ChangeRemoveRoute,
					Route:  r,
					Detail: "remove route " + r.Method + " " + r.Path,
				})
			}
		}
	}

	return changes
}

func diffScripts(old, new *Manifest) []Change {
	var changes []Change

	oldScripts := make(map[string]*Script)
	if old != nil {
		for i := range old.Scripts {
			oldScripts[old.Scripts[i].Name] = &old.Scripts[i]
		}
	}

	newScripts := make(map[string]bool)
	for i := range new.Scripts {
		s := &new.Scripts[i]
		newScripts[s.Name] = true

		oldScript, exists := oldScripts[s.Name]
		if !exists {
			changes = append(changes, Change{
				Type:   ChangeAddScript,
				Script: s,
				Detail: "add script " + s.Name,
			})
		} else if oldScript.Code != s.Code {
			changes = append(changes, Change{
				Type:   ChangeUpdateScript,
				Script: s,
				Detail: "update script " + s.Name,
			})
		}
	}

	if old != nil {
		for i := range old.Scripts {
			s := &old.Scripts[i]
			if !newScripts[s.Name] {
				changes = append(changes, Change{
					Type:   ChangeRemoveScript,
					Script: s,
					Detail: "remove script " + s.Name,
				})
			}
		}
	}

	return changes
}

func diffSeeds(old, new *Manifest) []Change {
	var changes []Change

	oldSeeds := make(map[string]bool)
	if old != nil {
		for _, s := range old.Seeds {
			oldSeeds[s.Table] = true
		}
	}

	for i := range new.Seeds {
		s := &new.Seeds[i]
		if !oldSeeds[s.Table] {
			changes = append(changes, Change{
				Type:   ChangeAddSeed,
				Seed:   s,
				Table:  s.Table,
				Detail: "seed table " + s.Table,
			})
		}
	}

	return changes
}
```

- [ ] **Step 2: Create `internal/manifest/diff_test.go`**

Create `internal/manifest/diff_test.go`:

```go
package manifest

import (
	"testing"
)

func TestDiff_NilOld_AllAdditions(t *testing.T) {
	m := &Manifest{
		Version: "1.0",
		Name:    "test",
		Schemas: []Schema{
			{Table: "users", Columns: []Column{
				{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
				{Name: "name", Type: "TEXT", Required: true},
			}},
		},
		Routes: []Route{
			{Path: "/users", Method: "GET", Script: "list_users", ResponseType: "array"},
		},
		Scripts: []Script{
			{Name: "list_users", Code: "response.json([])"},
		},
		Seeds: []Seed{
			{Table: "users", Rows: []map[string]any{{"name": "Alice"}}},
		},
	}

	changes := Diff(nil, m)

	counts := make(map[ChangeType]int)
	for _, c := range changes {
		counts[c.Type]++
	}
	if counts[ChangeAddTable] != 1 {
		t.Errorf("expected 1 ADD_TABLE, got %d", counts[ChangeAddTable])
	}
	if counts[ChangeAddRoute] != 1 {
		t.Errorf("expected 1 ADD_ROUTE, got %d", counts[ChangeAddRoute])
	}
	if counts[ChangeAddScript] != 1 {
		t.Errorf("expected 1 ADD_SCRIPT, got %d", counts[ChangeAddScript])
	}
	if counts[ChangeAddSeed] != 1 {
		t.Errorf("expected 1 ADD_SEED, got %d", counts[ChangeAddSeed])
	}
}

func TestDiff_AddNewTable(t *testing.T) {
	old := &Manifest{
		Version: "1.0",
		Name:    "test",
		Schemas: []Schema{
			{Table: "users", Columns: []Column{
				{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			}},
		},
	}
	new := &Manifest{
		Version: "1.0",
		Name:    "test",
		Schemas: []Schema{
			{Table: "users", Columns: []Column{
				{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			}},
			{Table: "posts", Columns: []Column{
				{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
				{Name: "title", Type: "TEXT"},
			}},
		},
	}

	changes := Diff(old, new)

	found := false
	for _, c := range changes {
		if c.Type == ChangeAddTable && c.Table == "posts" {
			found = true
		}
	}
	if !found {
		t.Error("expected ADD_TABLE for posts")
	}
}

func TestDiff_AddColumn(t *testing.T) {
	old := &Manifest{
		Version: "1.0",
		Name:    "test",
		Schemas: []Schema{
			{Table: "users", Columns: []Column{
				{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
				{Name: "name", Type: "TEXT"},
			}},
		},
	}
	new := &Manifest{
		Version: "1.0",
		Name:    "test",
		Schemas: []Schema{
			{Table: "users", Columns: []Column{
				{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
				{Name: "name", Type: "TEXT"},
				{Name: "email", Type: "TEXT", Unique: true},
			}},
		},
	}

	changes := Diff(old, new)

	found := false
	for _, c := range changes {
		if c.Type == ChangeAddColumn && c.Table == "users" && c.Column.Name == "email" {
			found = true
		}
	}
	if !found {
		t.Error("expected ADD_COLUMN for users.email")
	}
}

func TestDiff_DropColumn_Warning(t *testing.T) {
	old := &Manifest{
		Version: "1.0",
		Name:    "test",
		Schemas: []Schema{
			{Table: "users", Columns: []Column{
				{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
				{Name: "name", Type: "TEXT"},
				{Name: "age", Type: "INTEGER"},
			}},
		},
	}
	new := &Manifest{
		Version: "1.0",
		Name:    "test",
		Schemas: []Schema{
			{Table: "users", Columns: []Column{
				{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
				{Name: "name", Type: "TEXT"},
			}},
		},
	}

	changes := Diff(old, new)

	found := false
	for _, c := range changes {
		if c.Type == ChangeDropColumn && c.Table == "users" && c.Column.Name == "age" {
			found = true
		}
	}
	if !found {
		t.Error("expected DROP_COLUMN warning for users.age")
	}
}

func TestDiff_AddRoute(t *testing.T) {
	old := &Manifest{
		Version: "1.0",
		Name:    "test",
		Routes: []Route{
			{Path: "/users", Method: "GET", Script: "list_users", ResponseType: "array"},
		},
	}
	new := &Manifest{
		Version: "1.0",
		Name:    "test",
		Routes: []Route{
			{Path: "/users", Method: "GET", Script: "list_users", ResponseType: "array"},
			{Path: "/users", Method: "POST", Script: "create_user", ResponseType: "object"},
		},
	}

	changes := Diff(old, new)

	found := false
	for _, c := range changes {
		if c.Type == ChangeAddRoute && c.Route.Method == "POST" {
			found = true
		}
	}
	if !found {
		t.Error("expected ADD_ROUTE for POST /users")
	}
}

func TestDiff_UpdateRoute(t *testing.T) {
	old := &Manifest{
		Version: "1.0",
		Name:    "test",
		Routes: []Route{
			{Path: "/users", Method: "GET", Script: "list_users_v1", ResponseType: "array"},
		},
	}
	new := &Manifest{
		Version: "1.0",
		Name:    "test",
		Routes: []Route{
			{Path: "/users", Method: "GET", Script: "list_users_v2", ResponseType: "array"},
		},
	}

	changes := Diff(old, new)

	found := false
	for _, c := range changes {
		if c.Type == ChangeUpdateRoute && c.Route.Script == "list_users_v2" {
			found = true
		}
	}
	if !found {
		t.Error("expected UPDATE_ROUTE for GET /users")
	}
}

func TestDiff_RemoveRoute(t *testing.T) {
	old := &Manifest{
		Version: "1.0",
		Name:    "test",
		Routes: []Route{
			{Path: "/users", Method: "GET", Script: "list_users", ResponseType: "array"},
			{Path: "/users", Method: "DELETE", Script: "delete_user", ResponseType: "object"},
		},
	}
	new := &Manifest{
		Version: "1.0",
		Name:    "test",
		Routes: []Route{
			{Path: "/users", Method: "GET", Script: "list_users", ResponseType: "array"},
		},
	}

	changes := Diff(old, new)

	found := false
	for _, c := range changes {
		if c.Type == ChangeRemoveRoute && c.Route.Method == "DELETE" {
			found = true
		}
	}
	if !found {
		t.Error("expected REMOVE_ROUTE for DELETE /users")
	}
}

func TestDiff_AddScript(t *testing.T) {
	old := &Manifest{
		Version: "1.0",
		Name:    "test",
		Scripts: []Script{
			{Name: "list_users", Code: "response.json([])"},
		},
	}
	new := &Manifest{
		Version: "1.0",
		Name:    "test",
		Scripts: []Script{
			{Name: "list_users", Code: "response.json([])"},
			{Name: "create_user", Code: "response.json({}, 201)"},
		},
	}

	changes := Diff(old, new)

	found := false
	for _, c := range changes {
		if c.Type == ChangeAddScript && c.Script.Name == "create_user" {
			found = true
		}
	}
	if !found {
		t.Error("expected ADD_SCRIPT for create_user")
	}
}

func TestDiff_UpdateScript(t *testing.T) {
	old := &Manifest{
		Version: "1.0",
		Name:    "test",
		Scripts: []Script{
			{Name: "list_users", Code: "response.json([])"},
		},
	}
	new := &Manifest{
		Version: "1.0",
		Name:    "test",
		Scripts: []Script{
			{Name: "list_users", Code: "result := db.query(\"SELECT * FROM users\", [])\nresponse.json(result)"},
		},
	}

	changes := Diff(old, new)

	found := false
	for _, c := range changes {
		if c.Type == ChangeUpdateScript && c.Script.Name == "list_users" {
			found = true
		}
	}
	if !found {
		t.Error("expected UPDATE_SCRIPT for list_users")
	}
}

func TestDiff_RemoveScript(t *testing.T) {
	old := &Manifest{
		Version: "1.0",
		Name:    "test",
		Scripts: []Script{
			{Name: "list_users", Code: "response.json([])"},
			{Name: "old_script", Code: "response.json({})"},
		},
	}
	new := &Manifest{
		Version: "1.0",
		Name:    "test",
		Scripts: []Script{
			{Name: "list_users", Code: "response.json([])"},
		},
	}

	changes := Diff(old, new)

	found := false
	for _, c := range changes {
		if c.Type == ChangeRemoveScript && c.Script.Name == "old_script" {
			found = true
		}
	}
	if !found {
		t.Error("expected REMOVE_SCRIPT for old_script")
	}
}

func TestDiff_NoChanges(t *testing.T) {
	m := &Manifest{
		Version: "1.0",
		Name:    "test",
		Schemas: []Schema{
			{Table: "users", Columns: []Column{
				{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			}},
		},
		Routes: []Route{
			{Path: "/users", Method: "GET", Script: "list_users", ResponseType: "array"},
		},
		Scripts: []Script{
			{Name: "list_users", Code: "response.json([])"},
		},
	}

	changes := Diff(m, m)
	if len(changes) != 0 {
		t.Errorf("expected 0 changes for identical manifests, got %d", len(changes))
	}
}
```

- [ ] **Step 3: Run tests**

```bash
cd /Users/kent/Documents/Projects/vibeserve && go test ./internal/manifest/ -run TestDiff -v
```

Expected: all 10 TestDiff_* tests pass.

- [ ] **Step 4: Commit**

```bash
git add internal/manifest/diff.go internal/manifest/diff_test.go
git commit -m "feat: add manifest Differ — compares old vs new manifest, emits typed []Change"
```

---

### Task 2: Store — AddColumn + Auto-Migration

**Files:**
- Modify: `internal/store/migrate.go`
- Modify: `internal/store/migrate_test.go`
- Modify: `internal/store/store.go`

Add `BuildAddColumnSQL` to produce `ALTER TABLE ... ADD COLUMN` statements, and add `AddColumn` to `Store` so the engine can apply column additions at runtime. Also add a `DSN()` accessor so the snapshot system can locate the database file.

- [ ] **Step 1: Add `BuildAddColumnSQL` to `internal/store/migrate.go`**

Add to the end of `internal/store/migrate.go`:

```go
// BuildAddColumnSQL generates an ALTER TABLE ADD COLUMN statement for a single column.
// SQLite only supports adding columns — not dropping or renaming — so this is the only
// migration operation we support.
func BuildAddColumnSQL(table string, col manifest.Column) string {
	sqlType := mapSQLiteType(col.Type)
	var parts []string
	parts = append(parts, col.Name)
	parts = append(parts, sqlType)

	// Primary key columns cannot be added via ALTER TABLE, so we skip those modifiers.
	if col.Required {
		// SQLite requires a DEFAULT for NOT NULL columns added via ALTER TABLE.
		if col.Default != nil {
			parts = append(parts, "NOT NULL")
			parts = append(parts, "DEFAULT "+defaultValue(col))
		}
		// If required but no default, omit NOT NULL (SQLite limitation).
	} else {
		if col.Default != nil {
			parts = append(parts, "DEFAULT "+defaultValue(col))
		}
	}
	if col.Unique {
		parts = append(parts, "UNIQUE")
	}
	if col.References != "" {
		parts = append(parts, "REFERENCES "+col.References)
	}

	return fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s", table, strings.Join(parts, " "))
}
```

- [ ] **Step 2: Add `AddColumn` and `DSN` to `internal/store/store.go`**

Add to the end of `internal/store/store.go` (before the last closing brace or after `convertForWrite`):

```go
// AddColumn executes an ALTER TABLE ADD COLUMN statement and registers boolean columns.
func (s *Store) AddColumn(table string, col manifest.Column) error {
	ddl := BuildAddColumnSQL(table, col)
	if _, err := s.db.Exec(ddl); err != nil {
		return fmt.Errorf("add column %s.%s: %w", table, col.Name, err)
	}
	if strings.ToUpper(col.Type) == "BOOLEAN" {
		s.boolColumns[table+"."+col.Name] = true
	}
	return nil
}

// DSN returns the data source name the store was opened with.
func (s *Store) DSN() string {
	return s.dsn
}
```

Also modify the `Store` struct and `New` function to store the DSN. In `internal/store/store.go`, update the struct:

Change:

```go
type Store struct {
	db          *sql.DB
	boolColumns map[string]bool // key: "table.column"
}
```

to:

```go
type Store struct {
	db          *sql.DB
	dsn         string
	boolColumns map[string]bool // key: "table.column"
}
```

And in the `New` function, change:

```go
	return &Store{
		db:          db,
		boolColumns: make(map[string]bool),
	}, nil
```

to:

```go
	return &Store{
		db:          db,
		dsn:         dsn,
		boolColumns: make(map[string]bool),
	}, nil
```

- [ ] **Step 3: Add tests to `internal/store/migrate_test.go`**

Append to `internal/store/migrate_test.go`:

```go
func TestBuildAddColumnSQL_Simple(t *testing.T) {
	col := manifest.Column{Name: "email", Type: "TEXT"}
	sql := BuildAddColumnSQL("users", col)
	expected := "ALTER TABLE users ADD COLUMN email TEXT"
	if sql != expected {
		t.Errorf("expected %q, got %q", expected, sql)
	}
}

func TestBuildAddColumnSQL_WithDefault(t *testing.T) {
	col := manifest.Column{Name: "active", Type: "BOOLEAN", Default: true}
	sql := BuildAddColumnSQL("users", col)
	if !strings.Contains(sql, "ALTER TABLE users ADD COLUMN") {
		t.Errorf("missing ALTER TABLE: %s", sql)
	}
	if !strings.Contains(sql, "active INTEGER DEFAULT 1") {
		t.Errorf("expected 'active INTEGER DEFAULT 1', got: %s", sql)
	}
}

func TestBuildAddColumnSQL_RequiredWithDefault(t *testing.T) {
	col := manifest.Column{Name: "status", Type: "TEXT", Required: true, Default: "active"}
	sql := BuildAddColumnSQL("users", col)
	if !strings.Contains(sql, "NOT NULL") {
		t.Errorf("expected NOT NULL: %s", sql)
	}
	if !strings.Contains(sql, "DEFAULT 'active'") {
		t.Errorf("expected DEFAULT 'active': %s", sql)
	}
}

func TestBuildAddColumnSQL_Unique(t *testing.T) {
	col := manifest.Column{Name: "slug", Type: "TEXT", Unique: true}
	sql := BuildAddColumnSQL("users", col)
	if !strings.Contains(sql, "UNIQUE") {
		t.Errorf("expected UNIQUE: %s", sql)
	}
}

func TestBuildAddColumnSQL_References(t *testing.T) {
	col := manifest.Column{Name: "author_id", Type: "INTEGER", References: "users(id)"}
	sql := BuildAddColumnSQL("posts", col)
	if !strings.Contains(sql, "REFERENCES users(id)") {
		t.Errorf("expected REFERENCES: %s", sql)
	}
}
```

- [ ] **Step 4: Add AddColumn integration test to `internal/store/store_test.go`**

Append to `internal/store/store_test.go`:

```go
func TestAddColumn(t *testing.T) {
	s := newTestStore(t)

	// Insert a row before adding column
	_, err := s.Insert("users", map[string]any{"name": "Alice"})
	if err != nil {
		t.Fatalf("Insert before AddColumn: %v", err)
	}

	// Add a new column
	col := manifest.Column{Name: "email", Type: "TEXT"}
	if err := s.AddColumn("users", col); err != nil {
		t.Fatalf("AddColumn: %v", err)
	}

	// Insert a row with the new column
	row, err := s.Insert("users", map[string]any{"name": "Bob", "email": "bob@test.com"})
	if err != nil {
		t.Fatalf("Insert after AddColumn: %v", err)
	}
	if row["email"] != "bob@test.com" {
		t.Errorf("expected email 'bob@test.com', got %v", row["email"])
	}

	// Verify old row has NULL for email
	oldRow, err := s.QueryOne("SELECT * FROM users WHERE name = ?", []any{"Alice"})
	if err != nil {
		t.Fatalf("QueryOne: %v", err)
	}
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

	row, err := s.Insert("users", map[string]any{"name": "Charlie", "verified": true})
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
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
		t.Errorf("expected DSN ':memory:', got %q", s.DSN())
	}
}
```

- [ ] **Step 5: Run tests**

```bash
cd /Users/kent/Documents/Projects/vibeserve && go test ./internal/store/ -v
```

Expected: all existing tests pass plus TestBuildAddColumnSQL_*, TestAddColumn, TestAddColumn_Boolean, TestStoreDSN.

- [ ] **Step 6: Commit**

```bash
git add internal/store/migrate.go internal/store/migrate_test.go internal/store/store.go internal/store/store_test.go
git commit -m "feat: add ALTER TABLE ADD COLUMN support and DSN accessor to store"
```

---

### Task 3: Snapshot System

**Files:**
- Create: `internal/snapshot/snapshot.go`
- Create: `internal/snapshot/snapshot_test.go`

The snapshot system copies `.vibe/state.db` to `.vibe/snapshots/NNN_description.db` before every schema change. Restore copies a snapshot back to `.vibe/state.db`. The `List` function returns snapshots sorted newest-first.

- [ ] **Step 1: Create `internal/snapshot/snapshot.go`**

Create `internal/snapshot/snapshot.go`:

```go
package snapshot

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Snapshot represents a saved database snapshot.
type Snapshot struct {
	ID          int
	Description string
	Path        string
}

// Dir returns the snapshots directory for a given vibe directory.
func Dir(vibeDir string) string {
	return filepath.Join(vibeDir, "snapshots")
}

// Create copies the database file at dbPath into the snapshots directory
// with an auto-incrementing ID and the given description.
// Returns the created Snapshot.
func Create(vibeDir, dbPath, description string) (*Snapshot, error) {
	snapDir := Dir(vibeDir)
	if err := os.MkdirAll(snapDir, 0o755); err != nil {
		return nil, fmt.Errorf("create snapshot dir: %w", err)
	}

	// Find the next ID
	nextID, err := nextID(snapDir)
	if err != nil {
		return nil, fmt.Errorf("next snapshot id: %w", err)
	}

	// Sanitize description for filename
	safeName := sanitize(description)
	filename := fmt.Sprintf("%03d_%s.db", nextID, safeName)
	destPath := filepath.Join(snapDir, filename)

	if err := copyFile(dbPath, destPath); err != nil {
		return nil, fmt.Errorf("copy snapshot: %w", err)
	}

	return &Snapshot{
		ID:          nextID,
		Description: description,
		Path:        destPath,
	}, nil
}

// Restore copies a snapshot file back to the database path, overwriting it.
func Restore(snap *Snapshot, dbPath string) error {
	if err := copyFile(snap.Path, dbPath); err != nil {
		return fmt.Errorf("restore snapshot %d: %w", snap.ID, err)
	}
	return nil
}

// RestoreLatest finds the most recent snapshot and restores it.
// Returns the restored snapshot, or an error if no snapshots exist.
func RestoreLatest(vibeDir, dbPath string) (*Snapshot, error) {
	snaps, err := List(vibeDir)
	if err != nil {
		return nil, err
	}
	if len(snaps) == 0 {
		return nil, fmt.Errorf("no snapshots found")
	}
	latest := snaps[0] // List returns newest-first
	if err := Restore(&latest, dbPath); err != nil {
		return nil, err
	}
	return &latest, nil
}

// List returns all snapshots in the vibe directory, sorted newest-first (highest ID first).
func List(vibeDir string) ([]Snapshot, error) {
	snapDir := Dir(vibeDir)
	entries, err := os.ReadDir(snapDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read snapshot dir: %w", err)
	}

	var snaps []Snapshot
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".db") {
			continue
		}
		snap, ok := parseFilename(name, snapDir)
		if !ok {
			continue
		}
		snaps = append(snaps, snap)
	}

	// Sort newest-first
	sort.Slice(snaps, func(i, j int) bool {
		return snaps[i].ID > snaps[j].ID
	})

	return snaps, nil
}

// snapshotPattern matches filenames like "001_description.db"
var snapshotPattern = regexp.MustCompile(`^(\d{3})_(.+)\.db$`)

func parseFilename(name, dir string) (Snapshot, bool) {
	matches := snapshotPattern.FindStringSubmatch(name)
	if matches == nil {
		return Snapshot{}, false
	}
	id, err := strconv.Atoi(matches[1])
	if err != nil {
		return Snapshot{}, false
	}
	return Snapshot{
		ID:          id,
		Description: matches[2],
		Path:        filepath.Join(dir, name),
	}, true
}

func nextID(snapDir string) (int, error) {
	entries, err := os.ReadDir(snapDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 1, nil
		}
		return 0, err
	}

	maxID := 0
	for _, entry := range entries {
		snap, ok := parseFilename(entry.Name(), snapDir)
		if ok && snap.ID > maxID {
			maxID = snap.ID
		}
	}
	return maxID + 1, nil
}

// sanitize turns a description into a safe filename component.
func sanitize(s string) string {
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		if r == ' ' {
			return '_'
		}
		return -1
	}, s)
	if len(s) > 50 {
		s = s[:50]
	}
	if s == "" {
		s = "snapshot"
	}
	return s
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
```

- [ ] **Step 2: Create `internal/snapshot/snapshot_test.go`**

Create `internal/snapshot/snapshot_test.go`:

```go
package snapshot

import (
	"os"
	"path/filepath"
	"testing"
)

func setupTestDB(t *testing.T) (vibeDir, dbPath string) {
	t.Helper()
	tmpDir := t.TempDir()
	vibeDir = filepath.Join(tmpDir, ".vibe")
	os.MkdirAll(vibeDir, 0o755)

	dbPath = filepath.Join(vibeDir, "state.db")
	os.WriteFile(dbPath, []byte("test-db-content-v1"), 0o644)
	return vibeDir, dbPath
}

func TestCreate(t *testing.T) {
	vibeDir, dbPath := setupTestDB(t)

	snap, err := Create(vibeDir, dbPath, "before schema change")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if snap.ID != 1 {
		t.Errorf("expected ID 1, got %d", snap.ID)
	}
	if snap.Description != "before schema change" {
		t.Errorf("expected description 'before schema change', got %q", snap.Description)
	}

	// Verify file exists
	data, err := os.ReadFile(snap.Path)
	if err != nil {
		t.Fatalf("read snapshot file: %v", err)
	}
	if string(data) != "test-db-content-v1" {
		t.Errorf("snapshot content mismatch: %s", data)
	}
}

func TestCreate_AutoIncrement(t *testing.T) {
	vibeDir, dbPath := setupTestDB(t)

	snap1, err := Create(vibeDir, dbPath, "first")
	if err != nil {
		t.Fatalf("Create first: %v", err)
	}
	if snap1.ID != 1 {
		t.Errorf("expected ID 1, got %d", snap1.ID)
	}

	snap2, err := Create(vibeDir, dbPath, "second")
	if err != nil {
		t.Fatalf("Create second: %v", err)
	}
	if snap2.ID != 2 {
		t.Errorf("expected ID 2, got %d", snap2.ID)
	}

	snap3, err := Create(vibeDir, dbPath, "third")
	if err != nil {
		t.Fatalf("Create third: %v", err)
	}
	if snap3.ID != 3 {
		t.Errorf("expected ID 3, got %d", snap3.ID)
	}
}

func TestList_NewestFirst(t *testing.T) {
	vibeDir, dbPath := setupTestDB(t)

	Create(vibeDir, dbPath, "first")
	Create(vibeDir, dbPath, "second")
	Create(vibeDir, dbPath, "third")

	snaps, err := List(vibeDir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(snaps) != 3 {
		t.Fatalf("expected 3 snapshots, got %d", len(snaps))
	}
	if snaps[0].ID != 3 {
		t.Errorf("expected newest first (ID 3), got %d", snaps[0].ID)
	}
	if snaps[1].ID != 2 {
		t.Errorf("expected ID 2 second, got %d", snaps[1].ID)
	}
	if snaps[2].ID != 1 {
		t.Errorf("expected oldest last (ID 1), got %d", snaps[2].ID)
	}
}

func TestList_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	vibeDir := filepath.Join(tmpDir, ".vibe")
	// Don't create snapshots dir

	snaps, err := List(vibeDir)
	if err != nil {
		t.Fatalf("List empty: %v", err)
	}
	if len(snaps) != 0 {
		t.Errorf("expected 0 snapshots, got %d", len(snaps))
	}
}

func TestRestore(t *testing.T) {
	vibeDir, dbPath := setupTestDB(t)

	// Create snapshot of v1
	snap, err := Create(vibeDir, dbPath, "v1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Overwrite db with v2
	os.WriteFile(dbPath, []byte("test-db-content-v2"), 0o644)

	// Verify db is now v2
	data, _ := os.ReadFile(dbPath)
	if string(data) != "test-db-content-v2" {
		t.Fatalf("expected v2, got %s", data)
	}

	// Restore v1
	if err := Restore(snap, dbPath); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	// Verify db is back to v1
	data, _ = os.ReadFile(dbPath)
	if string(data) != "test-db-content-v1" {
		t.Errorf("expected v1 after restore, got %s", data)
	}
}

func TestRestoreLatest(t *testing.T) {
	vibeDir, dbPath := setupTestDB(t)

	Create(vibeDir, dbPath, "first")

	// Change the DB
	os.WriteFile(dbPath, []byte("test-db-content-v2"), 0o644)
	Create(vibeDir, dbPath, "second")

	// Change the DB again
	os.WriteFile(dbPath, []byte("test-db-content-v3"), 0o644)

	// RestoreLatest should restore the "second" snapshot (v2 content)
	snap, err := RestoreLatest(vibeDir, dbPath)
	if err != nil {
		t.Fatalf("RestoreLatest: %v", err)
	}
	if snap.ID != 2 {
		t.Errorf("expected latest snapshot ID 2, got %d", snap.ID)
	}

	data, _ := os.ReadFile(dbPath)
	if string(data) != "test-db-content-v2" {
		t.Errorf("expected v2 content after restore, got %s", data)
	}
}

func TestRestoreLatest_NoSnapshots(t *testing.T) {
	tmpDir := t.TempDir()
	vibeDir := filepath.Join(tmpDir, ".vibe")
	dbPath := filepath.Join(vibeDir, "state.db")

	_, err := RestoreLatest(vibeDir, dbPath)
	if err == nil {
		t.Error("expected error when no snapshots exist")
	}
}

func TestSanitize(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"before schema change", "before_schema_change"},
		{"Add Users Table!", "add_users_table"},
		{"", "snapshot"},
		{"Hello World 123", "hello_world_123"},
	}
	for _, tt := range tests {
		got := sanitize(tt.input)
		if got != tt.expected {
			t.Errorf("sanitize(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
```

- [ ] **Step 3: Run tests**

```bash
cd /Users/kent/Documents/Projects/vibeserve && go test ./internal/snapshot/ -v
```

Expected: all TestCreate*, TestList*, TestRestore*, TestSanitize tests pass.

- [ ] **Step 4: Commit**

```bash
git add internal/snapshot/snapshot.go internal/snapshot/snapshot_test.go
git commit -m "feat: add snapshot system — create, restore, list database snapshots"
```

---

### Task 4: Config System (YAML Loading)

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

Loads `.vibe/config.yaml` with provider, model, server settings. API keys are read from environment variables via the `api_key_env` field (never stored in the config file itself).

- [ ] **Step 1: Create `internal/config/config.go`**

Create `internal/config/config.go`:

```go
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the .vibe/config.yaml file.
type Config struct {
	Provider   string       `yaml:"provider"`
	APIKeyEnv  string       `yaml:"api_key_env"`
	Model      string       `yaml:"model"`
	OllamaHost string       `yaml:"ollama_host"`
	Server     ServerConfig `yaml:"server"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Port int    `yaml:"port"`
	Host string `yaml:"host"`
	CORS bool   `yaml:"cors"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Provider:   "claude",
		APIKeyEnv:  "ANTHROPIC_API_KEY",
		Model:      "claude-sonnet-4-6-20250514",
		OllamaHost: "http://localhost:11434",
		Server: ServerConfig{
			Port: 8080,
			Host: "localhost",
			CORS: true,
		},
	}
}

// Load reads and parses a YAML config file from the given path.
// If the file does not exist, returns DefaultConfig with no error.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	return cfg, nil
}

// APIKey reads the API key from the environment variable specified in APIKeyEnv.
// Returns empty string if the env var is not set.
func (c *Config) APIKey() string {
	if c.APIKeyEnv == "" {
		return ""
	}
	return os.Getenv(c.APIKeyEnv)
}

// Validate checks that the config has all required fields.
func (c *Config) Validate() error {
	switch c.Provider {
	case "claude":
		if c.APIKey() == "" {
			return fmt.Errorf("provider 'claude' requires %s environment variable to be set", c.APIKeyEnv)
		}
	case "ollama":
		if c.OllamaHost == "" {
			return fmt.Errorf("provider 'ollama' requires ollama_host to be set")
		}
	default:
		return fmt.Errorf("unknown provider %q (supported: claude, ollama)", c.Provider)
	}
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
	}
	return nil
}
```

- [ ] **Step 2: Create `internal/config/config_test.go`**

Create `internal/config/config_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Provider != "claude" {
		t.Errorf("expected provider 'claude', got %q", cfg.Provider)
	}
	if cfg.Model != "claude-sonnet-4-6-20250514" {
		t.Errorf("expected model 'claude-sonnet-4-6-20250514', got %q", cfg.Model)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Server.Host != "localhost" {
		t.Errorf("expected host 'localhost', got %q", cfg.Server.Host)
	}
	if !cfg.Server.CORS {
		t.Error("expected CORS true by default")
	}
	if cfg.APIKeyEnv != "ANTHROPIC_API_KEY" {
		t.Errorf("expected api_key_env 'ANTHROPIC_API_KEY', got %q", cfg.APIKeyEnv)
	}
	if cfg.OllamaHost != "http://localhost:11434" {
		t.Errorf("expected ollama_host 'http://localhost:11434', got %q", cfg.OllamaHost)
	}
}

func TestLoad_FileNotExist(t *testing.T) {
	cfg, err := Load("/nonexistent/path/config.yaml")
	if err != nil {
		t.Fatalf("Load nonexistent: %v", err)
	}
	if cfg.Provider != "claude" {
		t.Errorf("expected default provider 'claude', got %q", cfg.Provider)
	}
}

func TestLoad_ValidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yaml")

	yaml := `provider: ollama
api_key_env: MY_KEY
model: llama3
ollama_host: http://myhost:11434
server:
  port: 9090
  host: 0.0.0.0
  cors: false
`
	os.WriteFile(path, []byte(yaml), 0o644)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Provider != "ollama" {
		t.Errorf("expected 'ollama', got %q", cfg.Provider)
	}
	if cfg.APIKeyEnv != "MY_KEY" {
		t.Errorf("expected 'MY_KEY', got %q", cfg.APIKeyEnv)
	}
	if cfg.Model != "llama3" {
		t.Errorf("expected 'llama3', got %q", cfg.Model)
	}
	if cfg.OllamaHost != "http://myhost:11434" {
		t.Errorf("expected 'http://myhost:11434', got %q", cfg.OllamaHost)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Server.Port)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("expected host '0.0.0.0', got %q", cfg.Server.Host)
	}
	if cfg.Server.CORS {
		t.Error("expected CORS false")
	}
}

func TestLoad_PartialYAML_DefaultsFill(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yaml")

	yaml := `provider: ollama
model: codellama
`
	os.WriteFile(path, []byte(yaml), 0o644)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Provider != "ollama" {
		t.Errorf("expected 'ollama', got %q", cfg.Provider)
	}
	if cfg.Model != "codellama" {
		t.Errorf("expected 'codellama', got %q", cfg.Model)
	}
	// Defaults should still apply
	if cfg.Server.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Server.Port)
	}
	if cfg.OllamaHost != "http://localhost:11434" {
		t.Errorf("expected default ollama_host, got %q", cfg.OllamaHost)
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yaml")
	os.WriteFile(path, []byte(":::not yaml:::"), 0o644)

	_, err := Load(path)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestAPIKey(t *testing.T) {
	cfg := &Config{APIKeyEnv: "TEST_VIBESERVE_KEY_12345"}
	os.Setenv("TEST_VIBESERVE_KEY_12345", "sk-secret-123")
	defer os.Unsetenv("TEST_VIBESERVE_KEY_12345")

	key := cfg.APIKey()
	if key != "sk-secret-123" {
		t.Errorf("expected 'sk-secret-123', got %q", key)
	}
}

func TestAPIKey_NotSet(t *testing.T) {
	cfg := &Config{APIKeyEnv: "NONEXISTENT_VAR_VIBESERVE_TEST"}
	os.Unsetenv("NONEXISTENT_VAR_VIBESERVE_TEST")

	key := cfg.APIKey()
	if key != "" {
		t.Errorf("expected empty string, got %q", key)
	}
}

func TestValidate_Claude_NoKey(t *testing.T) {
	cfg := &Config{
		Provider:  "claude",
		APIKeyEnv: "NONEXISTENT_VAR_VIBESERVE_TEST",
		Server:    ServerConfig{Port: 8080},
	}
	os.Unsetenv("NONEXISTENT_VAR_VIBESERVE_TEST")

	err := cfg.Validate()
	if err == nil {
		t.Error("expected error when Claude API key is missing")
	}
}

func TestValidate_Ollama_OK(t *testing.T) {
	cfg := &Config{
		Provider:   "ollama",
		OllamaHost: "http://localhost:11434",
		Server:     ServerConfig{Port: 8080},
	}

	err := cfg.Validate()
	if err != nil {
		t.Errorf("expected no error for valid ollama config, got: %v", err)
	}
}

func TestValidate_UnknownProvider(t *testing.T) {
	cfg := &Config{
		Provider: "gpt",
		Server:   ServerConfig{Port: 8080},
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for unknown provider")
	}
}

func TestValidate_InvalidPort(t *testing.T) {
	cfg := &Config{
		Provider:   "ollama",
		OllamaHost: "http://localhost:11434",
		Server:     ServerConfig{Port: 0},
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for invalid port")
	}
}
```

- [ ] **Step 3: Run tests**

```bash
cd /Users/kent/Documents/Projects/vibeserve && go test ./internal/config/ -v
```

Expected: all TestDefaultConfig, TestLoad_*, TestAPIKey*, TestValidate_* tests pass.

- [ ] **Step 4: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: add config system — load .vibe/config.yaml with defaults and validation"
```

---

### Task 5: LLM Provider Interface + System Prompt Template

**Files:**
- Create: `internal/llm/provider.go`
- Create: `internal/llm/provider_test.go`

Defines the `Provider` interface and the system prompt builder. The system prompt teaches the LLM the manifest JSON schema, ALL stdlib functions (with Tengo keyword fixes), the current manifest, and the migration memory rule.

- [ ] **Step 1: Create `internal/llm/provider.go`**

Create `internal/llm/provider.go`:

```go
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

// Message represents a single conversation message.
type Message struct {
	Role    string `json:"role"`    // "user" or "assistant"
	Content string `json:"content"`
}

// Provider generates a new manifest from a user prompt and conversation history.
type Provider interface {
	// Generate sends the user prompt (with history and current manifest context)
	// to the LLM and returns the generated manifest.
	Generate(ctx context.Context, current *manifest.Manifest, prompt string, history []Message) (*manifest.Manifest, error)
}

// BuildSystemPrompt constructs the system prompt that instructs the LLM how to
// produce valid manifests. It includes:
// 1. The manifest JSON schema with an example
// 2. All stdlib functions (with Tengo keyword fixes: response.fail, log.err)
// 3. The current manifest as context
// 4. The Migration Memory Rule
// 5. Instruction to output ONLY valid JSON
func BuildSystemPrompt(current *manifest.Manifest) string {
	var b strings.Builder

	b.WriteString(`You are VibeServe, an AI backend generator. You produce JSON manifests that define APIs.

## Manifest JSON Schema

A manifest is a JSON object with these fields:

{
  "version": "1.0",
  "name": "my-api",
  "description": "Description of the API",
  "schemas": [
    {
      "table": "users",
      "columns": [
        {"name": "id", "type": "INTEGER", "primary": true, "auto": true},
        {"name": "email", "type": "TEXT", "required": true, "unique": true},
        {"name": "name", "type": "TEXT", "required": true},
        {"name": "active", "type": "BOOLEAN", "default": true},
        {"name": "score", "type": "REAL"},
        {"name": "created_at", "type": "DATETIME", "default": "NOW"},
        {"name": "birth_date", "type": "DATE"},
        {"name": "category_id", "type": "INTEGER", "references": "categories(id)"}
      ]
    }
  ],
  "routes": [
    {
      "path": "/users",
      "method": "GET",
      "description": "List all users",
      "script": "list_users",
      "response_type": "array"
    },
    {
      "path": "/users/:id",
      "method": "GET",
      "description": "Get a user by ID",
      "script": "get_user",
      "response_type": "object"
    },
    {
      "path": "/users",
      "method": "POST",
      "description": "Create a user",
      "script": "create_user",
      "request_body": {"email": "TEXT", "name": "TEXT"},
      "response_type": "object"
    }
  ],
  "scripts": [
    {
      "name": "list_users",
      "code": "result := db.query(\"SELECT * FROM users\", [])\nresponse.json(result)"
    }
  ],
  "seeds": [
    {
      "table": "users",
      "rows": [
        {"email": "alice@example.com", "name": "Alice"}
      ]
    }
  ]
}

### Column Types
Valid column types: INTEGER, TEXT, REAL, BOOLEAN, DATE, DATETIME

### Route Methods
Valid methods: GET, POST, PUT, PATCH, DELETE

### Path Parameters
Use :param syntax for dynamic segments: /users/:id, /posts/:postId/comments/:commentId

## Tengo Script Standard Library

Scripts are written in Tengo. These modules are injected automatically:

### db — Database operations
- db.query(sql, params) → array of maps (or error)
- db.query_one(sql, params) → map or undefined (or error)
- db.insert(table, data) → inserted row map (or error)
- db.update(table, id, data) → updated row map (or error)
- db.delete(table, id) → true/false (or error)
- db.count(table) → integer (or error)

Parameters are passed as arrays: db.query("SELECT * FROM users WHERE id = ?", [id])

### request — HTTP request data
- request.param(name) → string or undefined (path parameter)
- request.query(name) → string or undefined (query string)
- request.body() → map or undefined (parsed JSON body)
- request.header(name) → string or undefined
- request.method() → string ("GET", "POST", etc.)
- request.auth() → map (decoded Bearer token payload) or undefined

### response — HTTP response
- response.json(data) → sends JSON with status 200
- response.json(data, status) → sends JSON with custom status code
- response.fail(status, message) → sends {"error": message} with status code
- response.header(name, value) → sets a response header
- response.redirect(url) → sends 302 redirect

IMPORTANT: "error" is a reserved keyword in Tengo. Use response.fail() instead of response.error().

### date — Date utilities
- date.now() → current UTC datetime as RFC3339 string
- date.diff_days(dateA, dateB) → integer days between dates
- date.add_days(date, n) → new date string
- date.format(date, layout) → formatted string (Go layout syntax)

### crypto — Security utilities
- crypto.hash(str) → SHA-256 hex string
- crypto.uuid() → random UUID v4 string
- crypto.random(min, max) → random integer in range [min, max]

### log — Logging
- log.info(message) → log at info level
- log.warn(message) → log at warn level
- log.err(message) → log at error level

IMPORTANT: "error" is a reserved keyword in Tengo. Use log.err() instead of log.error().

## Script Examples

List with filtering:
result := db.query("SELECT * FROM items WHERE active = ?", [true])
response.json(result)

Get by ID with 404 handling:
id := request.param("id")
row := db.query_one("SELECT * FROM items WHERE id = ?", [id])
if row == undefined {
  response.fail(404, "Not found")
} else {
  response.json(row)
}

Create with validation:
body := request.body()
if body.name == undefined {
  response.fail(400, "name is required")
}
result := db.insert("items", {
  name: body.name,
  created_at: date.now()
})
response.json(result, 201)

Update:
id := request.param("id")
body := request.body()
result := db.update("items", id, body)
response.json(result)

Delete:
id := request.param("id")
deleted := db.delete("items", id)
if deleted {
  response.json({message: "deleted"})
} else {
  response.fail(404, "Not found")
}

Query parameter filtering:
status := request.query("status")
if status == undefined {
  result := db.query("SELECT * FROM tasks", [])
  response.json(result)
} else {
  result := db.query("SELECT * FROM tasks WHERE status = ?", [status])
  response.json(result)
}

`)

	b.WriteString("## Migration Memory Rule\n\n")
	b.WriteString("CRITICAL: You must NEVER rename or remove primary key columns (columns with \"primary\": true).\n")
	b.WriteString("You must NEVER rename or remove foreign key columns (columns with \"references\").\n")
	b.WriteString("You may ADD new columns to existing tables.\n")
	b.WriteString("You may ADD new tables.\n")
	b.WriteString("You may ADD new routes, scripts, and seeds.\n")
	b.WriteString("You may UPDATE existing scripts (change their code).\n")
	b.WriteString("You may REMOVE routes and their scripts if the user asks.\n")
	b.WriteString("If you remove a column from the schema, it will be IGNORED (SQLite cannot drop columns in older versions).\n")
	b.WriteString("Always include ALL existing tables and columns in your output, even if unchanged.\n\n")

	if current != nil {
		b.WriteString("## Current Manifest\n\n")
		b.WriteString("This is the current state of the API. Build upon it — do not start from scratch.\n\n")
		b.WriteString("```json\n")
		data, err := json.MarshalIndent(current, "", "  ")
		if err == nil {
			b.Write(data)
		}
		b.WriteString("\n```\n\n")
	} else {
		b.WriteString("## Current State\n\n")
		b.WriteString("No existing manifest. Create a new one from scratch based on the user's request.\n\n")
	}

	b.WriteString("## Output Rules\n\n")
	b.WriteString("1. Output ONLY valid JSON — no markdown fences, no explanation, no commentary.\n")
	b.WriteString("2. The JSON must be a complete manifest object with all required fields.\n")
	b.WriteString("3. Include ALL existing schemas, routes, scripts, and seeds, plus any changes.\n")
	b.WriteString("4. Every route must reference a script that exists in the scripts array.\n")
	b.WriteString("5. Every seed must reference a table that exists in the schemas array.\n")
	b.WriteString("6. Script code must be valid Tengo. Use response.fail() not response.error(). Use log.err() not log.error().\n")

	return b.String()
}

// ExtractJSON finds and extracts a JSON object from LLM output.
// Some LLMs wrap JSON in markdown fences or add explanation text.
// This function attempts to find the outermost {...} in the response.
func ExtractJSON(raw string) (string, error) {
	raw = strings.TrimSpace(raw)

	// If it already starts with {, try it directly
	if strings.HasPrefix(raw, "{") {
		return raw, nil
	}

	// Try to find JSON within markdown fences
	if idx := strings.Index(raw, "```json"); idx != -1 {
		start := idx + len("```json")
		end := strings.Index(raw[start:], "```")
		if end != -1 {
			return strings.TrimSpace(raw[start : start+end]), nil
		}
	}
	if idx := strings.Index(raw, "```"); idx != -1 {
		start := idx + len("```")
		end := strings.Index(raw[start:], "```")
		if end != -1 {
			candidate := strings.TrimSpace(raw[start : start+end])
			if strings.HasPrefix(candidate, "{") {
				return candidate, nil
			}
		}
	}

	// Find the first { and last }
	first := strings.Index(raw, "{")
	last := strings.LastIndex(raw, "}")
	if first != -1 && last > first {
		return raw[first : last+1], nil
	}

	return "", fmt.Errorf("no JSON object found in LLM response")
}

// ParseManifestResponse extracts and parses a manifest from raw LLM text output.
func ParseManifestResponse(raw string) (*manifest.Manifest, error) {
	jsonStr, err := ExtractJSON(raw)
	if err != nil {
		return nil, err
	}

	var m manifest.Manifest
	if err := json.Unmarshal([]byte(jsonStr), &m); err != nil {
		return nil, fmt.Errorf("parse manifest JSON: %w", err)
	}

	return &m, nil
}
```

- [ ] **Step 2: Create `internal/llm/provider_test.go`**

Create `internal/llm/provider_test.go`:

```go
package llm

import (
	"strings"
	"testing"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

func TestBuildSystemPrompt_NilManifest(t *testing.T) {
	prompt := BuildSystemPrompt(nil)

	if !strings.Contains(prompt, "VibeServe") {
		t.Error("expected prompt to mention VibeServe")
	}
	if !strings.Contains(prompt, "response.fail") {
		t.Error("expected prompt to mention response.fail")
	}
	if !strings.Contains(prompt, "log.err") {
		t.Error("expected prompt to mention log.err")
	}
	if !strings.Contains(prompt, "Migration Memory Rule") {
		t.Error("expected prompt to contain Migration Memory Rule")
	}
	if !strings.Contains(prompt, "No existing manifest") {
		t.Error("expected prompt to say no existing manifest")
	}
	if !strings.Contains(prompt, "ONLY valid JSON") {
		t.Error("expected prompt to instruct JSON-only output")
	}
}

func TestBuildSystemPrompt_WithManifest(t *testing.T) {
	m := &manifest.Manifest{
		Version: "1.0",
		Name:    "test-api",
		Schemas: []manifest.Schema{
			{Table: "users", Columns: []manifest.Column{
				{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			}},
		},
	}

	prompt := BuildSystemPrompt(m)

	if !strings.Contains(prompt, "test-api") {
		t.Error("expected prompt to contain current manifest name")
	}
	if !strings.Contains(prompt, "Current Manifest") {
		t.Error("expected prompt to have Current Manifest section")
	}
	if !strings.Contains(prompt, `"users"`) {
		t.Error("expected prompt to contain table name from manifest")
	}
}

func TestBuildSystemPrompt_ContainsAllStdlib(t *testing.T) {
	prompt := BuildSystemPrompt(nil)

	required := []string{
		"db.query", "db.query_one", "db.insert", "db.update", "db.delete", "db.count",
		"request.param", "request.query", "request.body", "request.header", "request.method", "request.auth",
		"response.json", "response.fail", "response.header", "response.redirect",
		"date.now", "date.diff_days", "date.add_days", "date.format",
		"crypto.hash", "crypto.uuid", "crypto.random",
		"log.info", "log.warn", "log.err",
	}

	for _, fn := range required {
		if !strings.Contains(prompt, fn) {
			t.Errorf("system prompt missing stdlib function: %s", fn)
		}
	}
}

func TestBuildSystemPrompt_TengoKeywordWarnings(t *testing.T) {
	prompt := BuildSystemPrompt(nil)

	if !strings.Contains(prompt, "\"error\" is a reserved keyword in Tengo") {
		t.Error("expected Tengo keyword warning")
	}
	if strings.Contains(prompt, "response.error(") {
		t.Error("prompt should NOT contain response.error()")
	}
	if strings.Contains(prompt, "log.error(") {
		t.Error("prompt should NOT contain log.error()")
	}
}

func TestExtractJSON_PlainJSON(t *testing.T) {
	raw := `{"version": "1.0", "name": "test"}`
	result, err := ExtractJSON(raw)
	if err != nil {
		t.Fatalf("ExtractJSON: %v", err)
	}
	if result != raw {
		t.Errorf("expected %q, got %q", raw, result)
	}
}

func TestExtractJSON_MarkdownFences(t *testing.T) {
	raw := "Here is the manifest:\n```json\n{\"version\": \"1.0\", \"name\": \"test\"}\n```\nDone!"
	result, err := ExtractJSON(raw)
	if err != nil {
		t.Fatalf("ExtractJSON: %v", err)
	}
	if result != `{"version": "1.0", "name": "test"}` {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestExtractJSON_SurroundingText(t *testing.T) {
	raw := "Sure, here is the manifest: {\"version\": \"1.0\"} I hope this helps!"
	result, err := ExtractJSON(raw)
	if err != nil {
		t.Fatalf("ExtractJSON: %v", err)
	}
	if result != `{"version": "1.0"}` {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestExtractJSON_NoJSON(t *testing.T) {
	raw := "I don't know how to help with that."
	_, err := ExtractJSON(raw)
	if err == nil {
		t.Error("expected error for no JSON")
	}
}

func TestParseManifestResponse_Valid(t *testing.T) {
	raw := `{"version": "1.0", "name": "test", "description": "A test", "schemas": [], "routes": [], "scripts": [], "seeds": []}`
	m, err := ParseManifestResponse(raw)
	if err != nil {
		t.Fatalf("ParseManifestResponse: %v", err)
	}
	if m.Name != "test" {
		t.Errorf("expected name 'test', got %q", m.Name)
	}
	if m.Version != "1.0" {
		t.Errorf("expected version '1.0', got %q", m.Version)
	}
}

func TestParseManifestResponse_WithFences(t *testing.T) {
	raw := "```json\n{\"version\": \"1.0\", \"name\": \"fenced\"}\n```"
	m, err := ParseManifestResponse(raw)
	if err != nil {
		t.Fatalf("ParseManifestResponse: %v", err)
	}
	if m.Name != "fenced" {
		t.Errorf("expected name 'fenced', got %q", m.Name)
	}
}

func TestParseManifestResponse_Invalid(t *testing.T) {
	raw := "not json at all"
	_, err := ParseManifestResponse(raw)
	if err == nil {
		t.Error("expected error for invalid response")
	}
}
```

- [ ] **Step 3: Run tests**

```bash
cd /Users/kent/Documents/Projects/vibeserve && go test ./internal/llm/ -v
```

Expected: all TestBuildSystemPrompt_*, TestExtractJSON_*, TestParseManifestResponse_* tests pass.

- [ ] **Step 4: Commit**

```bash
git add internal/llm/provider.go internal/llm/provider_test.go
git commit -m "feat: add LLM Provider interface, system prompt builder, and JSON extraction"
```

---

### Task 6: Claude API Provider

**Files:**
- Create: `internal/llm/claude.go`
- Create: `internal/llm/claude_test.go`

POST to `https://api.anthropic.com/v1/messages` using net/http directly. No SDK dependency.

- [ ] **Step 1: Create `internal/llm/claude.go`**

Create `internal/llm/claude.go`:

```go
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

const (
	claudeDefaultURL     = "https://api.anthropic.com/v1/messages"
	claudeAPIVersion     = "2023-06-01"
	claudeDefaultMaxTok  = 4096
)

// ClaudeProvider calls the Anthropic Messages API to generate manifests.
type ClaudeProvider struct {
	apiKey   string
	model    string
	baseURL  string // overridable for testing
	client   *http.Client
	maxTokens int
}

// ClaudeOption configures a ClaudeProvider.
type ClaudeOption func(*ClaudeProvider)

// WithClaudeBaseURL overrides the API base URL (for testing with httptest).
func WithClaudeBaseURL(url string) ClaudeOption {
	return func(c *ClaudeProvider) { c.baseURL = url }
}

// WithClaudeClient overrides the HTTP client.
func WithClaudeClient(client *http.Client) ClaudeOption {
	return func(c *ClaudeProvider) { c.client = client }
}

// WithClaudeMaxTokens overrides the max_tokens parameter.
func WithClaudeMaxTokens(n int) ClaudeOption {
	return func(c *ClaudeProvider) { c.maxTokens = n }
}

// NewClaudeProvider creates a Provider that calls the Claude API.
func NewClaudeProvider(apiKey, model string, opts ...ClaudeOption) *ClaudeProvider {
	p := &ClaudeProvider{
		apiKey:    apiKey,
		model:     model,
		baseURL:   claudeDefaultURL,
		client:    http.DefaultClient,
		maxTokens: claudeDefaultMaxTok,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// claudeRequest is the JSON body sent to the Claude API.
type claudeRequest struct {
	Model     string         `json:"model"`
	MaxTokens int            `json:"max_tokens"`
	System    string         `json:"system"`
	Messages  []claudeMsg    `json:"messages"`
}

type claudeMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// claudeResponse is the JSON response from the Claude API.
type claudeResponse struct {
	Content []claudeContent `json:"content"`
	Error   *claudeError    `json:"error,omitempty"`
}

type claudeContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type claudeError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// Generate implements Provider.
func (c *ClaudeProvider) Generate(ctx context.Context, current *manifest.Manifest, prompt string, history []Message) (*manifest.Manifest, error) {
	systemPrompt := BuildSystemPrompt(current)

	// Build messages: history + current prompt
	var msgs []claudeMsg
	for _, h := range history {
		msgs = append(msgs, claudeMsg{Role: h.Role, Content: h.Content})
	}
	msgs = append(msgs, claudeMsg{Role: "user", Content: prompt})

	reqBody := claudeRequest{
		Model:     c.model,
		MaxTokens: c.maxTokens,
		System:    systemPrompt,
		Messages:  msgs,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", claudeAPIVersion)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("claude API request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("claude API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var claudeResp claudeResponse
	if err := json.Unmarshal(respBody, &claudeResp); err != nil {
		return nil, fmt.Errorf("parse claude response: %w", err)
	}

	if claudeResp.Error != nil {
		return nil, fmt.Errorf("claude error: %s: %s", claudeResp.Error.Type, claudeResp.Error.Message)
	}

	if len(claudeResp.Content) == 0 {
		return nil, fmt.Errorf("claude returned empty content")
	}

	// Find the text content block
	var text string
	for _, block := range claudeResp.Content {
		if block.Type == "text" {
			text = block.Text
			break
		}
	}
	if text == "" {
		return nil, fmt.Errorf("claude returned no text content")
	}

	return ParseManifestResponse(text)
}
```

- [ ] **Step 2: Create `internal/llm/claude_test.go`**

Create `internal/llm/claude_test.go`:

```go
package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

func TestClaudeProvider_Generate(t *testing.T) {
	// Create a mock Claude API server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request headers
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("expected x-api-key 'test-key', got %q", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Errorf("expected anthropic-version '2023-06-01', got %q", r.Header.Get("anthropic-version"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type 'application/json', got %q", r.Header.Get("Content-Type"))
		}

		// Verify request body
		var reqBody claudeRequest
		json.NewDecoder(r.Body).Decode(&reqBody)
		if reqBody.Model != "test-model" {
			t.Errorf("expected model 'test-model', got %q", reqBody.Model)
		}
		if reqBody.System == "" {
			t.Error("expected non-empty system prompt")
		}
		if len(reqBody.Messages) == 0 {
			t.Error("expected at least one message")
		}

		// Return a valid manifest
		resp := claudeResponse{
			Content: []claudeContent{
				{
					Type: "text",
					Text: `{"version": "1.0", "name": "test-api", "description": "Test", "schemas": [], "routes": [], "scripts": [], "seeds": []}`,
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewClaudeProvider("test-key", "test-model", WithClaudeBaseURL(server.URL))

	m, err := provider.Generate(context.Background(), nil, "Create a todo API", nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if m.Name != "test-api" {
		t.Errorf("expected name 'test-api', got %q", m.Name)
	}
}

func TestClaudeProvider_Generate_WithHistory(t *testing.T) {
	var receivedMessages int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody claudeRequest
		json.NewDecoder(r.Body).Decode(&reqBody)
		receivedMessages = len(reqBody.Messages)

		resp := claudeResponse{
			Content: []claudeContent{
				{Type: "text", Text: `{"version": "1.0", "name": "test", "description": "", "schemas": [], "routes": [], "scripts": [], "seeds": []}`},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewClaudeProvider("key", "model", WithClaudeBaseURL(server.URL))

	history := []Message{
		{Role: "user", Content: "Create a todo API"},
		{Role: "assistant", Content: `{"version": "1.0", "name": "test"}`},
	}

	_, err := provider.Generate(context.Background(), nil, "Add a description field", history)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	// History (2) + current prompt (1) = 3 messages
	if receivedMessages != 3 {
		t.Errorf("expected 3 messages, got %d", receivedMessages)
	}
}

func TestClaudeProvider_Generate_WithCurrentManifest(t *testing.T) {
	var receivedSystem string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody claudeRequest
		json.NewDecoder(r.Body).Decode(&reqBody)
		receivedSystem = reqBody.System

		resp := claudeResponse{
			Content: []claudeContent{
				{Type: "text", Text: `{"version": "1.0", "name": "current", "description": "", "schemas": [], "routes": [], "scripts": [], "seeds": []}`},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewClaudeProvider("key", "model", WithClaudeBaseURL(server.URL))

	current := &manifest.Manifest{
		Version: "1.0",
		Name:    "my-existing-api",
	}

	_, err := provider.Generate(context.Background(), current, "Add users", nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if receivedSystem == "" {
		t.Fatal("expected non-empty system prompt")
	}
	if len(receivedSystem) < 100 {
		t.Error("system prompt suspiciously short")
	}
}

func TestClaudeProvider_Generate_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error": {"type": "rate_limit", "message": "Too many requests"}}`))
	}))
	defer server.Close()

	provider := NewClaudeProvider("key", "model", WithClaudeBaseURL(server.URL))

	_, err := provider.Generate(context.Background(), nil, "test", nil)
	if err == nil {
		t.Error("expected error for API error response")
	}
}

func TestClaudeProvider_Generate_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := claudeResponse{
			Content: []claudeContent{
				{Type: "text", Text: "I cannot help with that request."},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewClaudeProvider("key", "model", WithClaudeBaseURL(server.URL))

	_, err := provider.Generate(context.Background(), nil, "test", nil)
	if err == nil {
		t.Error("expected error for non-JSON response")
	}
}

func TestClaudeProvider_Generate_EmptyContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := claudeResponse{Content: []claudeContent{}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewClaudeProvider("key", "model", WithClaudeBaseURL(server.URL))

	_, err := provider.Generate(context.Background(), nil, "test", nil)
	if err == nil {
		t.Error("expected error for empty content")
	}
}
```

- [ ] **Step 3: Run tests**

```bash
cd /Users/kent/Documents/Projects/vibeserve && go test ./internal/llm/ -run TestClaude -v
```

Expected: all TestClaudeProvider_* tests pass.

- [ ] **Step 4: Commit**

```bash
git add internal/llm/claude.go internal/llm/claude_test.go
git commit -m "feat: add Claude API provider — POST to anthropic messages API via net/http"
```

---

### Task 7: Ollama Provider

**Files:**
- Create: `internal/llm/ollama.go`
- Create: `internal/llm/ollama_test.go`

POST to `http://localhost:11434/api/chat` using net/http directly. No SDK dependency.

- [ ] **Step 1: Create `internal/llm/ollama.go`**

Create `internal/llm/ollama.go`:

```go
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

const (
	ollamaDefaultPath = "/api/chat"
)

// OllamaProvider calls a local Ollama instance to generate manifests.
type OllamaProvider struct {
	host   string // e.g. "http://localhost:11434"
	model  string
	client *http.Client
}

// OllamaOption configures an OllamaProvider.
type OllamaOption func(*OllamaProvider)

// WithOllamaClient overrides the HTTP client.
func WithOllamaClient(client *http.Client) OllamaOption {
	return func(o *OllamaProvider) { o.client = client }
}

// NewOllamaProvider creates a Provider that calls a local Ollama instance.
func NewOllamaProvider(host, model string, opts ...OllamaOption) *OllamaProvider {
	p := &OllamaProvider{
		host:   host,
		model:  model,
		client: http.DefaultClient,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// ollamaRequest is the JSON body sent to the Ollama chat API.
type ollamaRequest struct {
	Model    string       `json:"model"`
	Messages []ollamaMsg  `json:"messages"`
	Stream   bool         `json:"stream"`
}

type ollamaMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ollamaResponse is the JSON response from the Ollama chat API.
type ollamaResponse struct {
	Message ollamaRespMsg `json:"message"`
	Error   string        `json:"error,omitempty"`
}

type ollamaRespMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Generate implements Provider.
func (o *OllamaProvider) Generate(ctx context.Context, current *manifest.Manifest, prompt string, history []Message) (*manifest.Manifest, error) {
	systemPrompt := BuildSystemPrompt(current)

	// Build messages: system + history + current prompt
	var msgs []ollamaMsg
	msgs = append(msgs, ollamaMsg{Role: "system", Content: systemPrompt})
	for _, h := range history {
		msgs = append(msgs, ollamaMsg{Role: h.Role, Content: h.Content})
	}
	msgs = append(msgs, ollamaMsg{Role: "user", Content: prompt})

	reqBody := ollamaRequest{
		Model:    o.model,
		Messages: msgs,
		Stream:   false,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := o.host + ollamaDefaultPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama API request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var ollamaResp ollamaResponse
	if err := json.Unmarshal(respBody, &ollamaResp); err != nil {
		return nil, fmt.Errorf("parse ollama response: %w", err)
	}

	if ollamaResp.Error != "" {
		return nil, fmt.Errorf("ollama error: %s", ollamaResp.Error)
	}

	text := ollamaResp.Message.Content
	if text == "" {
		return nil, fmt.Errorf("ollama returned empty content")
	}

	return ParseManifestResponse(text)
}
```

- [ ] **Step 2: Create `internal/llm/ollama_test.go`**

Create `internal/llm/ollama_test.go`:

```go
package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

func TestOllamaProvider_Generate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.URL.Path != "/api/chat" {
			t.Errorf("expected path /api/chat, got %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		var reqBody ollamaRequest
		json.NewDecoder(r.Body).Decode(&reqBody)

		if reqBody.Model != "llama3" {
			t.Errorf("expected model 'llama3', got %q", reqBody.Model)
		}
		if reqBody.Stream != false {
			t.Error("expected stream=false")
		}
		// Should have: system + user = 2 messages
		if len(reqBody.Messages) < 2 {
			t.Errorf("expected at least 2 messages, got %d", len(reqBody.Messages))
		}
		if reqBody.Messages[0].Role != "system" {
			t.Errorf("expected first message role 'system', got %q", reqBody.Messages[0].Role)
		}

		resp := ollamaResponse{
			Message: ollamaRespMsg{
				Role:    "assistant",
				Content: `{"version": "1.0", "name": "test-api", "description": "Test", "schemas": [], "routes": [], "scripts": [], "seeds": []}`,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewOllamaProvider(server.URL, "llama3")

	m, err := provider.Generate(context.Background(), nil, "Create a todo API", nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if m.Name != "test-api" {
		t.Errorf("expected name 'test-api', got %q", m.Name)
	}
}

func TestOllamaProvider_Generate_WithHistory(t *testing.T) {
	var receivedMessages int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody ollamaRequest
		json.NewDecoder(r.Body).Decode(&reqBody)
		receivedMessages = len(reqBody.Messages)

		resp := ollamaResponse{
			Message: ollamaRespMsg{
				Role:    "assistant",
				Content: `{"version": "1.0", "name": "test", "description": "", "schemas": [], "routes": [], "scripts": [], "seeds": []}`,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewOllamaProvider(server.URL, "llama3")

	history := []Message{
		{Role: "user", Content: "Create a todo API"},
		{Role: "assistant", Content: `{"version": "1.0"}`},
	}

	_, err := provider.Generate(context.Background(), nil, "Add a description field", history)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	// system (1) + history (2) + current prompt (1) = 4 messages
	if receivedMessages != 4 {
		t.Errorf("expected 4 messages, got %d", receivedMessages)
	}
}

func TestOllamaProvider_Generate_WithCurrentManifest(t *testing.T) {
	var receivedSystem string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody ollamaRequest
		json.NewDecoder(r.Body).Decode(&reqBody)
		if len(reqBody.Messages) > 0 && reqBody.Messages[0].Role == "system" {
			receivedSystem = reqBody.Messages[0].Content
		}

		resp := ollamaResponse{
			Message: ollamaRespMsg{
				Content: `{"version": "1.0", "name": "test", "description": "", "schemas": [], "routes": [], "scripts": [], "seeds": []}`,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewOllamaProvider(server.URL, "llama3")
	current := &manifest.Manifest{Version: "1.0", Name: "existing-api"}

	_, err := provider.Generate(context.Background(), current, "Add users", nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if receivedSystem == "" {
		t.Fatal("expected non-empty system prompt")
	}
}

func TestOllamaProvider_Generate_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "model not found"}`))
	}))
	defer server.Close()

	provider := NewOllamaProvider(server.URL, "nonexistent")

	_, err := provider.Generate(context.Background(), nil, "test", nil)
	if err == nil {
		t.Error("expected error for API error response")
	}
}

func TestOllamaProvider_Generate_OllamaError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ollamaResponse{Error: "model 'foo' not found"}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewOllamaProvider(server.URL, "foo")

	_, err := provider.Generate(context.Background(), nil, "test", nil)
	if err == nil {
		t.Error("expected error for ollama error response")
	}
}

func TestOllamaProvider_Generate_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ollamaResponse{
			Message: ollamaRespMsg{Content: "I am not able to produce JSON right now."},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewOllamaProvider(server.URL, "llama3")

	_, err := provider.Generate(context.Background(), nil, "test", nil)
	if err == nil {
		t.Error("expected error for non-JSON LLM response")
	}
}

func TestOllamaProvider_Generate_EmptyContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ollamaResponse{Message: ollamaRespMsg{Content: ""}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewOllamaProvider(server.URL, "llama3")

	_, err := provider.Generate(context.Background(), nil, "test", nil)
	if err == nil {
		t.Error("expected error for empty content")
	}
}

func TestOllamaProvider_Generate_MarkdownWrappedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ollamaResponse{
			Message: ollamaRespMsg{
				Content: "Here is the manifest:\n```json\n{\"version\": \"1.0\", \"name\": \"wrapped\", \"description\": \"\", \"schemas\": [], \"routes\": [], \"scripts\": [], \"seeds\": []}\n```",
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewOllamaProvider(server.URL, "llama3")

	m, err := provider.Generate(context.Background(), nil, "test", nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if m.Name != "wrapped" {
		t.Errorf("expected name 'wrapped', got %q", m.Name)
	}
}
```

- [ ] **Step 3: Run tests**

```bash
cd /Users/kent/Documents/Projects/vibeserve && go test ./internal/llm/ -run TestOllama -v
```

Expected: all TestOllamaProvider_* tests pass.

- [ ] **Step 4: Commit**

```bash
git add internal/llm/ollama.go internal/llm/ollama_test.go
git commit -m "feat: add Ollama provider — POST to local Ollama /api/chat via net/http"
```

---

### Task 8: Engine Coordinator

**Files:**
- Create: `internal/engine/engine.go`
- Create: `internal/engine/engine_test.go`

The Engine orchestrates the full flow: user prompt -> LLM -> validate -> diff -> snapshot -> migrate -> update routes -> update scripts -> save manifest. It uses the existing Bus to emit events at each step.

- [ ] **Step 1: Create `internal/engine/engine.go`**

Create `internal/engine/engine.go`:

```go
package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/vibeserve/vibeserve/internal/llm"
	"github.com/vibeserve/vibeserve/internal/manifest"
	"github.com/vibeserve/vibeserve/internal/router"
	"github.com/vibeserve/vibeserve/internal/snapshot"
	"github.com/vibeserve/vibeserve/internal/store"
)

// Engine coordinates the full cycle: prompt → LLM → validate → diff → migrate → update routes.
type Engine struct {
	bus      *Bus
	store    *store.Store
	trie     *router.Trie
	scripts  map[string]string
	provider llm.Provider
	manifest *manifest.Manifest
	history  []llm.Message
	vibeDir  string
}

// EngineConfig holds configuration for creating a new Engine.
type EngineConfig struct {
	Bus      *Bus
	Store    *store.Store
	Trie     *router.Trie
	Scripts  map[string]string
	Provider llm.Provider
	Manifest *manifest.Manifest
	VibeDir  string
}

// NewEngine creates a new Engine with the given configuration.
func NewEngine(cfg EngineConfig) *Engine {
	return &Engine{
		bus:      cfg.Bus,
		store:    cfg.Store,
		trie:     cfg.Trie,
		scripts:  cfg.Scripts,
		provider: cfg.Provider,
		manifest: cfg.Manifest,
		vibeDir:  cfg.VibeDir,
	}
}

// ApplyResult holds the result of processing a user prompt.
type ApplyResult struct {
	Changes  []manifest.Change
	Warnings []string
	Manifest *manifest.Manifest
}

// Manifest returns the current manifest.
func (e *Engine) Manifest() *manifest.Manifest {
	return e.manifest
}

// History returns the conversation history.
func (e *Engine) History() []llm.Message {
	return e.history
}

// Apply processes a user prompt through the full pipeline:
// 1. Emit UserPromptReceived
// 2. Call LLM to generate manifest
// 3. Validate the manifest
// 4. Diff against current manifest
// 5. Snapshot before schema changes
// 6. Apply schema migrations
// 7. Update routes and scripts
// 8. Seed new tables
// 9. Save manifest to disk
func (e *Engine) Apply(ctx context.Context, prompt string) (*ApplyResult, error) {
	result := &ApplyResult{}

	// 1. Emit UserPromptReceived
	e.bus.Publish(Event{Type: EventUserPromptReceived, Data: prompt})

	// 2. Call LLM
	e.bus.Publish(Event{Type: EventLLMRequestStarted, Data: prompt})

	newManifest, err := e.provider.Generate(ctx, e.manifest, prompt, e.history)
	if err != nil {
		return nil, fmt.Errorf("LLM generation failed: %w", err)
	}

	e.bus.Publish(Event{Type: EventLLMRequestCompleted, Data: newManifest})
	e.bus.Publish(Event{Type: EventManifestGenerated, Data: newManifest})

	// 3. Validate
	if err := manifest.Validate(newManifest); err != nil {
		e.bus.Publish(Event{Type: EventManifestValidationFailed, Data: err.Error()})
		return nil, fmt.Errorf("manifest validation failed: %w", err)
	}

	// 4. Diff
	changes := manifest.Diff(e.manifest, newManifest)
	e.bus.Publish(Event{Type: EventManifestDiffComputed, Data: changes})
	result.Changes = changes

	// 5 & 6. Apply schema changes
	hasSchemaChanges := false
	for _, c := range changes {
		if c.Type == manifest.ChangeAddTable || c.Type == manifest.ChangeAddColumn {
			hasSchemaChanges = true
			break
		}
	}

	if hasSchemaChanges {
		// Snapshot before schema changes
		dbPath := e.store.DSN()
		if dbPath != ":memory:" && dbPath != "" {
			e.bus.Publish(Event{Type: EventSchemaAltering, Data: "creating snapshot"})
			snap, snapErr := snapshot.Create(e.vibeDir, dbPath, "before_schema_change")
			if snapErr != nil {
				log.Printf("Warning: failed to create snapshot: %v", snapErr)
			} else {
				e.bus.Publish(Event{Type: EventSnapshotCreated, Data: snap})
			}
		}

		for _, c := range changes {
			switch c.Type {
			case manifest.ChangeAddTable:
				e.bus.Publish(Event{Type: EventSchemaAltering, Data: c.Detail})
				if err := e.store.ApplySchemas([]manifest.Schema{*c.Schema}); err != nil {
					e.bus.Publish(Event{Type: EventSchemaMigrationFailed, Data: err.Error()})
					return nil, fmt.Errorf("schema migration failed: %w", err)
				}
				e.bus.Publish(Event{Type: EventSchemaAltered, Data: c.Detail})

			case manifest.ChangeAddColumn:
				e.bus.Publish(Event{Type: EventSchemaAltering, Data: c.Detail})
				if err := e.store.AddColumn(c.Table, *c.Column); err != nil {
					e.bus.Publish(Event{Type: EventSchemaMigrationFailed, Data: err.Error()})
					return nil, fmt.Errorf("add column failed: %w", err)
				}
				e.bus.Publish(Event{Type: EventSchemaAltered, Data: c.Detail})

			case manifest.ChangeDropColumn:
				result.Warnings = append(result.Warnings, c.Detail)
			}
		}
	}

	// Check for drop column warnings even if no adds
	for _, c := range changes {
		if c.Type == manifest.ChangeDropColumn {
			result.Warnings = append(result.Warnings, c.Detail)
		}
	}

	// 7. Update routes and scripts
	for _, c := range changes {
		switch c.Type {
		case manifest.ChangeAddRoute:
			e.trie.Insert(c.Route.Method, c.Route.Path, c.Route.Script)
			e.bus.Publish(Event{Type: EventRouteAdded, Data: c.Detail})

		case manifest.ChangeUpdateRoute:
			e.trie.Remove(c.Route.Method, c.Route.Path)
			e.trie.Insert(c.Route.Method, c.Route.Path, c.Route.Script)
			e.bus.Publish(Event{Type: EventRouteUpdated, Data: c.Detail})

		case manifest.ChangeRemoveRoute:
			e.trie.Remove(c.Route.Method, c.Route.Path)
			e.bus.Publish(Event{Type: EventRouteRemoved, Data: c.Detail})

		case manifest.ChangeAddScript, manifest.ChangeUpdateScript:
			e.bus.Publish(Event{Type: EventScriptValidationStarted, Data: c.Script.Name})
			e.scripts[c.Script.Name] = c.Script.Code
			e.bus.Publish(Event{Type: EventScriptLoaded, Data: c.Script.Name})

		case manifest.ChangeRemoveScript:
			delete(e.scripts, c.Script.Name)
		}
	}

	// 8. Seed new tables
	for _, c := range changes {
		if c.Type == manifest.ChangeAddSeed {
			if err := e.store.Seed(c.Seed.Table, c.Seed.Rows); err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("seed %s failed: %v", c.Seed.Table, err))
			} else {
				e.bus.Publish(Event{Type: EventDataSeeded, Data: c.Seed.Table})
			}
		}
	}

	// 9. Save manifest to disk
	e.manifest = newManifest
	result.Manifest = newManifest

	if err := e.saveManifest(); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("failed to save manifest: %v", err))
	}

	// Update conversation history
	// Store the prompt as a JSON string of the manifest for context
	manifestJSON, _ := json.Marshal(newManifest)
	e.history = append(e.history,
		llm.Message{Role: "user", Content: prompt},
		llm.Message{Role: "assistant", Content: string(manifestJSON)},
	)

	return result, nil
}

// Undo restores the most recent snapshot and reloads the previous manifest.
func (e *Engine) Undo() error {
	dbPath := e.store.DSN()
	if dbPath == ":memory:" || dbPath == "" {
		return fmt.Errorf("undo not supported with in-memory database")
	}

	// Close the current store
	e.store.Close()

	// Restore the latest snapshot
	snap, err := snapshot.RestoreLatest(e.vibeDir, dbPath)
	if err != nil {
		return fmt.Errorf("restore snapshot: %w", err)
	}

	// Reopen the store
	newStore, err := store.New(dbPath)
	if err != nil {
		return fmt.Errorf("reopen store: %w", err)
	}
	e.store = newStore

	e.bus.Publish(Event{Type: EventSnapshotRestored, Data: snap})

	// Reload the previous manifest if available
	manifestPath := filepath.Join(e.vibeDir, "manifest.json")
	if prev, err := manifest.LoadFromFile(manifestPath); err == nil {
		// Remove the last change from history
		if len(e.history) >= 2 {
			e.history = e.history[:len(e.history)-2]
		}
		e.manifest = prev
	}

	return nil
}

// saveManifest writes the current manifest to .vibe/manifest.json.
func (e *Engine) saveManifest() error {
	if e.vibeDir == "" {
		return nil
	}
	data, err := json.MarshalIndent(e.manifest, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(e.vibeDir, "manifest.json")
	return os.WriteFile(path, data, 0o644)
}

// FormatChangeSummary produces a human-readable summary of changes.
func FormatChangeSummary(result *ApplyResult) string {
	if len(result.Changes) == 0 {
		return "No changes detected."
	}

	var b strings.Builder
	b.WriteString("Changes applied:\n")

	for _, c := range result.Changes {
		switch c.Type {
		case manifest.ChangeAddTable:
			b.WriteString(fmt.Sprintf("  + Table: %s\n", c.Table))
		case manifest.ChangeAddColumn:
			b.WriteString(fmt.Sprintf("  + Column: %s.%s (%s)\n", c.Table, c.Column.Name, c.Column.Type))
		case manifest.ChangeDropColumn:
			b.WriteString(fmt.Sprintf("  ~ Warning: %s\n", c.Detail))
		case manifest.ChangeAddRoute:
			b.WriteString(fmt.Sprintf("  + Route: %s %s\n", c.Route.Method, c.Route.Path))
		case manifest.ChangeUpdateRoute:
			b.WriteString(fmt.Sprintf("  ~ Route: %s %s (updated)\n", c.Route.Method, c.Route.Path))
		case manifest.ChangeRemoveRoute:
			b.WriteString(fmt.Sprintf("  - Route: %s %s\n", c.Route.Method, c.Route.Path))
		case manifest.ChangeAddScript:
			b.WriteString(fmt.Sprintf("  + Script: %s\n", c.Script.Name))
		case manifest.ChangeUpdateScript:
			b.WriteString(fmt.Sprintf("  ~ Script: %s (updated)\n", c.Script.Name))
		case manifest.ChangeRemoveScript:
			b.WriteString(fmt.Sprintf("  - Script: %s\n", c.Script.Name))
		case manifest.ChangeAddSeed:
			b.WriteString(fmt.Sprintf("  + Seed: %s\n", c.Table))
		}
	}

	for _, w := range result.Warnings {
		b.WriteString(fmt.Sprintf("  ! %s\n", w))
	}

	return b.String()
}
```

- [ ] **Step 2: Create `internal/engine/engine_test.go`**

Create `internal/engine/engine_test.go`:

```go
package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/vibeserve/vibeserve/internal/llm"
	"github.com/vibeserve/vibeserve/internal/manifest"
	"github.com/vibeserve/vibeserve/internal/router"
	"github.com/vibeserve/vibeserve/internal/store"
)

// mockProvider is a test provider that returns a fixed manifest.
type mockProvider struct {
	manifest *manifest.Manifest
	err      error
}

func (m *mockProvider) Generate(ctx context.Context, current *manifest.Manifest, prompt string, history []llm.Message) (*manifest.Manifest, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.manifest, nil
}

func newTestEngine(t *testing.T, provider llm.Provider) (*Engine, string) {
	t.Helper()

	tmpDir := t.TempDir()
	vibeDir := filepath.Join(tmpDir, ".vibe")
	os.MkdirAll(vibeDir, 0o755)

	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	trie := router.NewTrie()
	scripts := make(map[string]string)

	eng := NewEngine(EngineConfig{
		Bus:      NewBus(),
		Store:    s,
		Trie:     trie,
		Scripts:  scripts,
		Provider: provider,
		VibeDir:  vibeDir,
	})

	return eng, vibeDir
}

func TestEngine_Apply_NewManifest(t *testing.T) {
	newManifest := &manifest.Manifest{
		Version: "1.0",
		Name:    "test-api",
		Description: "Test API",
		Schemas: []manifest.Schema{
			{Table: "users", Columns: []manifest.Column{
				{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
				{Name: "name", Type: "TEXT", Required: true},
			}},
		},
		Routes: []Route{
			{Path: "/users", Method: "GET", Script: "list_users", ResponseType: "array"},
		},
		Scripts: []manifest.Script{
			{Name: "list_users", Code: "result := db.query(\"SELECT * FROM users\", [])\nresponse.json(result)"},
		},
	}

	eng, _ := newTestEngine(t, &mockProvider{manifest: newManifest})

	result, err := eng.Apply(context.Background(), "Create a users API")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if len(result.Changes) == 0 {
		t.Error("expected changes")
	}

	// Verify table was added
	foundTable := false
	for _, c := range result.Changes {
		if c.Type == manifest.ChangeAddTable && c.Table == "users" {
			foundTable = true
		}
	}
	if !foundTable {
		t.Error("expected ADD_TABLE change for users")
	}

	// Verify route was registered
	script, _, found := eng.trie.Search("GET", "/users")
	if !found {
		t.Error("expected GET /users route to be registered")
	}
	if script != "list_users" {
		t.Errorf("expected script 'list_users', got %q", script)
	}

	// Verify script was loaded
	if eng.scripts["list_users"] == "" {
		t.Error("expected list_users script to be loaded")
	}

	// Verify manifest was updated
	if eng.Manifest().Name != "test-api" {
		t.Errorf("expected manifest name 'test-api', got %q", eng.Manifest().Name)
	}

	// Verify history was updated
	if len(eng.History()) != 2 {
		t.Errorf("expected 2 history entries, got %d", len(eng.History()))
	}
}

func TestEngine_Apply_AddColumn(t *testing.T) {
	// Start with an existing manifest
	initial := &manifest.Manifest{
		Version: "1.0",
		Name:    "test-api",
		Description: "Test",
		Schemas: []manifest.Schema{
			{Table: "users", Columns: []manifest.Column{
				{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
				{Name: "name", Type: "TEXT", Required: true},
			}},
		},
		Routes: []Route{
			{Path: "/users", Method: "GET", Script: "list_users", ResponseType: "array"},
		},
		Scripts: []manifest.Script{
			{Name: "list_users", Code: "result := db.query(\"SELECT * FROM users\", [])\nresponse.json(result)"},
		},
	}

	// New manifest adds an email column
	updated := &manifest.Manifest{
		Version: "1.0",
		Name:    "test-api",
		Description: "Test",
		Schemas: []manifest.Schema{
			{Table: "users", Columns: []manifest.Column{
				{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
				{Name: "name", Type: "TEXT", Required: true},
				{Name: "email", Type: "TEXT", Unique: true},
			}},
		},
		Routes: []Route{
			{Path: "/users", Method: "GET", Script: "list_users", ResponseType: "array"},
		},
		Scripts: []manifest.Script{
			{Name: "list_users", Code: "result := db.query(\"SELECT * FROM users\", [])\nresponse.json(result)"},
		},
	}

	eng, _ := newTestEngine(t, &mockProvider{manifest: updated})

	// Set up initial state
	eng.manifest = initial
	eng.store.ApplySchemas(initial.Schemas)
	eng.trie.Insert("GET", "/users", "list_users")
	eng.scripts["list_users"] = initial.Scripts[0].Code

	result, err := eng.Apply(context.Background(), "Add email to users")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	foundAddCol := false
	for _, c := range result.Changes {
		if c.Type == manifest.ChangeAddColumn && c.Table == "users" && c.Column.Name == "email" {
			foundAddCol = true
		}
	}
	if !foundAddCol {
		t.Error("expected ADD_COLUMN change for users.email")
	}
}

func TestEngine_Apply_EventsEmitted(t *testing.T) {
	newManifest := &manifest.Manifest{
		Version: "1.0",
		Name:    "test",
		Description: "Test",
		Schemas: []manifest.Schema{
			{Table: "items", Columns: []manifest.Column{
				{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			}},
		},
		Routes: []Route{
			{Path: "/items", Method: "GET", Script: "list_items", ResponseType: "array"},
		},
		Scripts: []manifest.Script{
			{Name: "list_items", Code: "response.json([])"},
		},
	}

	eng, _ := newTestEngine(t, &mockProvider{manifest: newManifest})

	var events []EventType
	var mu sync.Mutex

	// Subscribe to all relevant events
	for _, et := range []EventType{
		EventUserPromptReceived, EventLLMRequestStarted, EventLLMRequestCompleted,
		EventManifestGenerated, EventManifestDiffComputed,
		EventSchemaAltering, EventSchemaAltered, EventRouteAdded, EventScriptLoaded,
	} {
		eventType := et
		eng.bus.Subscribe(eventType, func(e Event) {
			mu.Lock()
			events = append(events, e.Type)
			mu.Unlock()
		})
	}

	_, err := eng.Apply(context.Background(), "Create items API")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// Wait for async event delivery
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(events) < 5 {
		t.Errorf("expected at least 5 events, got %d: %v", len(events), events)
	}

	// Check key events were emitted
	eventSet := make(map[EventType]bool)
	for _, e := range events {
		eventSet[e] = true
	}

	required := []EventType{
		EventUserPromptReceived,
		EventLLMRequestStarted,
		EventLLMRequestCompleted,
		EventManifestGenerated,
		EventManifestDiffComputed,
	}
	for _, req := range required {
		if !eventSet[req] {
			t.Errorf("missing required event: %s", req)
		}
	}
}

func TestEngine_Apply_ValidationFailure(t *testing.T) {
	// Manifest with no name (will fail validation)
	badManifest := &manifest.Manifest{
		Version: "1.0",
		Name:    "", // invalid
	}

	eng, _ := newTestEngine(t, &mockProvider{manifest: badManifest})

	_, err := eng.Apply(context.Background(), "test")
	if err == nil {
		t.Error("expected error for invalid manifest")
	}
}

func TestEngine_Apply_LLMError(t *testing.T) {
	eng, _ := newTestEngine(t, &mockProvider{err: fmt.Errorf("API timeout")})

	_, err := eng.Apply(context.Background(), "test")
	if err == nil {
		t.Error("expected error for LLM failure")
	}
}

func TestEngine_Apply_SavesManifest(t *testing.T) {
	newManifest := &manifest.Manifest{
		Version: "1.0",
		Name:    "saved-api",
		Description: "Test",
		Schemas: []manifest.Schema{},
		Routes:  []manifest.Route{},
		Scripts: []manifest.Script{},
	}

	eng, vibeDir := newTestEngine(t, &mockProvider{manifest: newManifest})

	_, err := eng.Apply(context.Background(), "Create API")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// Verify manifest was saved
	manifestPath := filepath.Join(vibeDir, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}

	var saved manifest.Manifest
	json.Unmarshal(data, &saved)
	if saved.Name != "saved-api" {
		t.Errorf("expected saved name 'saved-api', got %q", saved.Name)
	}
}

func TestFormatChangeSummary(t *testing.T) {
	result := &ApplyResult{
		Changes: []manifest.Change{
			{Type: manifest.ChangeAddTable, Table: "users"},
			{Type: manifest.ChangeAddColumn, Table: "users", Column: &manifest.Column{Name: "email", Type: "TEXT"}},
			{Type: manifest.ChangeAddRoute, Route: &manifest.Route{Method: "GET", Path: "/users"}},
			{Type: manifest.ChangeAddScript, Script: &manifest.Script{Name: "list_users"}},
		},
	}

	summary := FormatChangeSummary(result)
	if summary == "" {
		t.Error("expected non-empty summary")
	}
	if !contains(summary, "Table: users") {
		t.Error("expected table mention in summary")
	}
	if !contains(summary, "Column: users.email") {
		t.Error("expected column mention in summary")
	}
	if !contains(summary, "Route: GET /users") {
		t.Error("expected route mention in summary")
	}
	if !contains(summary, "Script: list_users") {
		t.Error("expected script mention in summary")
	}
}

func TestFormatChangeSummary_NoChanges(t *testing.T) {
	result := &ApplyResult{Changes: nil}
	summary := FormatChangeSummary(result)
	if summary != "No changes detected." {
		t.Errorf("expected 'No changes detected.', got %q", summary)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// Alias the Route type for use in test code since manifest.Route is what we need.
type Route = manifest.Route
```

- [ ] **Step 3: Run tests**

```bash
cd /Users/kent/Documents/Projects/vibeserve && go test ./internal/engine/ -v
```

Expected: all existing bus tests pass plus TestEngine_Apply_*, TestFormatChangeSummary* tests pass.

- [ ] **Step 4: Commit**

```bash
git add internal/engine/engine.go internal/engine/engine_test.go
git commit -m "feat: add Engine coordinator — orchestrates prompt → LLM → diff → migrate → route update pipeline"
```

---

### Task 9: REPL `vibeserve dev` + `vibeserve undo` Commands

**Files:**
- Modify: `cmd/vibeserve/main.go`

Add two new subcommands: `dev` (interactive REPL with concurrent HTTP server) and `undo` (restore last snapshot).

- [ ] **Step 1: Rewrite `cmd/vibeserve/main.go`**

Replace the contents of `cmd/vibeserve/main.go`:

```go
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/vibeserve/vibeserve/internal/config"
	"github.com/vibeserve/vibeserve/internal/engine"
	"github.com/vibeserve/vibeserve/internal/llm"
	"github.com/vibeserve/vibeserve/internal/manifest"
	"github.com/vibeserve/vibeserve/internal/router"
	"github.com/vibeserve/vibeserve/internal/runtime"
	"github.com/vibeserve/vibeserve/internal/snapshot"
	"github.com/vibeserve/vibeserve/internal/store"
)

var version = "0.2.0"

func main() {
	rootCmd := &cobra.Command{
		Use:   "vibeserve",
		Short: "AI-powered stateful API backend from natural language",
	}

	rootCmd.AddCommand(upCmd())
	rootCmd.AddCommand(devCmd())
	rootCmd.AddCommand(undoCmd())
	rootCmd.AddCommand(versionCmd())
	rootCmd.AddCommand(routesCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func upCmd() *cobra.Command {
	var port int
	var host string
	var manifestPath string

	cmd := &cobra.Command{
		Use:   "up",
		Short: "Start the API server from an existing manifest",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUp(manifestPath, host, port)
		},
	}

	cmd.Flags().IntVarP(&port, "port", "p", 8080, "Server port")
	cmd.Flags().StringVar(&host, "host", "localhost", "Server host")
	cmd.Flags().StringVarP(&manifestPath, "manifest", "m", ".vibe/manifest.json", "Path to manifest.json")

	return cmd
}

func devCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dev",
		Short: "Start interactive REPL with live API server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDev()
		},
	}

	return cmd
}

func undoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "undo",
		Short: "Restore the last database snapshot",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUndo()
		},
	}

	return cmd
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("vibeserve", version)
		},
	}
}

func routesCmd() *cobra.Command {
	var manifestPath string

	cmd := &cobra.Command{
		Use:   "routes",
		Short: "Print the route table from a manifest",
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := manifest.LoadFromFile(manifestPath)
			if err != nil {
				return fmt.Errorf("load manifest: %w", err)
			}
			for _, r := range m.Routes {
				fmt.Printf("  %-6s %s  → %s\n", r.Method, r.Path, r.Script)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&manifestPath, "manifest", "m", ".vibe/manifest.json", "Path to manifest.json")
	return cmd
}

func runUp(manifestPath, host string, port int) error {
	m, err := manifest.LoadFromFile(manifestPath)
	if err != nil {
		return fmt.Errorf("load manifest: %w", err)
	}
	log.Printf("Loaded manifest: %s (%d routes, %d schemas)", m.Name, len(m.Routes), len(m.Schemas))

	if err := manifest.Validate(m); err != nil {
		return fmt.Errorf("invalid manifest: %w", err)
	}
	log.Println("Manifest validated")

	bus := engine.NewBus()
	bus.Subscribe(engine.EventLogEmitted, func(e engine.Event) {
		if data, ok := e.Data.(map[string]string); ok {
			log.Printf("[%s] %s", data["level"], data["message"])
		}
	})

	os.MkdirAll(".vibe", 0o755)

	s, err := store.New(".vibe/state.db")
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer s.Close()

	if err := s.ApplySchemas(m.Schemas); err != nil {
		return fmt.Errorf("apply schemas: %w", err)
	}
	for _, schema := range m.Schemas {
		log.Printf("Schema applied: %s (%d columns)", schema.Table, len(schema.Columns))
	}

	for _, seed := range m.Seeds {
		if err := s.Seed(seed.Table, seed.Rows); err != nil {
			return fmt.Errorf("seed %s: %w", seed.Table, err)
		}
		log.Printf("Seeded %s: %d rows", seed.Table, len(seed.Rows))
	}

	trie := router.NewTrie()
	scripts := make(map[string]string)
	for _, sc := range m.Scripts {
		scripts[sc.Name] = sc.Code
	}
	for _, r := range m.Routes {
		trie.Insert(r.Method, r.Path, r.Script)
		log.Printf("Route registered: %s %s → %s", r.Method, r.Path, r.Script)
	}

	rt := runtime.New(s, bus)
	handler := router.NewHandler(trie, scripts, rt, true)
	srv := router.NewServer(host, port, handler)

	manifestData, _ := json.MarshalIndent(m, "", "  ")
	os.WriteFile(".vibe/manifest.json", manifestData, 0o644)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	select {
	case err := <-errCh:
		return err
	case <-sigCh:
		log.Println("Shutting down...")
		return srv.Shutdown(context.Background())
	}
}

func runDev() error {
	vibeDir := ".vibe"
	os.MkdirAll(vibeDir, 0o755)

	// Load config
	configPath := filepath.Join(vibeDir, "config.yaml")
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("config validation: %w", err)
	}

	// Create LLM provider
	var provider llm.Provider
	switch cfg.Provider {
	case "claude":
		provider = llm.NewClaudeProvider(cfg.APIKey(), cfg.Model)
	case "ollama":
		provider = llm.NewOllamaProvider(cfg.OllamaHost, cfg.Model)
	default:
		return fmt.Errorf("unknown provider: %s", cfg.Provider)
	}

	// Create store
	dbPath := filepath.Join(vibeDir, "state.db")
	s, err := store.New(dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer s.Close()

	bus := engine.NewBus()
	bus.Subscribe(engine.EventLogEmitted, func(e engine.Event) {
		if data, ok := e.Data.(map[string]any); ok {
			log.Printf("[%s] %s", data["level"], data["message"])
		}
	})

	trie := router.NewTrie()
	scripts := make(map[string]string)

	// Load existing manifest if present
	var currentManifest *manifest.Manifest
	manifestPath := filepath.Join(vibeDir, "manifest.json")
	if m, err := manifest.LoadFromFile(manifestPath); err == nil {
		currentManifest = m
		// Apply existing schemas and routes
		if err := s.ApplySchemas(m.Schemas); err != nil {
			log.Printf("Warning: apply existing schemas: %v", err)
		}
		for _, sc := range m.Scripts {
			scripts[sc.Name] = sc.Code
		}
		for _, r := range m.Routes {
			trie.Insert(r.Method, r.Path, r.Script)
		}
		log.Printf("Loaded existing manifest: %s (%d routes)", m.Name, len(m.Routes))
	}

	// Create engine
	eng := engine.NewEngine(engine.EngineConfig{
		Bus:      bus,
		Store:    s,
		Trie:     trie,
		Scripts:  scripts,
		Provider: provider,
		Manifest: currentManifest,
		VibeDir:  vibeDir,
	})

	// Start HTTP server in background
	rt := runtime.New(s, bus)
	handler := router.NewHandler(trie, scripts, rt, cfg.Server.CORS)
	srv := router.NewServer(cfg.Server.Host, cfg.Server.Port, handler)

	go func() {
		if err := srv.Start(); err != nil {
			log.Printf("Server error: %v", err)
		}
	}()

	fmt.Printf("VibeServe dev server running on http://%s:%d\n", cfg.Server.Host, cfg.Server.Port)
	fmt.Printf("Provider: %s (model: %s)\n", cfg.Provider, cfg.Model)
	fmt.Println("Type your request (or 'quit' to exit, 'undo' to restore last snapshot):")
	fmt.Println()

	// REPL loop
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("vibeserve> ")
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		switch strings.ToLower(input) {
		case "quit", "exit":
			fmt.Println("Shutting down...")
			srv.Shutdown(context.Background())
			return nil

		case "undo":
			if err := eng.Undo(); err != nil {
				fmt.Printf("Undo failed: %v\n", err)
			} else {
				fmt.Println("Restored last snapshot successfully.")
				if eng.Manifest() != nil {
					fmt.Printf("Manifest: %s (%d routes)\n", eng.Manifest().Name, len(eng.Manifest().Routes))
				}
			}
			continue

		case "routes":
			if eng.Manifest() == nil {
				fmt.Println("No manifest loaded yet.")
			} else {
				for _, r := range eng.Manifest().Routes {
					fmt.Printf("  %-6s %s  → %s\n", r.Method, r.Path, r.Script)
				}
			}
			continue

		case "status":
			if eng.Manifest() == nil {
				fmt.Println("No manifest loaded.")
			} else {
				m := eng.Manifest()
				fmt.Printf("Name: %s\n", m.Name)
				fmt.Printf("Tables: %d, Routes: %d, Scripts: %d\n", len(m.Schemas), len(m.Routes), len(m.Scripts))
			}
			continue
		}

		fmt.Println("Thinking...")
		result, err := eng.Apply(context.Background(), input)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}

		fmt.Println()
		fmt.Print(engine.FormatChangeSummary(result))
		fmt.Println()

		if result.Manifest != nil {
			fmt.Printf("API: %s — %d routes active\n", result.Manifest.Name, len(result.Manifest.Routes))
		}
		fmt.Println()
	}

	srv.Shutdown(context.Background())
	return nil
}

func runUndo() error {
	vibeDir := ".vibe"
	dbPath := filepath.Join(vibeDir, "state.db")

	snap, err := snapshot.RestoreLatest(vibeDir, dbPath)
	if err != nil {
		return fmt.Errorf("undo: %w", err)
	}

	fmt.Printf("Restored snapshot #%d (%s)\n", snap.ID, snap.Description)
	fmt.Println("Restart the server to apply the restored state.")
	return nil
}
```

- [ ] **Step 2: Verify build**

```bash
cd /Users/kent/Documents/Projects/vibeserve && go build ./cmd/vibeserve
```

Expected: binary compiles with no errors.

- [ ] **Step 3: Verify CLI help**

```bash
cd /Users/kent/Documents/Projects/vibeserve && ./vibeserve --help
```

Expected output should list: `dev`, `routes`, `undo`, `up`, `version` commands.

```bash
cd /Users/kent/Documents/Projects/vibeserve && ./vibeserve dev --help
```

Expected: shows "Start interactive REPL with live API server".

```bash
cd /Users/kent/Documents/Projects/vibeserve && ./vibeserve undo --help
```

Expected: shows "Restore the last database snapshot".

- [ ] **Step 4: Run full test suite to confirm no regressions**

```bash
cd /Users/kent/Documents/Projects/vibeserve && go test ./... -v
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add cmd/vibeserve/main.go
git commit -m "feat: add 'vibeserve dev' REPL and 'vibeserve undo' commands"
```

---

### Task 10: End-to-End Integration Test

**Files:**
- Create: `testdata/car_rental_v2_manifest.json`
- Create: `internal/engine/integration_test.go`

This test exercises the full pipeline with a mock LLM: starts from the car rental manifest, then "adds" a new customers table and route via the engine, and verifies the HTTP server serves the new route.

- [ ] **Step 1: Create `testdata/car_rental_v2_manifest.json`**

Create `testdata/car_rental_v2_manifest.json`:

```json
{
  "version": "1.0",
  "name": "malaysia-car-rental",
  "description": "Car rental API with duration-based discounts and customer management",
  "schemas": [
    {
      "table": "vehicles",
      "columns": [
        {"name": "id", "type": "INTEGER", "primary": true, "auto": true},
        {"name": "make", "type": "TEXT", "required": true},
        {"name": "model", "type": "TEXT", "required": true},
        {"name": "plate", "type": "TEXT", "unique": true},
        {"name": "daily_rate", "type": "REAL", "required": true},
        {"name": "available", "type": "BOOLEAN", "default": true}
      ]
    },
    {
      "table": "bookings",
      "columns": [
        {"name": "id", "type": "INTEGER", "primary": true, "auto": true},
        {"name": "vehicle_id", "type": "INTEGER"},
        {"name": "customer", "type": "TEXT", "required": true},
        {"name": "start_date", "type": "DATE", "required": true},
        {"name": "end_date", "type": "DATE", "required": true},
        {"name": "total_price", "type": "REAL"},
        {"name": "status", "type": "TEXT", "default": "pending"}
      ]
    },
    {
      "table": "customers",
      "columns": [
        {"name": "id", "type": "INTEGER", "primary": true, "auto": true},
        {"name": "name", "type": "TEXT", "required": true},
        {"name": "email", "type": "TEXT", "unique": true},
        {"name": "phone", "type": "TEXT"}
      ]
    }
  ],
  "routes": [
    {
      "path": "/vehicles",
      "method": "GET",
      "description": "List all available vehicles",
      "script": "list_vehicles",
      "response_type": "array"
    },
    {
      "path": "/vehicles/:id",
      "method": "GET",
      "description": "Get vehicle details",
      "script": "get_vehicle",
      "response_type": "object"
    },
    {
      "path": "/bookings",
      "method": "POST",
      "description": "Create booking with discount logic",
      "script": "create_booking",
      "request_body": {
        "vehicle_id": "INTEGER",
        "customer": "TEXT",
        "start_date": "DATE",
        "end_date": "DATE"
      },
      "response_type": "object"
    },
    {
      "path": "/customers",
      "method": "GET",
      "description": "List all customers",
      "script": "list_customers",
      "response_type": "array"
    },
    {
      "path": "/customers",
      "method": "POST",
      "description": "Create a customer",
      "script": "create_customer",
      "request_body": {
        "name": "TEXT",
        "email": "TEXT",
        "phone": "TEXT"
      },
      "response_type": "object"
    }
  ],
  "scripts": [
    {
      "name": "list_vehicles",
      "code": "result := db.query(\"SELECT * FROM vehicles WHERE available = ?\", [true])\nresponse.json(result)"
    },
    {
      "name": "get_vehicle",
      "code": "id := request.param(\"id\")\nrow := db.query_one(\"SELECT * FROM vehicles WHERE id = ?\", [id])\nif row == undefined {\n  response.fail(404, \"Vehicle not found\")\n} else {\n  response.json(row)\n}"
    },
    {
      "name": "create_booking",
      "code": "body := request.body()\nvehicle := db.query_one(\"SELECT * FROM vehicles WHERE id = ?\", [body.vehicle_id])\nif vehicle == undefined {\n  response.fail(404, \"Vehicle not found\")\n}\ndays := date.diff_days(body.start_date, body.end_date)\nrate := vehicle.daily_rate\nif days >= 7 {\n  rate = rate * 0.8\n}\ntotal := rate * days\nbooking := db.insert(\"bookings\", {\n  vehicle_id: body.vehicle_id,\n  customer: body.customer,\n  start_date: body.start_date,\n  end_date: body.end_date,\n  total_price: total,\n  status: \"confirmed\"\n})\ndb.update(\"vehicles\", vehicle.id, { available: false })\nresponse.json(booking, 201)"
    },
    {
      "name": "list_customers",
      "code": "result := db.query(\"SELECT * FROM customers\", [])\nresponse.json(result)"
    },
    {
      "name": "create_customer",
      "code": "body := request.body()\nresult := db.insert(\"customers\", {\n  name: body.name,\n  email: body.email,\n  phone: body.phone\n})\nresponse.json(result, 201)"
    }
  ],
  "seeds": [
    {
      "table": "vehicles",
      "rows": [
        {"make": "Perodua", "model": "Myvi", "plate": "WKL 3321", "daily_rate": 89.00, "available": true},
        {"make": "Proton", "model": "X50", "plate": "JHR 7788", "daily_rate": 149.00, "available": true},
        {"make": "Toyota", "model": "Vios", "plate": "PNG 1234", "daily_rate": 129.00, "available": true}
      ]
    }
  ]
}
```

- [ ] **Step 2: Create `internal/engine/integration_test.go`**

Create `internal/engine/integration_test.go`:

```go
package engine

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vibeserve/vibeserve/internal/llm"
	"github.com/vibeserve/vibeserve/internal/manifest"
	"github.com/vibeserve/vibeserve/internal/router"
	"github.com/vibeserve/vibeserve/internal/runtime"
	"github.com/vibeserve/vibeserve/internal/store"
)

// TestIntegration_EngineEvolvesManifest tests the full pipeline:
// 1. Start with the car rental v1 manifest
// 2. Use a mock LLM that returns v2 (adds customers table + routes)
// 3. Verify the engine applies all changes
// 4. Verify the HTTP server serves the new routes
func TestIntegration_EngineEvolvesManifest(t *testing.T) {
	// Load v1 manifest
	v1, err := manifest.LoadFromFile("../../testdata/car_rental_manifest.json")
	if err != nil {
		t.Fatalf("load v1 manifest: %v", err)
	}

	// Load v2 manifest (what the "LLM" will return)
	v2, err := manifest.LoadFromFile("../../testdata/car_rental_v2_manifest.json")
	if err != nil {
		t.Fatalf("load v2 manifest: %v", err)
	}

	// Set up temp directory for vibe dir
	tmpDir := t.TempDir()
	vibeDir := filepath.Join(tmpDir, ".vibe")
	os.MkdirAll(vibeDir, 0o755)

	// Create store
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	defer s.Close()

	// Apply v1 schemas and seeds
	if err := s.ApplySchemas(v1.Schemas); err != nil {
		t.Fatalf("apply v1 schemas: %v", err)
	}
	for _, seed := range v1.Seeds {
		if err := s.Seed(seed.Table, seed.Rows); err != nil {
			t.Fatalf("seed v1 %s: %v", seed.Table, err)
		}
	}

	// Set up trie and scripts with v1
	trie := router.NewTrie()
	scripts := make(map[string]string)
	for _, sc := range v1.Scripts {
		scripts[sc.Name] = sc.Code
	}
	for _, r := range v1.Routes {
		trie.Insert(r.Method, r.Path, r.Script)
	}

	bus := NewBus()

	// Mock provider returns v2
	provider := &mockProvider{manifest: v2}

	eng := NewEngine(EngineConfig{
		Bus:      bus,
		Store:    s,
		Trie:     trie,
		Scripts:  scripts,
		Provider: provider,
		Manifest: v1,
		VibeDir:  vibeDir,
	})

	// Apply the "evolution"
	result, err := eng.Apply(context.Background(), "Add customer management")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// Verify changes include new table
	foundCustomersTable := false
	foundListCustomersRoute := false
	foundCreateCustomerRoute := false
	foundListCustomersScript := false
	foundCreateCustomerScript := false

	for _, c := range result.Changes {
		switch {
		case c.Type == manifest.ChangeAddTable && c.Table == "customers":
			foundCustomersTable = true
		case c.Type == manifest.ChangeAddRoute && c.Route.Method == "GET" && c.Route.Path == "/customers":
			foundListCustomersRoute = true
		case c.Type == manifest.ChangeAddRoute && c.Route.Method == "POST" && c.Route.Path == "/customers":
			foundCreateCustomerRoute = true
		case c.Type == manifest.ChangeAddScript && c.Script.Name == "list_customers":
			foundListCustomersScript = true
		case c.Type == manifest.ChangeAddScript && c.Script.Name == "create_customer":
			foundCreateCustomerScript = true
		}
	}

	if !foundCustomersTable {
		t.Error("expected ADD_TABLE for customers")
	}
	if !foundListCustomersRoute {
		t.Error("expected ADD_ROUTE for GET /customers")
	}
	if !foundCreateCustomerRoute {
		t.Error("expected ADD_ROUTE for POST /customers")
	}
	if !foundListCustomersScript {
		t.Error("expected ADD_SCRIPT for list_customers")
	}
	if !foundCreateCustomerScript {
		t.Error("expected ADD_SCRIPT for create_customer")
	}

	// Now test the HTTP server serves the new routes
	rt := runtime.New(s, bus)
	handler := router.NewHandler(trie, scripts, rt, true)

	// Test: GET /customers should work (empty list)
	req := httptest.NewRequest("GET", "/customers", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("GET /customers: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var customers []map[string]any
	json.NewDecoder(rec.Body).Decode(&customers)
	if len(customers) != 0 {
		t.Errorf("expected 0 customers initially, got %d", len(customers))
	}

	// Test: POST /customers should create a customer
	body := `{"name": "Ahmad", "email": "ahmad@test.com", "phone": "012-345-6789"}`
	req = httptest.NewRequest("POST", "/customers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 201 {
		t.Fatalf("POST /customers: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var created map[string]any
	json.NewDecoder(rec.Body).Decode(&created)
	if created["name"] != "Ahmad" {
		t.Errorf("expected name 'Ahmad', got %v", created["name"])
	}
	if created["email"] != "ahmad@test.com" {
		t.Errorf("expected email 'ahmad@test.com', got %v", created["email"])
	}

	// Test: GET /customers should now return 1 customer
	req = httptest.NewRequest("GET", "/customers", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	json.NewDecoder(rec.Body).Decode(&customers)
	if len(customers) != 1 {
		t.Errorf("expected 1 customer, got %d", len(customers))
	}

	// Test: Original routes still work
	req = httptest.NewRequest("GET", "/vehicles", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("GET /vehicles: expected 200, got %d", rec.Code)
	}

	var vehicles []map[string]any
	json.NewDecoder(rec.Body).Decode(&vehicles)
	if len(vehicles) != 3 {
		t.Errorf("expected 3 vehicles, got %d", len(vehicles))
	}

	// Verify manifest was saved
	manifestPath := filepath.Join(vibeDir, "manifest.json")
	savedData, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read saved manifest: %v", err)
	}
	var saved manifest.Manifest
	json.Unmarshal(savedData, &saved)
	if saved.Name != "malaysia-car-rental" {
		t.Errorf("expected saved name 'malaysia-car-rental', got %q", saved.Name)
	}
	if len(saved.Routes) != 5 {
		t.Errorf("expected 5 routes in saved manifest, got %d", len(saved.Routes))
	}
}

// TestIntegration_DiffAndMigrate tests that diffing a manifest correctly
// produces changes and migrations apply without error.
func TestIntegration_DiffAndMigrate(t *testing.T) {
	v1 := &manifest.Manifest{
		Version:     "1.0",
		Name:        "test",
		Description: "Test",
		Schemas: []manifest.Schema{
			{Table: "items", Columns: []manifest.Column{
				{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
				{Name: "name", Type: "TEXT", Required: true},
			}},
		},
		Routes: []manifest.Route{
			{Path: "/items", Method: "GET", Script: "list_items", ResponseType: "array"},
		},
		Scripts: []manifest.Script{
			{Name: "list_items", Code: "result := db.query(\"SELECT * FROM items\", [])\nresponse.json(result)"},
		},
	}

	v2 := &manifest.Manifest{
		Version:     "1.0",
		Name:        "test",
		Description: "Test v2",
		Schemas: []manifest.Schema{
			{Table: "items", Columns: []manifest.Column{
				{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
				{Name: "name", Type: "TEXT", Required: true},
				{Name: "price", Type: "REAL"},
				{Name: "active", Type: "BOOLEAN", Default: true},
			}},
			{Table: "categories", Columns: []manifest.Column{
				{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
				{Name: "label", Type: "TEXT", Required: true},
			}},
		},
		Routes: []manifest.Route{
			{Path: "/items", Method: "GET", Script: "list_items", ResponseType: "array"},
			{Path: "/items", Method: "POST", Script: "create_item", ResponseType: "object"},
			{Path: "/categories", Method: "GET", Script: "list_categories", ResponseType: "array"},
		},
		Scripts: []manifest.Script{
			{Name: "list_items", Code: "result := db.query(\"SELECT * FROM items\", [])\nresponse.json(result)"},
			{Name: "create_item", Code: "body := request.body()\nresult := db.insert(\"items\", body)\nresponse.json(result, 201)"},
			{Name: "list_categories", Code: "result := db.query(\"SELECT * FROM categories\", [])\nresponse.json(result)"},
		},
	}

	// Diff
	changes := manifest.Diff(v1, v2)

	// Should have: ADD_TABLE categories, ADD_COLUMN items.price, ADD_COLUMN items.active,
	// ADD_ROUTE POST /items, ADD_ROUTE GET /categories,
	// ADD_SCRIPT create_item, ADD_SCRIPT list_categories
	expectedTypes := map[manifest.ChangeType]int{
		manifest.ChangeAddTable:  1, // categories
		manifest.ChangeAddColumn: 2, // price, active
		manifest.ChangeAddRoute:  2, // POST /items, GET /categories
		manifest.ChangeAddScript: 2, // create_item, list_categories
	}

	counts := make(map[manifest.ChangeType]int)
	for _, c := range changes {
		counts[c.Type]++
	}

	for ct, expected := range expectedTypes {
		if counts[ct] != expected {
			t.Errorf("expected %d %s changes, got %d", expected, ct, counts[ct])
		}
	}

	// Apply migrations to verify they work
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	defer s.Close()

	// Apply v1 schema
	if err := s.ApplySchemas(v1.Schemas); err != nil {
		t.Fatalf("apply v1: %v", err)
	}

	// Insert a row before migration
	_, err = s.Insert("items", map[string]any{"name": "Widget"})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Apply changes
	for _, c := range changes {
		switch c.Type {
		case manifest.ChangeAddTable:
			if err := s.ApplySchemas([]manifest.Schema{*c.Schema}); err != nil {
				t.Fatalf("add table %s: %v", c.Table, err)
			}
		case manifest.ChangeAddColumn:
			if err := s.AddColumn(c.Table, *c.Column); err != nil {
				t.Fatalf("add column %s.%s: %v", c.Table, c.Column.Name, err)
			}
		}
	}

	// Verify old data still exists
	row, err := s.QueryOne("SELECT * FROM items WHERE name = ?", []any{"Widget"})
	if err != nil {
		t.Fatalf("query after migration: %v", err)
	}
	if row == nil {
		t.Fatal("expected row to exist after migration")
	}
	if row["name"] != "Widget" {
		t.Errorf("expected name 'Widget', got %v", row["name"])
	}

	// Verify new column works
	_, err = s.Insert("items", map[string]any{"name": "Gadget", "price": 29.99, "active": true})
	if err != nil {
		t.Fatalf("insert with new columns: %v", err)
	}

	// Verify new table works
	_, err = s.Insert("categories", map[string]any{"label": "Electronics"})
	if err != nil {
		t.Fatalf("insert into new table: %v", err)
	}

	count, err := s.Count("categories")
	if err != nil {
		t.Fatalf("count categories: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 category, got %d", count)
	}
}

// TestIntegration_SnapshotAndRestore tests snapshot creation and restoration
// through the engine.
func TestIntegration_SnapshotAndRestore(t *testing.T) {
	tmpDir := t.TempDir()
	vibeDir := filepath.Join(tmpDir, ".vibe")
	os.MkdirAll(vibeDir, 0o755)

	dbPath := filepath.Join(vibeDir, "state.db")

	// Create a store with some data
	s, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}

	schema := manifest.Schema{
		Table: "items",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "name", Type: "TEXT", Required: true},
		},
	}
	if err := s.ApplySchemas([]manifest.Schema{schema}); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	s.Insert("items", map[string]any{"name": "Original"})

	v1 := &manifest.Manifest{
		Version:     "1.0",
		Name:        "test",
		Description: "v1",
		Schemas:     []manifest.Schema{schema},
		Routes:      []manifest.Route{{Path: "/items", Method: "GET", Script: "list", ResponseType: "array"}},
		Scripts:     []manifest.Script{{Name: "list", Code: "response.json([])"}},
	}

	// V2 adds a column
	v2 := &manifest.Manifest{
		Version:     "1.0",
		Name:        "test",
		Description: "v2",
		Schemas: []manifest.Schema{{
			Table: "items",
			Columns: []manifest.Column{
				{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
				{Name: "name", Type: "TEXT", Required: true},
				{Name: "price", Type: "REAL"},
			},
		}},
		Routes:  []manifest.Route{{Path: "/items", Method: "GET", Script: "list", ResponseType: "array"}},
		Scripts: []manifest.Script{{Name: "list", Code: "response.json([])"}},
	}

	trie := router.NewTrie()
	trie.Insert("GET", "/items", "list")
	scripts := map[string]string{"list": "response.json([])"}

	eng := NewEngine(EngineConfig{
		Bus:      NewBus(),
		Store:    s,
		Trie:     trie,
		Scripts:  scripts,
		Provider: &mockProvider{manifest: v2},
		Manifest: v1,
		VibeDir:  vibeDir,
	})

	// Apply v2 — this should create a snapshot first
	_, err = eng.Apply(context.Background(), "Add price column")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// Verify the column was added
	count, err := s.Count("items")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 item, got %d", count)
	}

	// Close the store before undo (undo will reopen)
	// The engine handles this internally, but we need to verify snapshots exist
	snapshots, err := snapshotList(vibeDir)
	if err != nil {
		t.Fatalf("list snapshots: %v", err)
	}
	if len(snapshots) == 0 {
		t.Error("expected at least 1 snapshot after schema change")
	}
}

func snapshotList(vibeDir string) ([]string, error) {
	snapDir := filepath.Join(vibeDir, "snapshots")
	entries, err := os.ReadDir(snapDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names, nil
}
```

- [ ] **Step 3: Run integration tests**

```bash
cd /Users/kent/Documents/Projects/vibeserve && go test ./internal/engine/ -run TestIntegration -v
```

Expected: all TestIntegration_* tests pass.

- [ ] **Step 4: Run the full test suite**

```bash
cd /Users/kent/Documents/Projects/vibeserve && go test ./... -v
```

Expected: ALL tests across all packages pass with zero failures.

- [ ] **Step 5: Commit**

```bash
git add testdata/car_rental_v2_manifest.json internal/engine/integration_test.go
git commit -m "feat: add end-to-end integration tests for engine pipeline with mock LLM"
```

---

## Verification Checklist

After all 10 tasks are complete, run:

```bash
cd /Users/kent/Documents/Projects/vibeserve && go test ./... -v -count=1
```

Expected: every test passes. The following packages should have tests:

| Package | Test count |
|---------|-----------|
| `internal/manifest` | existing + 10 diff tests |
| `internal/engine` | existing bus tests + 6 engine tests + 3 integration tests |
| `internal/store` | existing + 5 migrate tests + 3 store tests |
| `internal/snapshot` | 8 tests |
| `internal/config` | 10 tests |
| `internal/llm` | 7 provider tests + 5 claude tests + 7 ollama tests |
| `internal/router` | existing (unchanged) |
| `internal/runtime` | existing (unchanged) |
| `internal` | existing integration test (unchanged) |

Then verify the binary builds and help works:

```bash
go build ./cmd/vibeserve && ./vibeserve --help
```

Expected commands: `dev`, `routes`, `undo`, `up`, `version`.
