# Phase 1: The Skeleton — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Get a static manifest-driven API server running — user hand-writes a `manifest.json`, VibeServe hydrates routes + DB + Tengo scripts, and serves real HTTP requests with stateful data.

**Architecture:** Internal message bus (Approach B). Engine coordinates manifest → store → runtime → router via interface injection. Event bus emits typed events (logged only in Phase 1). No LLM, no TUI — just `vibeserve up`.

**Tech Stack:** Go 1.23+, modernc.org/sqlite (pure Go), d5/tengo/v2, spf13/cobra, gopkg.in/yaml.v3

---

## File Map

```
vibeserve/
├── cmd/vibeserve/
│   └── main.go                    # Cobra root + "up" subcommand
├── internal/
│   ├── engine/
│   │   ├── interfaces.go          # DataStore interface (ScriptEvaluator deferred to Phase 2)
│   │   ├── events.go              # Event type constants + Event struct
│   │   ├── bus.go                 # Typed pub/sub event bus
│   │   └── bus_test.go
│   ├── manifest/
│   │   ├── types.go               # Manifest, Schema, Column, Route, Script, Seed structs
│   │   ├── types_test.go
│   │   ├── validate.go            # Three-layer validation
│   │   └── validate_test.go
│   ├── store/
│   │   ├── store.go               # SQLite wrapper implementing DataStore
│   │   ├── store_test.go
│   │   ├── migrate.go             # Schema → CREATE TABLE SQL
│   │   └── migrate_test.go
│   ├── runtime/
│   │   ├── runtime.go             # Tengo VM implementing ScriptEvaluator
│   │   ├── runtime_test.go
│   │   ├── stdlib.go              # All stdlib modules: db, request, response, date, crypto, log
│   │   └── stdlib_test.go
│   └── router/
│       ├── trie.go                # Trie-based dynamic route matching
│       ├── trie_test.go
│       ├── handler.go             # HTTP handler: request → Tengo → response
│       ├── handler_test.go
│       └── server.go              # HTTP server lifecycle + CORS middleware
├── testdata/
│   └── car_rental_manifest.json   # Integration test fixture
├── go.mod
└── go.sum
```

---

### Task 1: Project Scaffolding

**Files:**
- Create: `go.mod`
- Create: `cmd/vibeserve/main.go` (placeholder)
- Create: `.gitignore`

- [ ] **Step 1: Initialize Go module and install dependencies**

```bash
cd /Users/kent/Documents/Projects/vibeserve
go mod init github.com/vibeserve/vibeserve
go get github.com/spf13/cobra@latest
go get modernc.org/sqlite@latest
go get github.com/d5/tengo/v2@latest
go get gopkg.in/yaml.v3@latest
```

- [ ] **Step 2: Create directory structure**

```bash
mkdir -p cmd/vibeserve
mkdir -p internal/{engine,manifest,store,runtime,router}
mkdir -p testdata
```

- [ ] **Step 3: Create .gitignore**

Create `.gitignore`:

```
.vibe/
*.db
```

- [ ] **Step 4: Create placeholder main.go**

Create `cmd/vibeserve/main.go`:

```go
package main

import "fmt"

func main() {
	fmt.Println("vibeserve")
}
```

- [ ] **Step 5: Verify build**

```bash
go build ./cmd/vibeserve
```

Expected: binary compiles with no errors.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum cmd/ internal/ testdata/ .gitignore
git commit -m "feat: scaffold project structure and dependencies"
```

---

### Task 2: Manifest Types + JSON Parsing

**Files:**
- Create: `internal/manifest/types.go`
- Create: `internal/manifest/types_test.go`

- [ ] **Step 1: Write the failing test — parse a minimal manifest JSON**

Create `internal/manifest/types_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/manifest/ -v
```

Expected: FAIL — `Manifest` type not defined.

- [ ] **Step 3: Implement manifest types**

Create `internal/manifest/types.go`:

```go
package manifest

import (
	"encoding/json"
	"os"
)

type Manifest struct {
	Version     string   `json:"version"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Schemas     []Schema `json:"schemas"`
	Routes      []Route  `json:"routes"`
	Scripts     []Script `json:"scripts"`
	Seeds       []Seed   `json:"seeds"`
}

type Schema struct {
	Table   string   `json:"table"`
	Columns []Column `json:"columns"`
}

type Column struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Primary    bool   `json:"primary,omitempty"`
	Auto       bool   `json:"auto,omitempty"`
	Required   bool   `json:"required,omitempty"`
	Unique     bool   `json:"unique,omitempty"`
	Default    any    `json:"default,omitempty"`
	References string `json:"references,omitempty"`
}

type Route struct {
	Path         string            `json:"path"`
	Method       string            `json:"method"`
	Description  string            `json:"description"`
	Script       string            `json:"script"`
	RequestBody  map[string]string `json:"request_body,omitempty"`
	ResponseType string            `json:"response_type"`
}

type Script struct {
	Name string `json:"name"`
	Code string `json:"code"`
}

type Seed struct {
	Table string           `json:"table"`
	Rows  []map[string]any `json:"rows"`
}

// LoadFromFile reads and parses a manifest JSON file.
func LoadFromFile(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/manifest/ -v
```

Expected: all 3 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/manifest/types.go internal/manifest/types_test.go
git commit -m "feat: add manifest types with JSON parsing"
```

---

### Task 3: Manifest Validation

**Files:**
- Create: `internal/manifest/validate.go`
- Create: `internal/manifest/validate_test.go`

- [ ] **Step 1: Write failing tests for structural validation**

Create `internal/manifest/validate_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/manifest/ -run TestValidate -v
```

Expected: FAIL — `Validate` function not defined.

- [ ] **Step 3: Implement three-layer validation**

Create `internal/manifest/validate.go`:

```go
package manifest

import (
	"fmt"
	"strings"

	"github.com/d5/tengo/v2"
)

var validMethods = map[string]bool{
	"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true,
}

var validColumnTypes = map[string]bool{
	"INTEGER": true, "TEXT": true, "REAL": true,
	"BOOLEAN": true, "DATE": true, "DATETIME": true,
}

// Validate runs all three validation layers on a manifest.
func Validate(m *Manifest) error {
	if err := validateStructural(m); err != nil {
		return fmt.Errorf("structural: %w", err)
	}
	if err := validateReferential(m); err != nil {
		return fmt.Errorf("referential: %w", err)
	}
	if err := validateCompilation(m); err != nil {
		return fmt.Errorf("compilation: %w", err)
	}
	return nil
}

func validateStructural(m *Manifest) error {
	if m.Name == "" {
		return fmt.Errorf("manifest name is required")
	}
	if m.Version == "" {
		return fmt.Errorf("manifest version is required")
	}

	tableNames := make(map[string]bool)
	for _, s := range m.Schemas {
		if s.Table == "" {
			return fmt.Errorf("schema table name is required")
		}
		if tableNames[s.Table] {
			return fmt.Errorf("duplicate table name: %q", s.Table)
		}
		tableNames[s.Table] = true

		for _, c := range s.Columns {
			if c.Name == "" {
				return fmt.Errorf("column name is required in table %q", s.Table)
			}
			if !validColumnTypes[c.Type] {
				return fmt.Errorf("invalid column type %q for %s.%s", c.Type, s.Table, c.Name)
			}
		}
	}

	for _, r := range m.Routes {
		if r.Path == "" {
			return fmt.Errorf("route path is required")
		}
		if !validMethods[r.Method] {
			return fmt.Errorf("invalid HTTP method %q for route %s", r.Method, r.Path)
		}
	}

	scriptNames := make(map[string]bool)
	for _, s := range m.Scripts {
		if s.Name == "" {
			return fmt.Errorf("script name is required")
		}
		if scriptNames[s.Name] {
			return fmt.Errorf("duplicate script name: %q", s.Name)
		}
		scriptNames[s.Name] = true
	}

	return nil
}

func validateReferential(m *Manifest) error {
	tables := make(map[string]map[string]bool)
	for _, s := range m.Schemas {
		cols := make(map[string]bool)
		for _, c := range s.Columns {
			cols[c.Name] = true
		}
		tables[s.Table] = cols
	}

	scripts := make(map[string]bool)
	for _, s := range m.Scripts {
		scripts[s.Name] = true
	}

	for _, r := range m.Routes {
		if !scripts[r.Script] {
			return fmt.Errorf("route %s %s references nonexistent script %q", r.Method, r.Path, r.Script)
		}
	}

	for _, s := range m.Seeds {
		if _, ok := tables[s.Table]; !ok {
			return fmt.Errorf("seed references nonexistent table %q", s.Table)
		}
	}

	for _, s := range m.Schemas {
		for _, c := range s.Columns {
			if c.References == "" {
				continue
			}
			parts := strings.SplitN(c.References, ".", 2)
			if len(parts) != 2 {
				return fmt.Errorf("invalid reference format %q (expected table.column)", c.References)
			}
			refTable, refCol := parts[0], parts[1]
			cols, ok := tables[refTable]
			if !ok {
				return fmt.Errorf("%s.%s references nonexistent table %q", s.Table, c.Name, refTable)
			}
			if !cols[refCol] {
				return fmt.Errorf("%s.%s references nonexistent column %s.%s", s.Table, c.Name, refTable, refCol)
			}
		}
	}

	return nil
}

// stdlibNames are the variable names injected into every Tengo script.
var stdlibNames = []string{"db", "request", "response", "date", "crypto", "log"}

func validateCompilation(m *Manifest) error {
	for _, s := range m.Scripts {
		script := tengo.NewScript([]byte(s.Code))
		for _, name := range stdlibNames {
			// Add empty maps so compiler knows these variables exist.
			_ = script.Add(name, map[string]interface{}{})
		}
		_, err := script.Compile()
		if err != nil {
			return fmt.Errorf("script %q: %w", s.Name, err)
		}
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/manifest/ -run TestValidate -v
```

Expected: all 9 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/manifest/validate.go internal/manifest/validate_test.go
git commit -m "feat: add three-layer manifest validation"
```

---

### Task 4: Event Bus + Engine Interfaces

**Files:**
- Create: `internal/engine/events.go`
- Create: `internal/engine/interfaces.go`
- Create: `internal/engine/bus.go`
- Create: `internal/engine/bus_test.go`

- [ ] **Step 1: Write failing test for event bus pub/sub**

Create `internal/engine/bus_test.go`:

```go
package engine

import (
	"sync"
	"testing"
	"time"
)

func TestBusPublishSubscribe(t *testing.T) {
	bus := NewBus()

	var received []Event
	var mu sync.Mutex

	bus.Subscribe(EventRouteAdded, func(e Event) {
		mu.Lock()
		received = append(received, e)
		mu.Unlock()
	})

	bus.Publish(Event{Type: EventRouteAdded, Data: "GET /users"})
	bus.Publish(Event{Type: EventRouteAdded, Data: "POST /users"})

	// Events are dispatched asynchronously; give them a moment.
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 2 {
		t.Fatalf("expected 2 events, got %d", len(received))
	}
	if received[0].Data != "GET /users" {
		t.Errorf("expected 'GET /users', got %v", received[0].Data)
	}
}

func TestBusFiltersByEventType(t *testing.T) {
	bus := NewBus()

	var count int
	var mu sync.Mutex

	bus.Subscribe(EventRouteAdded, func(e Event) {
		mu.Lock()
		count++
		mu.Unlock()
	})

	bus.Publish(Event{Type: EventRouteAdded, Data: "match"})
	bus.Publish(Event{Type: EventSchemaAltered, Data: "no match"})
	bus.Publish(Event{Type: EventRouteAdded, Data: "match"})

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if count != 2 {
		t.Fatalf("expected 2 events (filtered), got %d", count)
	}
}

func TestBusMultipleSubscribers(t *testing.T) {
	bus := NewBus()

	var count1, count2 int
	var mu sync.Mutex

	bus.Subscribe(EventScriptLoaded, func(e Event) {
		mu.Lock()
		count1++
		mu.Unlock()
	})
	bus.Subscribe(EventScriptLoaded, func(e Event) {
		mu.Lock()
		count2++
		mu.Unlock()
	})

	bus.Publish(Event{Type: EventScriptLoaded, Data: "hello"})

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if count1 != 1 || count2 != 1 {
		t.Errorf("expected both subscribers to receive event, got %d and %d", count1, count2)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/engine/ -v
```

Expected: FAIL — types not defined.

- [ ] **Step 3: Implement event types**

Create `internal/engine/events.go`:

```go
package engine

// EventType identifies what happened.
type EventType string

const (
	// Manifest lifecycle
	EventManifestGenerated        EventType = "MANIFEST_GENERATED"
	EventManifestValidationFailed EventType = "MANIFEST_VALIDATION_FAILED"
	EventManifestDiffComputed     EventType = "MANIFEST_DIFF_COMPUTED"

	// Schema / Data
	EventSchemaAltering       EventType = "SCHEMA_ALTERING"
	EventSchemaAltered        EventType = "SCHEMA_ALTERED"
	EventSchemaMigrationFailed EventType = "SCHEMA_MIGRATION_FAILED"
	EventSnapshotCreated      EventType = "SNAPSHOT_CREATED"
	EventSnapshotRestored     EventType = "SNAPSHOT_RESTORED"
	EventDataSeeded           EventType = "DATA_SEEDED"

	// Routes
	EventRouteAdded   EventType = "ROUTE_ADDED"
	EventRouteUpdated EventType = "ROUTE_UPDATED"
	EventRouteRemoved EventType = "ROUTE_REMOVED"

	// Script / Runtime
	EventScriptValidationStarted EventType = "SCRIPT_VALIDATION_STARTED"
	EventScriptValidationPassed  EventType = "SCRIPT_VALIDATION_PASSED"
	EventScriptValidationFailed  EventType = "SCRIPT_VALIDATION_FAILED"
	EventScriptLoaded            EventType = "SCRIPT_LOADED"
	EventScriptExecuted          EventType = "SCRIPT_EXECUTED"
	EventScriptError             EventType = "SCRIPT_ERROR"

	// Session
	EventSessionStarted      EventType = "SESSION_STARTED"
	EventUserPromptReceived  EventType = "USER_PROMPT_RECEIVED"
	EventLLMRequestStarted   EventType = "LLM_REQUEST_STARTED"
	EventLLMRequestCompleted EventType = "LLM_REQUEST_COMPLETED"

	// Observability
	EventHTTPRequestReceived EventType = "HTTP_REQUEST_RECEIVED"
	EventHTTPResponseSent    EventType = "HTTP_RESPONSE_SENT"
	EventLogEmitted          EventType = "LOG_EMITTED"
)

// Event is a single message on the bus.
type Event struct {
	Type EventType
	Data any
}
```

- [ ] **Step 4: Implement event bus**

Create `internal/engine/bus.go`:

```go
package engine

import "sync"

// SubscriberFunc handles a published event.
type SubscriberFunc func(Event)

// Bus is a typed pub/sub event bus.
type Bus struct {
	mu   sync.RWMutex
	subs map[EventType][]SubscriberFunc
}

// NewBus creates an event bus.
func NewBus() *Bus {
	return &Bus{subs: make(map[EventType][]SubscriberFunc)}
}

// Subscribe registers a handler for a specific event type.
func (b *Bus) Subscribe(t EventType, fn SubscriberFunc) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subs[t] = append(b.subs[t], fn)
}

// Publish dispatches an event to all subscribers of its type.
// Handlers run asynchronously in goroutines to avoid blocking the publisher.
func (b *Bus) Publish(e Event) {
	b.mu.RLock()
	handlers := b.subs[e.Type]
	b.mu.RUnlock()

	for _, fn := range handlers {
		go fn(e)
	}
}
```

- [ ] **Step 5: Implement engine interfaces**

Create `internal/engine/interfaces.go`:

```go
package engine

import (
	"github.com/vibeserve/vibeserve/internal/manifest"
)

// DataStore provides database operations for the Tengo stdlib.
type DataStore interface {
	// Schema management
	ApplySchemas(schemas []manifest.Schema) error
	Seed(table string, rows []map[string]any) error

	// Query operations (exposed to Tengo scripts via stdlib)
	Query(sql string, params []any) ([]map[string]any, error)
	QueryOne(sql string, params []any) (map[string]any, error)
	Insert(table string, data map[string]any) (map[string]any, error)
	Update(table string, id any, data map[string]any) (map[string]any, error)
	Delete(table string, id any) (bool, error)
	Count(table string) (int, error)

	Close() error
}

// Note: ScriptEvaluator interface deferred to Phase 2.
// In Phase 1, the handler uses *runtime.Runtime directly.
// The interface will be extracted when we need mock evaluators for testing
// the Engine coordinator without the Tengo VM.
```

- [ ] **Step 6: Run tests to verify they pass**

```bash
go test ./internal/engine/ -v
```

Expected: all 3 tests PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/engine/
git commit -m "feat: add event bus, event types, and engine interfaces"
```

---

### Task 5: Store — SQLite Schema + CRUD + Seeding

**Files:**
- Create: `internal/store/store.go`
- Create: `internal/store/store_test.go`
- Create: `internal/store/migrate.go`
- Create: `internal/store/migrate_test.go`

- [ ] **Step 1: Write failing tests for schema creation**

Create `internal/store/migrate_test.go`:

```go
package store

import (
	"testing"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

func TestBuildCreateTableSQL(t *testing.T) {
	schema := manifest.Schema{
		Table: "vehicles",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "make", Type: "TEXT", Required: true},
			{Name: "daily_rate", Type: "REAL", Required: true},
			{Name: "available", Type: "BOOLEAN", Default: true},
			{Name: "created_at", Type: "DATETIME", Default: "NOW"},
		},
	}

	sql := BuildCreateTableSQL(schema)

	// Should contain the table name
	if sql == "" {
		t.Fatal("expected non-empty SQL")
	}

	// Check key fragments
	expects := []string{
		"CREATE TABLE IF NOT EXISTS vehicles",
		"id INTEGER PRIMARY KEY AUTOINCREMENT",
		"make TEXT NOT NULL",
		"daily_rate REAL NOT NULL",
		"available INTEGER DEFAULT 1",
		"created_at TEXT DEFAULT CURRENT_TIMESTAMP",
	}
	for _, want := range expects {
		if !containsStr(sql, want) {
			t.Errorf("expected SQL to contain %q\ngot: %s", want, sql)
		}
	}
}

func TestBuildCreateTableSQL_ForeignKey(t *testing.T) {
	schema := manifest.Schema{
		Table: "bookings",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "vehicle_id", Type: "INTEGER", References: "vehicles.id"},
		},
	}

	sql := BuildCreateTableSQL(schema)
	if !containsStr(sql, "REFERENCES vehicles(id)") {
		t.Errorf("expected foreign key clause, got: %s", sql)
	}
}

func containsStr(haystack, needle string) bool {
	return len(haystack) >= len(needle) &&
		(haystack == needle || len(needle) == 0 ||
			findSubstring(haystack, needle))
}

func findSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/store/ -run TestBuild -v
```

Expected: FAIL — `BuildCreateTableSQL` not defined.

- [ ] **Step 3: Implement schema-to-SQL generation**

Create `internal/store/migrate.go`:

```go
package store

import (
	"fmt"
	"strings"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

// BuildCreateTableSQL generates a CREATE TABLE IF NOT EXISTS statement from a schema.
func BuildCreateTableSQL(s manifest.Schema) string {
	var cols []string
	for _, c := range s.Columns {
		cols = append(cols, buildColumnDef(c))
	}
	return fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (\n  %s\n)", s.Table, strings.Join(cols, ",\n  "))
}

func buildColumnDef(c manifest.Column) string {
	var parts []string
	parts = append(parts, c.Name)
	parts = append(parts, sqliteType(c.Type))

	if c.Primary {
		parts = append(parts, "PRIMARY KEY")
		if c.Auto {
			parts = append(parts, "AUTOINCREMENT")
		}
	}

	if c.Required && !c.Primary {
		parts = append(parts, "NOT NULL")
	}

	if c.Unique {
		parts = append(parts, "UNIQUE")
	}

	if c.Default != nil {
		parts = append(parts, "DEFAULT", sqliteDefault(c.Type, c.Default))
	}

	if c.References != "" {
		dotIdx := strings.IndexByte(c.References, '.')
		if dotIdx > 0 {
			table := c.References[:dotIdx]
			col := c.References[dotIdx+1:]
			parts = append(parts, fmt.Sprintf("REFERENCES %s(%s)", table, col))
		}
	}

	return strings.Join(parts, " ")
}

// sqliteType maps manifest types to SQLite storage types.
func sqliteType(t string) string {
	switch t {
	case "INTEGER":
		return "INTEGER"
	case "TEXT":
		return "TEXT"
	case "REAL":
		return "REAL"
	case "BOOLEAN":
		return "INTEGER" // SQLite stores booleans as 0/1
	case "DATE", "DATETIME":
		return "TEXT" // SQLite stores dates as ISO 8601 text
	default:
		return "TEXT"
	}
}

// sqliteDefault converts a manifest default value to its SQL representation.
func sqliteDefault(colType string, val any) string {
	switch v := val.(type) {
	case string:
		if v == "NOW" {
			return "CURRENT_TIMESTAMP"
		}
		return fmt.Sprintf("'%s'", v)
	case bool:
		if v {
			return "1"
		}
		return "0"
	case float64: // JSON numbers decode as float64
		if colType == "INTEGER" {
			return fmt.Sprintf("%d", int64(v))
		}
		return fmt.Sprintf("%g", v)
	default:
		return fmt.Sprintf("'%v'", v)
	}
}
```

- [ ] **Step 4: Run migration tests**

```bash
go test ./internal/store/ -run TestBuild -v
```

Expected: PASS.

- [ ] **Step 5: Write failing tests for Store CRUD operations**

Create `internal/store/store_test.go`:

```go
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

func TestStoreApplySchemas(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer s.Close()

	if err := s.ApplySchemas(testSchemas()); err != nil {
		t.Fatalf("apply schemas: %v", err)
	}

	// Verify table exists by counting
	count, err := s.Count("users")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 rows, got %d", count)
	}
}

func TestStoreInsertAndQuery(t *testing.T) {
	s := newTestStore(t)

	row, err := s.Insert("users", map[string]any{"name": "Alice", "active": true})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if row["id"] == nil {
		t.Error("expected auto-generated id")
	}
	if row["name"] != "Alice" {
		t.Errorf("expected name Alice, got %v", row["name"])
	}

	rows, err := s.Query("SELECT * FROM users WHERE name = ?", []any{"Alice"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0]["name"] != "Alice" {
		t.Errorf("expected Alice, got %v", rows[0]["name"])
	}
}

func TestStoreQueryOne(t *testing.T) {
	s := newTestStore(t)
	s.Insert("users", map[string]any{"name": "Bob"})

	row, err := s.QueryOne("SELECT * FROM users WHERE name = ?", []any{"Bob"})
	if err != nil {
		t.Fatalf("query_one: %v", err)
	}
	if row == nil {
		t.Fatal("expected a row, got nil")
	}
	if row["name"] != "Bob" {
		t.Errorf("expected Bob, got %v", row["name"])
	}

	// No match should return nil, not error
	row, err = s.QueryOne("SELECT * FROM users WHERE name = ?", []any{"Nobody"})
	if err != nil {
		t.Fatalf("query_one no match: %v", err)
	}
	if row != nil {
		t.Errorf("expected nil for no match, got %v", row)
	}
}

func TestStoreUpdate(t *testing.T) {
	s := newTestStore(t)
	inserted, _ := s.Insert("users", map[string]any{"name": "Carol", "score": 50.0})

	updated, err := s.Update("users", inserted["id"], map[string]any{"score": 99.5})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated["score"] != 99.5 {
		t.Errorf("expected score 99.5, got %v", updated["score"])
	}
	if updated["name"] != "Carol" {
		t.Errorf("expected name Carol preserved, got %v", updated["name"])
	}
}

func TestStoreDelete(t *testing.T) {
	s := newTestStore(t)
	inserted, _ := s.Insert("users", map[string]any{"name": "Dave"})

	ok, err := s.Delete("users", inserted["id"])
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if !ok {
		t.Error("expected delete to return true")
	}

	count, _ := s.Count("users")
	if count != 0 {
		t.Errorf("expected 0 rows after delete, got %d", count)
	}

	// Deleting non-existent row returns false
	ok, err = s.Delete("users", 999)
	if err != nil {
		t.Fatalf("delete nonexistent: %v", err)
	}
	if ok {
		t.Error("expected false for non-existent row")
	}
}

func TestStoreSeed(t *testing.T) {
	s := newTestStore(t)

	err := s.Seed("users", []map[string]any{
		{"name": "Seed1", "active": true},
		{"name": "Seed2", "active": false},
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	count, _ := s.Count("users")
	if count != 2 {
		t.Errorf("expected 2 seeded rows, got %d", count)
	}
}

func TestStoreBooleanTypeMapping(t *testing.T) {
	s := newTestStore(t)
	s.Insert("users", map[string]any{"name": "BoolTest", "active": true})

	rows, _ := s.Query("SELECT active FROM users WHERE name = ?", []any{"BoolTest"})
	if len(rows) != 1 {
		t.Fatal("expected 1 row")
	}
	// active should come back as bool true, not int64(1)
	active, ok := rows[0]["active"].(bool)
	if !ok {
		t.Fatalf("expected bool, got %T: %v", rows[0]["active"], rows[0]["active"])
	}
	if !active {
		t.Error("expected active=true")
	}
}

// newTestStore creates an in-memory store with the test schemas applied.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.ApplySchemas(testSchemas()); err != nil {
		t.Fatalf("apply schemas: %v", err)
	}
	return s
}
```

- [ ] **Step 6: Run CRUD tests to verify they fail**

```bash
go test ./internal/store/ -v
```

Expected: FAIL — `Store` type not defined.

- [ ] **Step 7: Implement the Store**

Create `internal/store/store.go`:

```go
package store

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/vibeserve/vibeserve/internal/manifest"
	_ "modernc.org/sqlite"
)

// Store implements the engine.DataStore interface using SQLite.
type Store struct {
	db *sql.DB
	// boolColumns tracks which columns are BOOLEAN for type mapping.
	// Key: "table.column"
	boolColumns map[string]bool
}

// New opens a SQLite database at the given path. Use ":memory:" for tests.
func New(dsn string) (*Store, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// Enable foreign keys and WAL mode for performance.
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db, boolColumns: make(map[string]bool)}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// ApplySchemas creates tables from the manifest schemas.
func (s *Store) ApplySchemas(schemas []manifest.Schema) error {
	for _, schema := range schemas {
		ddl := BuildCreateTableSQL(schema)
		if _, err := s.db.Exec(ddl); err != nil {
			return fmt.Errorf("create table %s: %w", schema.Table, err)
		}
		// Track boolean columns for type mapping on read.
		for _, c := range schema.Columns {
			if c.Type == "BOOLEAN" {
				s.boolColumns[schema.Table+"."+c.Name] = true
			}
		}
	}
	return nil
}

// Seed inserts initial data rows into a table.
func (s *Store) Seed(table string, rows []map[string]any) error {
	for _, row := range rows {
		if _, err := s.Insert(table, row); err != nil {
			return fmt.Errorf("seed %s: %w", table, err)
		}
	}
	return nil
}

// Query executes a parameterized SELECT and returns rows as maps.
func (s *Store) Query(query string, params []any) ([]map[string]any, error) {
	rows, err := s.db.Query(query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return s.scanRows(rows)
}

// QueryOne executes a parameterized SELECT and returns the first row, or nil.
func (s *Store) QueryOne(query string, params []any) (map[string]any, error) {
	rows, err := s.db.Query(query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results, err := s.scanRows(rows)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	return results[0], nil
}

// Insert adds a row and returns it with the auto-generated ID.
func (s *Store) Insert(table string, data map[string]any) (map[string]any, error) {
	cols := make([]string, 0, len(data))
	placeholders := make([]string, 0, len(data))
	vals := make([]any, 0, len(data))

	for k, v := range data {
		cols = append(cols, k)
		placeholders = append(placeholders, "?")
		vals = append(vals, convertForWrite(v))
	}

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		table, strings.Join(cols, ", "), strings.Join(placeholders, ", "))

	result, err := s.db.Exec(query, vals...)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	// Return the full row by reading it back.
	return s.QueryOne(fmt.Sprintf("SELECT * FROM %s WHERE rowid = ?", table), []any{id})
}

// Update modifies a row by its primary key ID and returns the updated row.
func (s *Store) Update(table string, id any, data map[string]any) (map[string]any, error) {
	sets := make([]string, 0, len(data))
	vals := make([]any, 0, len(data)+1)

	for k, v := range data {
		sets = append(sets, fmt.Sprintf("%s = ?", k))
		vals = append(vals, convertForWrite(v))
	}
	vals = append(vals, id)

	query := fmt.Sprintf("UPDATE %s SET %s WHERE id = ?", table, strings.Join(sets, ", "))
	if _, err := s.db.Exec(query, vals...); err != nil {
		return nil, err
	}

	return s.QueryOne(fmt.Sprintf("SELECT * FROM %s WHERE id = ?", table), []any{id})
}

// Delete removes a row by primary key ID. Returns true if a row was deleted.
func (s *Store) Delete(table string, id any) (bool, error) {
	result, err := s.db.Exec(fmt.Sprintf("DELETE FROM %s WHERE id = ?", table), id)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// Count returns the number of rows in a table.
func (s *Store) Count(table string) (int, error) {
	var count int
	err := s.db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&count)
	return count, err
}

// scanRows reads all rows from a sql.Rows into []map[string]any with type mapping.
func (s *Store) scanRows(rows *sql.Rows) ([]map[string]any, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	// Try to determine the table name from the first column's table info.
	// This is a best-effort approach for boolean mapping.
	var results []map[string]any
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}

		row := make(map[string]any, len(cols))
		for i, col := range cols {
			row[col] = values[i]
		}
		results = append(results, row)
	}

	// Apply boolean type mapping: convert int64(0)/int64(1) → bool for known boolean columns.
	s.applyBoolMapping(results, cols)

	return results, rows.Err()
}

// applyBoolMapping converts int64 values to booleans for columns known to be BOOLEAN.
func (s *Store) applyBoolMapping(rows []map[string]any, cols []string) {
	for _, row := range rows {
		for _, col := range cols {
			// Check all tables for this column being boolean.
			// Since we don't always know the table in a SELECT, check all known boolean columns.
			for key := range s.boolColumns {
				parts := strings.SplitN(key, ".", 2)
				if len(parts) == 2 && parts[1] == col {
					if v, ok := row[col].(int64); ok {
						row[col] = v != 0
					}
				}
			}
		}
	}
}

// convertForWrite converts Go values to SQLite-compatible values for writes.
func convertForWrite(v any) any {
	switch val := v.(type) {
	case bool:
		if val {
			return 1
		}
		return 0
	default:
		return v
	}
}
```

- [ ] **Step 8: Run all store tests**

```bash
go test ./internal/store/ -v
```

Expected: all tests PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/store/
git commit -m "feat: add SQLite store with schema creation, CRUD, seeding, and type mapping"
```

---

### Task 6: Runtime — Tengo VM + Stdlib

**Files:**
- Create: `internal/runtime/stdlib.go`
- Create: `internal/runtime/stdlib_test.go`
- Create: `internal/runtime/runtime.go`
- Create: `internal/runtime/runtime_test.go`

- [ ] **Step 1: Write failing tests for stdlib modules**

Create `internal/runtime/stdlib_test.go`:

```go
package runtime

import (
	"testing"
)

func TestBuildRequestModule(t *testing.T) {
	ctx := &RequestContext{
		PathParams: map[string]string{"id": "42"},
		QueryParams: map[string]string{"limit": "10"},
		Body:       map[string]any{"name": "Alice"},
		Headers:    map[string]string{"Content-Type": "application/json"},
		Method:     "POST",
	}

	mod := buildRequestModule(ctx)
	if mod == nil {
		t.Fatal("expected non-nil module")
	}
}

func TestBuildDateModule(t *testing.T) {
	mod := buildDateModule()
	if mod == nil {
		t.Fatal("expected non-nil module")
	}
}

func TestBuildCryptoModule(t *testing.T) {
	mod := buildCryptoModule()
	if mod == nil {
		t.Fatal("expected non-nil module")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/runtime/ -run TestBuild -v
```

Expected: FAIL — types and functions not defined.

- [ ] **Step 3: Implement stdlib modules**

Create `internal/runtime/stdlib.go`:

```go
package runtime

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/d5/tengo/v2"
	"github.com/vibeserve/vibeserve/internal/engine"
)

// RequestContext holds parsed HTTP request data for injection into Tengo.
type RequestContext struct {
	PathParams  map[string]string
	QueryParams map[string]string
	Body        map[string]any
	Headers     map[string]string
	Method      string
}

// ResponseCapture collects the response produced by a Tengo script.
type ResponseCapture struct {
	StatusCode int
	Body       any
	Headers    map[string]string
	Written    bool
}

// --- Request Module ---

func buildRequestModule(ctx *RequestContext) *tengo.ImmutableMap {
	return &tengo.ImmutableMap{
		Value: map[string]tengo.Object{
			"param": &tengo.UserFunction{Name: "param", Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) != 1 {
					return nil, tengo.ErrWrongNumArguments
				}
				name, ok := tengo.ToString(args[0])
				if !ok {
					return nil, fmt.Errorf("param: expected string argument")
				}
				val, exists := ctx.PathParams[name]
				if !exists {
					return tengo.UndefinedValue, nil
				}
				return &tengo.String{Value: val}, nil
			}},
			"query": &tengo.UserFunction{Name: "query", Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) != 1 {
					return nil, tengo.ErrWrongNumArguments
				}
				name, ok := tengo.ToString(args[0])
				if !ok {
					return nil, fmt.Errorf("query: expected string argument")
				}
				val, exists := ctx.QueryParams[name]
				if !exists {
					return tengo.UndefinedValue, nil
				}
				return &tengo.String{Value: val}, nil
			}},
			"body": &tengo.UserFunction{Name: "body", Value: func(args ...tengo.Object) (tengo.Object, error) {
				obj, err := tengo.FromInterface(ctx.Body)
				if err != nil {
					return nil, fmt.Errorf("body: %w", err)
				}
				return obj, nil
			}},
			"header": &tengo.UserFunction{Name: "header", Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) != 1 {
					return nil, tengo.ErrWrongNumArguments
				}
				name, ok := tengo.ToString(args[0])
				if !ok {
					return nil, fmt.Errorf("header: expected string argument")
				}
				val, exists := ctx.Headers[strings.ToLower(name)]
				if !exists {
					return tengo.UndefinedValue, nil
				}
				return &tengo.String{Value: val}, nil
			}},
			"method": &tengo.UserFunction{Name: "method", Value: func(args ...tengo.Object) (tengo.Object, error) {
				return &tengo.String{Value: ctx.Method}, nil
			}},
			"auth": &tengo.UserFunction{Name: "auth", Value: func(args ...tengo.Object) (tengo.Object, error) {
				authHeader, exists := ctx.Headers["authorization"]
				if !exists {
					return tengo.UndefinedValue, nil
				}
				if strings.HasPrefix(authHeader, "Bearer ") {
					token := strings.TrimPrefix(authHeader, "Bearer ")
					// Try Base64-decode as JSON
					decoded, err := base64.StdEncoding.DecodeString(token)
					if err == nil {
						var payload map[string]any
						if json.Unmarshal(decoded, &payload) == nil {
							obj, _ := tengo.FromInterface(payload)
							return obj, nil
						}
					}
					// Fallback: return raw_token
					obj, _ := tengo.FromInterface(map[string]any{"raw_token": token})
					return obj, nil
				}
				return tengo.UndefinedValue, nil
			}},
		},
	}
}

// --- Response Module ---

func buildResponseModule(capture *ResponseCapture) *tengo.ImmutableMap {
	return &tengo.ImmutableMap{
		Value: map[string]tengo.Object{
			"json": &tengo.UserFunction{Name: "json", Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) < 1 || len(args) > 2 {
					return nil, tengo.ErrWrongNumArguments
				}
				capture.Body = tengo.ToInterface(args[0])
				capture.StatusCode = 200
				if len(args) == 2 {
					status, ok := tengo.ToInt(args[1])
					if !ok {
						return nil, fmt.Errorf("json: status must be int")
					}
					capture.StatusCode = status
				}
				capture.Written = true
				return tengo.UndefinedValue, nil
			}},
			"error": &tengo.UserFunction{Name: "error", Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) != 2 {
					return nil, tengo.ErrWrongNumArguments
				}
				status, ok := tengo.ToInt(args[0])
				if !ok {
					return nil, fmt.Errorf("error: status must be int")
				}
				msg, ok := tengo.ToString(args[1])
				if !ok {
					return nil, fmt.Errorf("error: message must be string")
				}
				capture.StatusCode = status
				capture.Body = map[string]any{"error": msg}
				capture.Written = true
				return tengo.UndefinedValue, nil
			}},
			"header": &tengo.UserFunction{Name: "header", Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) != 2 {
					return nil, tengo.ErrWrongNumArguments
				}
				name, _ := tengo.ToString(args[0])
				val, _ := tengo.ToString(args[1])
				if capture.Headers == nil {
					capture.Headers = make(map[string]string)
				}
				capture.Headers[name] = val
				return tengo.UndefinedValue, nil
			}},
			"redirect": &tengo.UserFunction{Name: "redirect", Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) != 1 {
					return nil, tengo.ErrWrongNumArguments
				}
				url, _ := tengo.ToString(args[0])
				capture.StatusCode = 302
				if capture.Headers == nil {
					capture.Headers = make(map[string]string)
				}
				capture.Headers["Location"] = url
				capture.Written = true
				return tengo.UndefinedValue, nil
			}},
		},
	}
}

// --- DB Module ---

func buildDBModule(store engine.DataStore) *tengo.ImmutableMap {
	return &tengo.ImmutableMap{
		Value: map[string]tengo.Object{
			"query": &tengo.UserFunction{Name: "query", Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) != 2 {
					return nil, tengo.ErrWrongNumArguments
				}
				sqlStr, ok := tengo.ToString(args[0])
				if !ok {
					return nil, fmt.Errorf("db.query: sql must be string")
				}
				params := tengoArrayToGoSlice(args[1])
				rows, err := store.Query(sqlStr, params)
				if err != nil {
					return nil, fmt.Errorf("db.query: %w", err)
				}
				obj, err := tengo.FromInterface(rows)
				if err != nil {
					return nil, err
				}
				return obj, nil
			}},
			"query_one": &tengo.UserFunction{Name: "query_one", Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) != 2 {
					return nil, tengo.ErrWrongNumArguments
				}
				sqlStr, ok := tengo.ToString(args[0])
				if !ok {
					return nil, fmt.Errorf("db.query_one: sql must be string")
				}
				params := tengoArrayToGoSlice(args[1])
				row, err := store.QueryOne(sqlStr, params)
				if err != nil {
					return nil, fmt.Errorf("db.query_one: %w", err)
				}
				if row == nil {
					return tengo.UndefinedValue, nil
				}
				obj, err := tengo.FromInterface(row)
				if err != nil {
					return nil, err
				}
				return obj, nil
			}},
			"insert": &tengo.UserFunction{Name: "insert", Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) != 2 {
					return nil, tengo.ErrWrongNumArguments
				}
				table, ok := tengo.ToString(args[0])
				if !ok {
					return nil, fmt.Errorf("db.insert: table must be string")
				}
				data, err := tengoToGoMap(args[1])
				if err != nil {
					return nil, fmt.Errorf("db.insert: %w", err)
				}
				row, err := store.Insert(table, data)
				if err != nil {
					return nil, fmt.Errorf("db.insert: %w", err)
				}
				obj, _ := tengo.FromInterface(row)
				return obj, nil
			}},
			"update": &tengo.UserFunction{Name: "update", Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) != 3 {
					return nil, tengo.ErrWrongNumArguments
				}
				table, _ := tengo.ToString(args[0])
				id := tengo.ToInterface(args[1])
				data, err := tengoToGoMap(args[2])
				if err != nil {
					return nil, fmt.Errorf("db.update: %w", err)
				}
				row, err := store.Update(table, id, data)
				if err != nil {
					return nil, fmt.Errorf("db.update: %w", err)
				}
				obj, _ := tengo.FromInterface(row)
				return obj, nil
			}},
			"delete": &tengo.UserFunction{Name: "delete", Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) != 2 {
					return nil, tengo.ErrWrongNumArguments
				}
				table, _ := tengo.ToString(args[0])
				id := tengo.ToInterface(args[1])
				ok, err := store.Delete(table, id)
				if err != nil {
					return nil, fmt.Errorf("db.delete: %w", err)
				}
				if ok {
					return tengo.TrueValue, nil
				}
				return tengo.FalseValue, nil
			}},
			"count": &tengo.UserFunction{Name: "count", Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) != 1 {
					return nil, tengo.ErrWrongNumArguments
				}
				table, _ := tengo.ToString(args[0])
				n, err := store.Count(table)
				if err != nil {
					return nil, fmt.Errorf("db.count: %w", err)
				}
				return &tengo.Int{Value: int64(n)}, nil
			}},
		},
	}
}

// --- Date Module ---

func buildDateModule() *tengo.ImmutableMap {
	return &tengo.ImmutableMap{
		Value: map[string]tengo.Object{
			"now": &tengo.UserFunction{Name: "now", Value: func(args ...tengo.Object) (tengo.Object, error) {
				return &tengo.String{Value: time.Now().UTC().Format(time.RFC3339)}, nil
			}},
			"diff_days": &tengo.UserFunction{Name: "diff_days", Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) != 2 {
					return nil, tengo.ErrWrongNumArguments
				}
				aStr, _ := tengo.ToString(args[0])
				bStr, _ := tengo.ToString(args[1])
				a, err := time.Parse("2006-01-02", aStr)
				if err != nil {
					a, err = time.Parse(time.RFC3339, aStr)
					if err != nil {
						return nil, fmt.Errorf("diff_days: cannot parse date %q", aStr)
					}
				}
				b, err := time.Parse("2006-01-02", bStr)
				if err != nil {
					b, err = time.Parse(time.RFC3339, bStr)
					if err != nil {
						return nil, fmt.Errorf("diff_days: cannot parse date %q", bStr)
					}
				}
				days := int(b.Sub(a).Hours() / 24)
				return &tengo.Int{Value: int64(days)}, nil
			}},
			"add_days": &tengo.UserFunction{Name: "add_days", Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) != 2 {
					return nil, tengo.ErrWrongNumArguments
				}
				dStr, _ := tengo.ToString(args[0])
				n, _ := tengo.ToInt(args[1])
				d, err := time.Parse("2006-01-02", dStr)
				if err != nil {
					d, err = time.Parse(time.RFC3339, dStr)
					if err != nil {
						return nil, fmt.Errorf("add_days: cannot parse date %q", dStr)
					}
				}
				result := d.AddDate(0, 0, n)
				return &tengo.String{Value: result.Format("2006-01-02")}, nil
			}},
			"format": &tengo.UserFunction{Name: "format", Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) != 2 {
					return nil, tengo.ErrWrongNumArguments
				}
				dStr, _ := tengo.ToString(args[0])
				fmtStr, _ := tengo.ToString(args[1])
				d, err := time.Parse("2006-01-02", dStr)
				if err != nil {
					d, err = time.Parse(time.RFC3339, dStr)
					if err != nil {
						return nil, fmt.Errorf("format: cannot parse date %q", dStr)
					}
				}
				return &tengo.String{Value: d.Format(fmtStr)}, nil
			}},
		},
	}
}

// --- Crypto Module ---

func buildCryptoModule() *tengo.ImmutableMap {
	return &tengo.ImmutableMap{
		Value: map[string]tengo.Object{
			"hash": &tengo.UserFunction{Name: "hash", Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) != 1 {
					return nil, tengo.ErrWrongNumArguments
				}
				s, _ := tengo.ToString(args[0])
				h := sha256.Sum256([]byte(s))
				return &tengo.String{Value: fmt.Sprintf("%x", h)}, nil
			}},
			"uuid": &tengo.UserFunction{Name: "uuid", Value: func(args ...tengo.Object) (tengo.Object, error) {
				uuid := make([]byte, 16)
				rand.Read(uuid)
				uuid[6] = (uuid[6] & 0x0f) | 0x40 // version 4
				uuid[8] = (uuid[8] & 0x3f) | 0x80 // variant 2
				s := fmt.Sprintf("%x-%x-%x-%x-%x", uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:])
				return &tengo.String{Value: s}, nil
			}},
			"random": &tengo.UserFunction{Name: "random", Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) != 2 {
					return nil, tengo.ErrWrongNumArguments
				}
				minVal, _ := tengo.ToInt(args[0])
				maxVal, _ := tengo.ToInt(args[1])
				diff := int64(maxVal - minVal + 1)
				n, _ := rand.Int(rand.Reader, big.NewInt(diff))
				return &tengo.Int{Value: int64(minVal) + n.Int64()}, nil
			}},
		},
	}
}

// --- Log Module ---

func buildLogModule(bus *engine.Bus) *tengo.ImmutableMap {
	emit := func(level string, args ...tengo.Object) (tengo.Object, error) {
		if len(args) != 1 {
			return nil, tengo.ErrWrongNumArguments
		}
		msg, _ := tengo.ToString(args[0])
		if bus != nil {
			bus.Publish(engine.Event{
				Type: engine.EventLogEmitted,
				Data: map[string]string{"level": level, "message": msg},
			})
		}
		return tengo.UndefinedValue, nil
	}
	return &tengo.ImmutableMap{
		Value: map[string]tengo.Object{
			"info":  &tengo.UserFunction{Name: "info", Value: func(args ...tengo.Object) (tengo.Object, error) { return emit("info", args...) }},
			"warn":  &tengo.UserFunction{Name: "warn", Value: func(args ...tengo.Object) (tengo.Object, error) { return emit("warn", args...) }},
			"error": &tengo.UserFunction{Name: "error", Value: func(args ...tengo.Object) (tengo.Object, error) { return emit("error", args...) }},
		},
	}
}

// --- Helpers ---

// tengoArrayToGoSlice converts a Tengo Array to a Go []any for SQL params.
func tengoArrayToGoSlice(obj tengo.Object) []any {
	arr, ok := obj.(*tengo.Array)
	if !ok {
		// Also handle ImmutableArray
		if immArr, ok := obj.(*tengo.ImmutableArray); ok {
			result := make([]any, len(immArr.Value))
			for i, v := range immArr.Value {
				result[i] = tengo.ToInterface(v)
			}
			return result
		}
		return nil
	}
	result := make([]any, len(arr.Value))
	for i, v := range arr.Value {
		result[i] = tengo.ToInterface(v)
	}
	return result
}

// tengoToGoMap converts a Tengo Map to Go map[string]any.
func tengoToGoMap(obj tengo.Object) (map[string]any, error) {
	switch m := obj.(type) {
	case *tengo.Map:
		result := make(map[string]any, len(m.Value))
		for k, v := range m.Value {
			result[k] = tengo.ToInterface(v)
		}
		return result, nil
	case *tengo.ImmutableMap:
		result := make(map[string]any, len(m.Value))
		for k, v := range m.Value {
			result[k] = tengo.ToInterface(v)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("expected map, got %s", obj.TypeName())
	}
}
```

- [ ] **Step 4: Run stdlib tests**

```bash
go test ./internal/runtime/ -run TestBuild -v
```

Expected: PASS.

- [ ] **Step 5: Write failing tests for the Tengo runtime executor**

Create `internal/runtime/runtime_test.go`:

```go
package runtime

import (
	"testing"

	"github.com/vibeserve/vibeserve/internal/engine"
	"github.com/vibeserve/vibeserve/internal/manifest"
	"github.com/vibeserve/vibeserve/internal/store"
)

func newTestRuntime(t *testing.T) (*Runtime, engine.DataStore) {
	t.Helper()
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	err = s.ApplySchemas([]manifest.Schema{{
		Table: "items",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "title", Type: "TEXT", Required: true},
			{Name: "price", Type: "REAL"},
		},
	}})
	if err != nil {
		t.Fatalf("apply schemas: %v", err)
	}
	s.Insert("items", map[string]any{"title": "Widget", "price": 9.99})
	s.Insert("items", map[string]any{"title": "Gadget", "price": 19.99})

	rt := New(nil) // nil bus for tests
	return rt, s
}

func TestExecuteSimpleQuery(t *testing.T) {
	rt, ds := newTestRuntime(t)

	code := `result := db.query("SELECT * FROM items", [])
response.json(result)`

	req := &RequestContext{
		PathParams: map[string]string{}, QueryParams: map[string]string{},
		Body: nil, Headers: map[string]string{}, Method: "GET",
	}
	status, body, _, err := rt.Execute(code, req, ds)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if status != 200 {
		t.Errorf("expected status 200, got %d", status)
	}
	rows, ok := body.([]any)
	if !ok {
		t.Fatalf("expected []any body, got %T", body)
	}
	if len(rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(rows))
	}
}

func TestExecuteInsertAndReturn(t *testing.T) {
	rt, ds := newTestRuntime(t)

	code := `body := request.body()
row := db.insert("items", body)
response.json(row, 201)`

	req := &RequestContext{
		PathParams: map[string]string{}, QueryParams: map[string]string{},
		Body:    map[string]any{"title": "New Item", "price": 5.0},
		Headers: map[string]string{}, Method: "POST",
	}
	status, body, _, err := rt.Execute(code, req, ds)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if status != 201 {
		t.Errorf("expected status 201, got %d", status)
	}
	row, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected map body, got %T", body)
	}
	if row["title"] != "New Item" {
		t.Errorf("expected title 'New Item', got %v", row["title"])
	}
}

func TestExecuteErrorResponse(t *testing.T) {
	rt, ds := newTestRuntime(t)

	code := `response.error(404, "not found")`

	req := &RequestContext{
		PathParams: map[string]string{}, QueryParams: map[string]string{},
		Headers: map[string]string{}, Method: "GET",
	}
	status, body, _, err := rt.Execute(code, req, ds)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if status != 404 {
		t.Errorf("expected status 404, got %d", status)
	}
	m, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected map body, got %T", body)
	}
	if m["error"] != "not found" {
		t.Errorf("expected error 'not found', got %v", m["error"])
	}
}

func TestExecutePathParams(t *testing.T) {
	rt, ds := newTestRuntime(t)

	code := `id := request.param("id")
row := db.query_one("SELECT * FROM items WHERE id = ?", [id])
if row == undefined {
	response.error(404, "not found")
} else {
	response.json(row)
}`

	req := &RequestContext{
		PathParams: map[string]string{"id": "1"}, QueryParams: map[string]string{},
		Headers: map[string]string{}, Method: "GET",
	}
	status, body, _, err := rt.Execute(code, req, ds)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if status != 200 {
		t.Errorf("expected 200, got %d", status)
	}
	row := body.(map[string]any)
	if row["title"] != "Widget" {
		t.Errorf("expected Widget, got %v", row["title"])
	}
}

func TestExecuteNoResponse(t *testing.T) {
	rt, ds := newTestRuntime(t)

	code := `x := 1 + 2` // Script doesn't call response.json()

	req := &RequestContext{
		PathParams: map[string]string{}, QueryParams: map[string]string{},
		Headers: map[string]string{}, Method: "GET",
	}
	status, _, _, err := rt.Execute(code, req, ds)
	if err == nil {
		t.Error("expected error for script that produces no response")
	}
	_ = status
}

func TestExecuteDateDiffDays(t *testing.T) {
	rt, ds := newTestRuntime(t)

	code := `days := date.diff_days("2026-04-01", "2026-04-10")
response.json({"days": days})`

	req := &RequestContext{
		PathParams: map[string]string{}, QueryParams: map[string]string{},
		Headers: map[string]string{}, Method: "GET",
	}
	status, body, _, err := rt.Execute(code, req, ds)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if status != 200 {
		t.Errorf("expected 200, got %d", status)
	}
	m := body.(map[string]any)
	days, ok := m["days"].(int64)
	if !ok {
		t.Fatalf("expected int64, got %T: %v", m["days"], m["days"])
	}
	if days != 9 {
		t.Errorf("expected 9 days, got %d", days)
	}
}
```

- [ ] **Step 6: Run runtime tests to verify they fail**

```bash
go test ./internal/runtime/ -v
```

Expected: FAIL — `Runtime` type not defined.

- [ ] **Step 7: Implement the Tengo runtime**

Create `internal/runtime/runtime.go`:

```go
package runtime

import (
	"fmt"

	"github.com/d5/tengo/v2"
	"github.com/vibeserve/vibeserve/internal/engine"
)

// Runtime implements script execution using the Tengo VM.
type Runtime struct {
	bus *engine.Bus
}

// New creates a Runtime. Bus can be nil (events won't be emitted).
func New(bus *engine.Bus) *Runtime {
	return &Runtime{bus: bus}
}

// Execute runs a Tengo script with the full Vibe Standard Library injected.
// Returns the captured HTTP response (status, body, headers).
func (rt *Runtime) Execute(code string, reqCtx *RequestContext, store engine.DataStore) (int, any, map[string]string, error) {
	capture := &ResponseCapture{StatusCode: 200}

	script := tengo.NewScript([]byte(code))

	// Inject all stdlib modules
	_ = script.Add("db", buildDBModule(store))
	_ = script.Add("request", buildRequestModule(reqCtx))
	_ = script.Add("response", buildResponseModule(capture))
	_ = script.Add("date", buildDateModule())
	_ = script.Add("crypto", buildCryptoModule())
	_ = script.Add("log", buildLogModule(rt.bus))

	// Execute
	_, err := script.Run()
	if err != nil {
		return 500, map[string]any{"error": err.Error()}, nil, fmt.Errorf("script execution: %w", err)
	}

	if !capture.Written {
		return 0, nil, nil, fmt.Errorf("script produced no response (missing response.json() or response.error() call)")
	}

	return capture.StatusCode, capture.Body, capture.Headers, nil
}
```

- [ ] **Step 8: Run all runtime tests**

```bash
go test ./internal/runtime/ -v
```

Expected: all tests PASS. If any Tengo API signatures differ, fix them based on compiler errors.

- [ ] **Step 9: Commit**

```bash
git add internal/runtime/
git commit -m "feat: add Tengo runtime with complete Vibe Standard Library"
```

---

### Task 7: Router — Trie + HTTP Handler + CORS

**Files:**
- Create: `internal/router/trie.go`
- Create: `internal/router/trie_test.go`
- Create: `internal/router/handler.go`
- Create: `internal/router/handler_test.go`
- Create: `internal/router/server.go`

- [ ] **Step 1: Write failing tests for trie route matching**

Create `internal/router/trie_test.go`:

```go
package router

import "testing"

func TestTrieStaticRoutes(t *testing.T) {
	tr := NewTrie()
	tr.Insert("GET", "/users", "list_users")
	tr.Insert("POST", "/users", "create_user")
	tr.Insert("GET", "/vehicles", "list_vehicles")

	script, params, ok := tr.Search("GET", "/users")
	if !ok || script != "list_users" {
		t.Errorf("GET /users: expected list_users, got %q (found=%v)", script, ok)
	}
	if len(params) != 0 {
		t.Errorf("expected no params, got %v", params)
	}

	script, _, ok = tr.Search("POST", "/users")
	if !ok || script != "create_user" {
		t.Errorf("POST /users: expected create_user, got %q", script)
	}

	_, _, ok = tr.Search("DELETE", "/users")
	if ok {
		t.Error("expected no match for DELETE /users")
	}
}

func TestTrieParameterRoutes(t *testing.T) {
	tr := NewTrie()
	tr.Insert("GET", "/users/:id", "get_user")
	tr.Insert("GET", "/users/:id/posts/:post_id", "get_user_post")

	script, params, ok := tr.Search("GET", "/users/42")
	if !ok || script != "get_user" {
		t.Errorf("expected get_user, got %q (found=%v)", script, ok)
	}
	if params["id"] != "42" {
		t.Errorf("expected id=42, got %v", params)
	}

	script, params, ok = tr.Search("GET", "/users/42/posts/7")
	if !ok || script != "get_user_post" {
		t.Errorf("expected get_user_post, got %q", script)
	}
	if params["id"] != "42" || params["post_id"] != "7" {
		t.Errorf("expected id=42 post_id=7, got %v", params)
	}
}

func TestTrieNoMatch(t *testing.T) {
	tr := NewTrie()
	tr.Insert("GET", "/users", "list_users")

	_, _, ok := tr.Search("GET", "/nonexistent")
	if ok {
		t.Error("expected no match for /nonexistent")
	}
}

func TestTrieRemove(t *testing.T) {
	tr := NewTrie()
	tr.Insert("GET", "/users", "list_users")

	tr.Remove("GET", "/users")

	_, _, ok := tr.Search("GET", "/users")
	if ok {
		t.Error("expected no match after remove")
	}
}

func TestTrieRoutes(t *testing.T) {
	tr := NewTrie()
	tr.Insert("GET", "/a", "s1")
	tr.Insert("POST", "/b", "s2")

	routes := tr.Routes()
	if len(routes) != 2 {
		t.Errorf("expected 2 routes, got %d", len(routes))
	}
}
```

- [ ] **Step 2: Run trie tests to verify they fail**

```bash
go test ./internal/router/ -run TestTrie -v
```

Expected: FAIL.

- [ ] **Step 3: Implement the trie**

Create `internal/router/trie.go`:

```go
package router

import "strings"

// RouteEntry holds a matched route's info.
type RouteEntry struct {
	Method string
	Path   string
	Script string
}

type trieNode struct {
	children map[string]*trieNode
	param    string // non-empty if this is a :param node
	// scripts maps HTTP method → script name for this path
	scripts map[string]string
}

// Trie is a prefix-tree for dynamic HTTP route matching.
type Trie struct {
	root *trieNode
}

// NewTrie creates an empty route trie.
func NewTrie() *Trie {
	return &Trie{root: &trieNode{children: make(map[string]*trieNode)}}
}

// Insert registers a route.
func (t *Trie) Insert(method, path, script string) {
	node := t.root
	for _, seg := range splitPath(path) {
		if strings.HasPrefix(seg, ":") {
			// Parameter segment
			paramName := seg[1:]
			child, exists := node.children[":"]
			if !exists {
				child = &trieNode{children: make(map[string]*trieNode), param: paramName}
				node.children[":"] = child
			}
			node = child
		} else {
			child, exists := node.children[seg]
			if !exists {
				child = &trieNode{children: make(map[string]*trieNode)}
				node.children[seg] = child
			}
			node = child
		}
	}
	if node.scripts == nil {
		node.scripts = make(map[string]string)
	}
	node.scripts[method] = script
}

// Search finds the script and path parameters for a method+path.
func (t *Trie) Search(method, path string) (script string, params map[string]string, found bool) {
	params = make(map[string]string)
	node := t.root

	for _, seg := range splitPath(path) {
		// Try exact match first
		if child, ok := node.children[seg]; ok {
			node = child
			continue
		}
		// Try parameter match
		if child, ok := node.children[":"]; ok {
			params[child.param] = seg
			node = child
			continue
		}
		return "", nil, false
	}

	if node.scripts == nil {
		return "", nil, false
	}
	script, found = node.scripts[method]
	if !found {
		return "", nil, false
	}
	return script, params, true
}

// Remove deregisters a route.
func (t *Trie) Remove(method, path string) {
	node := t.root
	for _, seg := range splitPath(path) {
		key := seg
		if strings.HasPrefix(seg, ":") {
			key = ":"
		}
		child, ok := node.children[key]
		if !ok {
			return
		}
		node = child
	}
	if node.scripts != nil {
		delete(node.scripts, method)
	}
}

// Routes returns all registered routes.
func (t *Trie) Routes() []RouteEntry {
	var entries []RouteEntry
	t.collect(t.root, "", &entries)
	return entries
}

func (t *Trie) collect(node *trieNode, prefix string, entries *[]RouteEntry) {
	for method, script := range node.scripts {
		*entries = append(*entries, RouteEntry{Method: method, Path: prefix, Script: script})
	}
	for seg, child := range node.children {
		p := prefix + "/" + seg
		if seg == ":" {
			p = prefix + "/:" + child.param
		}
		t.collect(child, p, entries)
	}
}

func splitPath(path string) []string {
	path = strings.Trim(path, "/")
	if path == "" {
		return nil
	}
	return strings.Split(path, "/")
}
```

- [ ] **Step 4: Run trie tests**

```bash
go test ./internal/router/ -run TestTrie -v
```

Expected: all PASS.

- [ ] **Step 5: Write failing test for HTTP handler**

Create `internal/router/handler_test.go`:

```go
package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vibeserve/vibeserve/internal/engine"
	"github.com/vibeserve/vibeserve/internal/manifest"
	"github.com/vibeserve/vibeserve/internal/runtime"
	"github.com/vibeserve/vibeserve/internal/store"
)

func setupTestHandler(t *testing.T) http.Handler {
	t.Helper()

	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	s.ApplySchemas([]manifest.Schema{{
		Table: "items",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "title", Type: "TEXT", Required: true},
		},
	}})
	s.Insert("items", map[string]any{"title": "Test Item"})

	tr := NewTrie()
	scripts := map[string]string{
		"list_items": `result := db.query("SELECT * FROM items", [])
response.json(result)`,
		"create_item": `body := request.body()
row := db.insert("items", body)
response.json(row, 201)`,
		"get_item": `id := request.param("id")
row := db.query_one("SELECT * FROM items WHERE id = ?", [id])
if row == undefined {
	response.error(404, "not found")
} else {
	response.json(row)
}`,
	}

	tr.Insert("GET", "/items", "list_items")
	tr.Insert("POST", "/items", "create_item")
	tr.Insert("GET", "/items/:id", "get_item")

	rt := runtime.New(nil)
	return NewHandler(tr, scripts, rt, s, true)
}

func TestHandlerListItems(t *testing.T) {
	h := setupTestHandler(t)

	req := httptest.NewRequest("GET", "/items", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("expected JSON content type, got %q", rec.Header().Get("Content-Type"))
	}

	var body []any
	json.NewDecoder(rec.Body).Decode(&body)
	if len(body) != 1 {
		t.Errorf("expected 1 item, got %d", len(body))
	}
}

func TestHandlerCreateItem(t *testing.T) {
	h := setupTestHandler(t)

	req := httptest.NewRequest("POST", "/items", strings.NewReader(`{"title":"New"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != 201 {
		t.Errorf("expected 201, got %d", rec.Code)
	}
}

func TestHandlerGetItemNotFound(t *testing.T) {
	h := setupTestHandler(t)

	req := httptest.NewRequest("GET", "/items/999", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != 404 {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestHandlerRouteNotFound(t *testing.T) {
	h := setupTestHandler(t)

	req := httptest.NewRequest("GET", "/nonexistent", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != 404 {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestHandlerCORSPreflight(t *testing.T) {
	h := setupTestHandler(t)

	req := httptest.NewRequest("OPTIONS", "/items", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != 204 {
		t.Errorf("expected 204 for preflight, got %d", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("expected CORS Allow-Origin header")
	}
}

func TestHandlerCORSHeaders(t *testing.T) {
	h := setupTestHandler(t)

	req := httptest.NewRequest("GET", "/items", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("expected CORS headers on regular response")
	}
}
```

- [ ] **Step 6: Run handler tests to verify they fail**

```bash
go test ./internal/router/ -run TestHandler -v
```

Expected: FAIL.

- [ ] **Step 7: Implement handler and server**

Create `internal/router/handler.go`:

```go
package router

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/vibeserve/vibeserve/internal/engine"
	"github.com/vibeserve/vibeserve/internal/runtime"
)

// NewHandler creates an http.Handler that routes requests through the trie → Tengo runtime.
func NewHandler(trie *Trie, scripts map[string]string, rt *runtime.Runtime, store engine.DataStore, cors bool) http.Handler {
	return &handler{trie: trie, scripts: scripts, rt: rt, store: store, cors: cors}
}

type handler struct {
	trie    *Trie
	scripts map[string]string
	rt      *runtime.Runtime
	store   engine.DataStore
	cors    bool
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// CORS headers
	if h.cors {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	}

	// Handle preflight
	if r.Method == "OPTIONS" {
		w.WriteHeader(204)
		return
	}

	// Route lookup
	scriptName, params, found := h.trie.Search(r.Method, r.URL.Path)
	if !found {
		writeJSON(w, 404, map[string]any{"error": "route not found"})
		return
	}

	code, exists := h.scripts[scriptName]
	if !exists {
		writeJSON(w, 500, map[string]any{"error": "script not found: " + scriptName})
		return
	}

	// Parse request body for POST/PUT/PATCH
	var body map[string]any
	if r.Body != nil && (r.Method == "POST" || r.Method == "PUT" || r.Method == "PATCH") {
		data, _ := io.ReadAll(r.Body)
		if len(data) > 0 {
			json.Unmarshal(data, &body)
		}
	}

	// Parse query params
	queryParams := make(map[string]string)
	for k, v := range r.URL.Query() {
		if len(v) > 0 {
			queryParams[k] = v[0]
		}
	}

	// Build request headers map (lowercase keys)
	headers := make(map[string]string)
	for k, v := range r.Header {
		if len(v) > 0 {
			headers[strings.ToLower(k)] = v[0]
		}
	}

	// Execute script
	reqCtx := &runtime.RequestContext{
		PathParams:  params,
		QueryParams: queryParams,
		Body:        body,
		Headers:     headers,
		Method:      r.Method,
	}

	status, respBody, respHeaders, err := h.rt.Execute(code, reqCtx, h.store)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}

	// Apply response headers from script
	for k, v := range respHeaders {
		w.Header().Set(k, v)
	}

	writeJSON(w, status, respBody)
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
```

Create `internal/router/server.go`:

```go
package router

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"
)

// Server wraps an HTTP server for the VibeServe API.
type Server struct {
	httpServer *http.Server
}

// NewServer creates a Server bound to host:port with the given handler.
func NewServer(host string, port int, handler http.Handler) *Server {
	return &Server{
		httpServer: &http.Server{
			Addr:         fmt.Sprintf("%s:%d", host, port),
			Handler:      handler,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		},
	}
}

// Start begins serving HTTP requests. Blocks until the server stops.
func (s *Server) Start() error {
	log.Printf("VibeServe listening on %s", s.httpServer.Addr)
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// Addr returns the server's address.
func (s *Server) Addr() string {
	return s.httpServer.Addr
}
```

- [ ] **Step 8: Run all router tests**

```bash
go test ./internal/router/ -v
```

Expected: all tests PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/router/
git commit -m "feat: add trie router, HTTP handler with CORS, and server"
```

---

### Task 8: CLI — `vibeserve up` Command

**Files:**
- Modify: `cmd/vibeserve/main.go`

- [ ] **Step 1: Implement the full CLI with `up` subcommand**

Replace `cmd/vibeserve/main.go`:

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/vibeserve/vibeserve/internal/engine"
	"github.com/vibeserve/vibeserve/internal/manifest"
	"github.com/vibeserve/vibeserve/internal/router"
	"github.com/vibeserve/vibeserve/internal/runtime"
	"github.com/vibeserve/vibeserve/internal/store"
)

var version = "0.1.0"

func main() {
	rootCmd := &cobra.Command{
		Use:   "vibeserve",
		Short: "AI-powered stateful API backend from natural language",
		Long:  "VibeServe generates stateful, logic-aware API backends from natural language descriptions.",
	}

	rootCmd.AddCommand(upCmd())
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
	// 1. Load manifest
	m, err := manifest.LoadFromFile(manifestPath)
	if err != nil {
		return fmt.Errorf("load manifest: %w", err)
	}
	log.Printf("Loaded manifest: %s (%d routes, %d schemas)", m.Name, len(m.Routes), len(m.Schemas))

	// 2. Validate manifest
	if err := manifest.Validate(m); err != nil {
		return fmt.Errorf("invalid manifest: %w", err)
	}
	log.Println("Manifest validated (structural + referential + compilation)")

	// 3. Create event bus
	bus := engine.NewBus()
	bus.Subscribe(engine.EventLogEmitted, func(e engine.Event) {
		if data, ok := e.Data.(map[string]string); ok {
			log.Printf("[%s] %s", data["level"], data["message"])
		}
	})

	// 4. Open store
	dbPath := ".vibe/state.db"
	// Ensure .vibe directory exists
	os.MkdirAll(".vibe", 0o755)

	s, err := store.New(dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer s.Close()

	// 5. Apply schemas
	if err := s.ApplySchemas(m.Schemas); err != nil {
		return fmt.Errorf("apply schemas: %w", err)
	}
	for _, schema := range m.Schemas {
		bus.Publish(engine.Event{Type: engine.EventSchemaAltered, Data: schema.Table})
		log.Printf("Schema applied: %s (%d columns)", schema.Table, len(schema.Columns))
	}

	// 6. Seed data
	for _, seed := range m.Seeds {
		if err := s.Seed(seed.Table, seed.Rows); err != nil {
			return fmt.Errorf("seed %s: %w", seed.Table, err)
		}
		bus.Publish(engine.Event{Type: engine.EventDataSeeded, Data: seed.Table})
		log.Printf("Seeded %s: %d rows", seed.Table, len(seed.Rows))
	}

	// 7. Build route trie + script map
	trie := router.NewTrie()
	scripts := make(map[string]string)
	for _, s := range m.Scripts {
		scripts[s.Name] = s.Code
	}
	for _, r := range m.Routes {
		trie.Insert(r.Method, r.Path, r.Script)
		bus.Publish(engine.Event{Type: engine.EventRouteAdded, Data: r.Method + " " + r.Path})
		log.Printf("Route registered: %s %s → %s", r.Method, r.Path, r.Script)
	}

	// 8. Create runtime + handler
	rt := runtime.New(bus)
	handler := router.NewHandler(trie, scripts, rt, s, true)
	srv := router.NewServer(host, port, handler)

	// 9. Save manifest to .vibe/
	manifestData, _ := json.MarshalIndent(m, "", "  ")
	os.WriteFile(".vibe/manifest.json", manifestData, 0o644)

	// 10. Start server with graceful shutdown
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
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./cmd/vibeserve
```

Expected: compiles with no errors.

- [ ] **Step 3: Commit**

```bash
git add cmd/vibeserve/main.go
git commit -m "feat: add CLI with up, routes, and version commands"
```

---

### Task 9: Integration Test — End to End

**Files:**
- Create: `testdata/car_rental_manifest.json`
- Create: `internal/integration_test.go`

- [ ] **Step 1: Create the car rental test fixture**

Create `testdata/car_rental_manifest.json`:

```json
{
  "version": "1.0",
  "name": "malaysia-car-rental",
  "description": "Car rental API with duration-based discounts",
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
        {"name": "vehicle_id", "type": "INTEGER", "references": "vehicles.id"},
        {"name": "customer", "type": "TEXT", "required": true},
        {"name": "start_date", "type": "DATE", "required": true},
        {"name": "end_date", "type": "DATE", "required": true},
        {"name": "total_price", "type": "REAL"},
        {"name": "status", "type": "TEXT", "default": "pending"}
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
    }
  ],
  "scripts": [
    {
      "name": "list_vehicles",
      "code": "result := db.query(\"SELECT * FROM vehicles WHERE available = ?\", [true])\nresponse.json(result)"
    },
    {
      "name": "get_vehicle",
      "code": "id := request.param(\"id\")\nrow := db.query_one(\"SELECT * FROM vehicles WHERE id = ?\", [id])\nif row == undefined {\n  response.error(404, \"Vehicle not found\")\n} else {\n  response.json(row)\n}"
    },
    {
      "name": "create_booking",
      "code": "body := request.body()\nvehicle := db.query_one(\"SELECT * FROM vehicles WHERE id = ?\", [body.vehicle_id])\nif vehicle == undefined {\n  response.error(404, \"Vehicle not found\")\n}\ndays := date.diff_days(body.start_date, body.end_date)\nrate := vehicle.daily_rate\nif days >= 7 {\n  rate = rate * 0.8\n}\ntotal := rate * days\nbooking := db.insert(\"bookings\", {\n  vehicle_id: body.vehicle_id,\n  customer: body.customer,\n  start_date: body.start_date,\n  end_date: body.end_date,\n  total_price: total,\n  status: \"confirmed\"\n})\ndb.update(\"vehicles\", vehicle.id, { available: false })\nresponse.json(booking, 201)"
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

- [ ] **Step 2: Write the integration test**

Create `internal/integration_test.go`:

```go
package internal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vibeserve/vibeserve/internal/engine"
	"github.com/vibeserve/vibeserve/internal/manifest"
	"github.com/vibeserve/vibeserve/internal/router"
	"github.com/vibeserve/vibeserve/internal/runtime"
	"github.com/vibeserve/vibeserve/internal/store"
)

// setupCarRentalServer loads the car rental manifest and returns a test server handler.
func setupCarRentalServer(t *testing.T) http.Handler {
	t.Helper()

	m, err := manifest.LoadFromFile("../testdata/car_rental_manifest.json")
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	if err := manifest.Validate(m); err != nil {
		t.Fatalf("validate: %v", err)
	}

	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	if err := s.ApplySchemas(m.Schemas); err != nil {
		t.Fatalf("apply schemas: %v", err)
	}

	for _, seed := range m.Seeds {
		if err := s.Seed(seed.Table, seed.Rows); err != nil {
			t.Fatalf("seed %s: %v", seed.Table, err)
		}
	}

	trie := router.NewTrie()
	scripts := make(map[string]string)
	for _, sc := range m.Scripts {
		scripts[sc.Name] = sc.Code
	}
	for _, r := range m.Routes {
		trie.Insert(r.Method, r.Path, r.Script)
	}

	rt := runtime.New(engine.NewBus())
	return router.NewHandler(trie, scripts, rt, s, true)
}

func TestIntegration_ListVehicles(t *testing.T) {
	h := setupCarRentalServer(t)

	req := httptest.NewRequest("GET", "/vehicles", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var vehicles []map[string]any
	json.NewDecoder(rec.Body).Decode(&vehicles)

	if len(vehicles) != 3 {
		t.Fatalf("expected 3 vehicles, got %d", len(vehicles))
	}

	// Check Malaysian context in seed data
	found := false
	for _, v := range vehicles {
		if v["make"] == "Perodua" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected Perodua in vehicle list")
	}
}

func TestIntegration_GetVehicle(t *testing.T) {
	h := setupCarRentalServer(t)

	req := httptest.NewRequest("GET", "/vehicles/1", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var vehicle map[string]any
	json.NewDecoder(rec.Body).Decode(&vehicle)

	if vehicle["make"] != "Perodua" {
		t.Errorf("expected Perodua, got %v", vehicle["make"])
	}
}

func TestIntegration_GetVehicle_NotFound(t *testing.T) {
	h := setupCarRentalServer(t)

	req := httptest.NewRequest("GET", "/vehicles/999", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != 404 {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestIntegration_CreateBooking_WithDiscount(t *testing.T) {
	h := setupCarRentalServer(t)

	// Book Perodua Myvi (daily_rate=89) for 10 days → 20% discount
	body := `{
		"vehicle_id": 1,
		"customer": "Ahmad",
		"start_date": "2026-04-10",
		"end_date": "2026-04-20"
	}`

	req := httptest.NewRequest("POST", "/bookings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var booking map[string]any
	json.NewDecoder(rec.Body).Decode(&booking)

	// 10 days * 89 * 0.8 = 712.0
	totalPrice, ok := booking["total_price"].(float64)
	if !ok {
		t.Fatalf("expected float64 total_price, got %T: %v", booking["total_price"], booking["total_price"])
	}
	if totalPrice != 712.0 {
		t.Errorf("expected total_price 712.0 (10 days * 89 * 0.8), got %v", totalPrice)
	}

	if booking["status"] != "confirmed" {
		t.Errorf("expected status 'confirmed', got %v", booking["status"])
	}

	// Verify vehicle is now marked as unavailable
	req2 := httptest.NewRequest("GET", "/vehicles", nil)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)

	var vehicles []map[string]any
	json.NewDecoder(rec2.Body).Decode(&vehicles)

	// Only 2 should be available now (Perodua was booked)
	if len(vehicles) != 2 {
		t.Errorf("expected 2 available vehicles after booking, got %d", len(vehicles))
	}
}

func TestIntegration_CreateBooking_NoDiscount(t *testing.T) {
	h := setupCarRentalServer(t)

	// Book Proton X50 (daily_rate=149) for 3 days → no discount
	body := `{
		"vehicle_id": 2,
		"customer": "Siti",
		"start_date": "2026-04-10",
		"end_date": "2026-04-13"
	}`

	req := httptest.NewRequest("POST", "/bookings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var booking map[string]any
	json.NewDecoder(rec.Body).Decode(&booking)

	// 3 days * 149 = 447.0 (no discount, < 7 days)
	totalPrice, ok := booking["total_price"].(float64)
	if !ok {
		t.Fatalf("expected float64 total_price, got %T: %v", booking["total_price"], booking["total_price"])
	}
	if totalPrice != 447.0 {
		t.Errorf("expected total_price 447.0 (3 days * 149, no discount), got %v", totalPrice)
	}
}

func TestIntegration_CORS(t *testing.T) {
	h := setupCarRentalServer(t)

	req := httptest.NewRequest("OPTIONS", "/vehicles", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != 204 {
		t.Errorf("expected 204 for preflight, got %d", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("missing CORS header")
	}
}
```

- [ ] **Step 3: Run the integration tests**

```bash
go test ./internal/ -v -run TestIntegration
```

Expected: all 6 integration tests PASS — list vehicles, get vehicle, 404, booking with discount (RM712), booking without discount (RM447), CORS.

- [ ] **Step 4: Run full test suite**

```bash
go test ./... -v
```

Expected: ALL tests across all packages PASS.

- [ ] **Step 5: Build and smoke test the binary manually**

```bash
go build -o vibeserve ./cmd/vibeserve
mkdir -p .vibe
cp testdata/car_rental_manifest.json .vibe/manifest.json
./vibeserve routes
```

Expected output:
```
  GET    /vehicles      → list_vehicles
  GET    /vehicles/:id  → get_vehicle
  POST   /bookings      → create_booking
```

- [ ] **Step 6: Commit**

```bash
git add testdata/ internal/integration_test.go
git commit -m "feat: add car rental integration test — end-to-end manifest → API verified"
```

- [ ] **Step 7: Final commit — clean build**

```bash
go vet ./...
go build -o vibeserve ./cmd/vibeserve
rm vibeserve
git add -A
git status
```

If there are any remaining untracked files (go.sum changes, etc.), commit them:

```bash
git add go.sum
git commit -m "chore: update go.sum"
```

---

## Spec Coverage Check

| Spec Section | Covered By |
|---|---|
| 5.1 Module boundaries | Task 1 (structure), all tasks (packages) |
| 5.2 Interface injection | Task 4 (interfaces.go), Task 8 (wiring in main.go) |
| 5.3 Event bus + taxonomy | Task 4 (all event types + pub/sub) |
| 6.0 Manifest schema | Task 2 (types) |
| 6.1 Three-layer validation | Task 3 (structural + referential + compilation) |
| 7.0 Vibe Standard Library | Task 6 (all 6 modules: db, request, response, date, crypto, log) |
| 7.1 SQLite type mapping | Task 5 (boolColumns, convertForWrite, applyBoolMapping) |
| 7.2 CORS | Task 7 (handler with CORS headers + OPTIONS preflight) |
| 10. CLI `up` command | Task 8 |
| 10. CLI `routes` command | Task 8 |
| 10. CLI `version` command | Task 8 |
| 12. Runtime state (.vibe/) | Task 8 (auto-creates .vibe/, saves manifest) |
| 14. Dependencies | Task 1 (go.mod) |
| Phase 1 hard gate (parameterized queries) | Task 5 (all SQL uses `?` params), Task 6 (db.query takes params array) |
| request.auth() with Base64 + raw_token fallback | Task 6 (stdlib.go auth function) |

**Not covered (deferred to Phase 2+):** Manifest differ, snapshots/undo, LLM integration, TUI, web console, export.
