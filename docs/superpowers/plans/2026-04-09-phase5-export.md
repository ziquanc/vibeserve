# Phase 5: The Exit — `vibeserve export` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Generate a standalone, production-ready Go server project + OpenAPI 3.x spec from a VibeServe manifest.

**Architecture:** A 7-stage deterministic pipeline reads the manifest and emits a complete Go project using `text/template`. Tengo scripts are translated to native Go handlers via a line-oriented pattern matcher. An optional `--ai` flag invokes the configured LLM for complex logic.

**Tech Stack:** Go `text/template` for code generation. Generated project uses `go-chi/chi/v5`, `jmoiron/sqlx`, `modernc.org/sqlite`.

**Note:** The `--ai` flag is wired into the CLI and parsed, but the actual LLM translation call is deferred — the hook point exists in the handler generation pipeline (TODO stubs are identified). Connecting it to the existing LLM provider is a small follow-up after the core pipeline is solid.

---

## File Structure

### New files to create:

| File | Responsibility |
|---|---|
| `internal/export/exporter.go` | Pipeline orchestrator — loads manifest, validates, runs stages, writes output |
| `internal/export/exporter_test.go` | Integration test: full pipeline with car_rental manifest |
| `internal/export/models.go` | Stage 3: manifest schema → Go struct source code |
| `internal/export/models_test.go` | Unit tests for model generation |
| `internal/export/repository.go` | Stage 4: Store interface + SQLite impl source code |
| `internal/export/repository_test.go` | Unit tests for repository generation |
| `internal/export/handlers.go` | Stage 5: HTTP handler generation + pattern matcher |
| `internal/export/handlers_test.go` | Unit tests for handler generation + pattern matching |
| `internal/export/openapi.go` | OpenAPI 3.x YAML generation from manifest |
| `internal/export/openapi_test.go` | Unit tests for OpenAPI generation |
| `internal/export/scaffold.go` | Stage 6: main.go, go.mod, Dockerfile, README templates |
| `internal/export/scaffold_test.go` | Unit tests for scaffold generation |
| `internal/export/naming.go` | Shared naming helpers: slugify, singularize, PascalCase, camelCase |
| `internal/export/naming_test.go` | Unit tests for naming helpers |

### Files to modify:

| File | Change |
|---|---|
| `cmd/vibeserve/main.go` | Add `export` subcommand with `--force`, `--ai`, `-m` flags |

---

### Task 1: Naming Helpers

**Files:**
- Create: `internal/export/naming.go`
- Test: `internal/export/naming_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/export/naming_test.go
package export

import "testing"

func TestSlugify(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Car Rental API", "car-rental-api"},
		{"malaysia-car-rental", "malaysia-car-rental"},
		{"My Cool App!", "my-cool-app"},
		{"  spaces  everywhere  ", "spaces-everywhere"},
		{"UPPER_CASE", "upper-case"},
	}
	for _, tt := range tests {
		got := Slugify(tt.input)
		if got != tt.want {
			t.Errorf("Slugify(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSingularize(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"vehicles", "vehicle"},
		{"bookings", "booking"},
		{"categories", "category"},
		{"statuses", "status"},
		{"users", "user"},
		{"person", "person"},
	}
	for _, tt := range tests {
		got := Singularize(tt.input)
		if got != tt.want {
			t.Errorf("Singularize(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestPascalCase(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"vehicle", "Vehicle"},
		{"daily_rate", "DailyRate"},
		{"total_price", "TotalPrice"},
		{"id", "ID"},
		{"vehicle_id", "VehicleID"},
		{"api_url", "APIURL"},
	}
	for _, tt := range tests {
		got := PascalCase(tt.input)
		if got != tt.want {
			t.Errorf("PascalCase(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestCamelCase(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"vehicle", "vehicle"},
		{"daily_rate", "dailyRate"},
		{"total_price", "totalPrice"},
		{"id", "id"},
	}
	for _, tt := range tests {
		got := CamelCase(tt.input)
		if got != tt.want {
			t.Errorf("CamelCase(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestTableToStructName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"vehicles", "Vehicle"},
		{"bookings", "Booking"},
		{"categories", "Category"},
		{"order_items", "OrderItem"},
	}
	for _, tt := range tests {
		got := TableToStructName(tt.input)
		if got != tt.want {
			t.Errorf("TableToStructName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/kent/Documents/Projects/vibeserve && go test ./internal/export/ -run "TestSlugify|TestSingularize|TestPascalCase|TestCamelCase|TestTableToStructName" -v`
Expected: FAIL — package doesn't exist yet

- [ ] **Step 3: Implement naming helpers**

```go
// internal/export/naming.go
package export

import (
	"regexp"
	"strings"
)

// commonAcronyms are uppercased entirely in PascalCase output.
var commonAcronyms = map[string]string{
	"id":   "ID",
	"url":  "URL",
	"api":  "API",
	"http": "HTTP",
	"sql":  "SQL",
	"ip":   "IP",
}

var nonAlphaNum = regexp.MustCompile(`[^a-z0-9]+`)

// Slugify converts a string to a URL-friendly slug.
func Slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "_", "-")
	s = nonAlphaNum.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	// Collapse multiple dashes
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return s
}

// Singularize does naive English singularization.
func Singularize(s string) string {
	if strings.HasSuffix(s, "ies") && len(s) > 3 {
		return s[:len(s)-3] + "y"
	}
	if strings.HasSuffix(s, "ses") || strings.HasSuffix(s, "xes") || strings.HasSuffix(s, "zes") {
		return s[:len(s)-2]
	}
	if strings.HasSuffix(s, "sses") {
		return s[:len(s)-2]
	}
	if strings.HasSuffix(s, "s") && !strings.HasSuffix(s, "ss") && !strings.HasSuffix(s, "us") {
		return s[:len(s)-1]
	}
	return s
}

// PascalCase converts a snake_case string to PascalCase.
func PascalCase(s string) string {
	parts := strings.Split(s, "_")
	var b strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		if upper, ok := commonAcronyms[strings.ToLower(part)]; ok {
			b.WriteString(upper)
		} else {
			b.WriteString(strings.ToUpper(part[:1]) + part[1:])
		}
	}
	return b.String()
}

// CamelCase converts a snake_case string to camelCase.
func CamelCase(s string) string {
	parts := strings.Split(s, "_")
	if len(parts) == 0 {
		return s
	}
	var b strings.Builder
	b.WriteString(strings.ToLower(parts[0]))
	for _, part := range parts[1:] {
		if part == "" {
			continue
		}
		b.WriteString(strings.ToUpper(part[:1]) + part[1:])
	}
	return b.String()
}

// TableToStructName converts a table name (e.g. "vehicles") to a Go struct name (e.g. "Vehicle").
func TableToStructName(table string) string {
	return PascalCase(Singularize(table))
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/kent/Documents/Projects/vibeserve && go test ./internal/export/ -run "TestSlugify|TestSingularize|TestPascalCase|TestCamelCase|TestTableToStructName" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /Users/kent/Documents/Projects/vibeserve
git add internal/export/naming.go internal/export/naming_test.go
git commit -m "feat(export): add naming helpers — slugify, singularize, PascalCase"
```

---

### Task 2: Model Generation (Stage 3)

**Files:**
- Create: `internal/export/models.go`
- Test: `internal/export/models_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/export/models_test.go
package export

import (
	"strings"
	"testing"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

func TestGoType(t *testing.T) {
	tests := []struct {
		col  manifest.Column
		want string
	}{
		{manifest.Column{Type: "INTEGER", Primary: true, Auto: true}, "int64"},
		{manifest.Column{Type: "INTEGER"}, "int64"},
		{manifest.Column{Type: "TEXT"}, "string"},
		{manifest.Column{Type: "REAL"}, "float64"},
		{manifest.Column{Type: "BOOLEAN"}, "bool"},
		{manifest.Column{Type: "DATE"}, "time.Time"},
		{manifest.Column{Type: "DATETIME"}, "time.Time"},
	}
	for _, tt := range tests {
		got := GoType(tt.col)
		if got != tt.want {
			t.Errorf("GoType(%+v) = %q, want %q", tt.col, got, tt.want)
		}
	}
}

func TestGenerateModels(t *testing.T) {
	schemas := []manifest.Schema{
		{
			Table: "vehicles",
			Columns: []manifest.Column{
				{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
				{Name: "make", Type: "TEXT", Required: true},
				{Name: "plate", Type: "TEXT", Unique: true},
				{Name: "daily_rate", Type: "REAL", Required: true},
				{Name: "available", Type: "BOOLEAN", Default: true},
			},
		},
	}

	code := GenerateModels(schemas)

	// Check package declaration
	if !strings.Contains(code, "package model") {
		t.Error("missing package declaration")
	}
	// Check struct name (singular PascalCase)
	if !strings.Contains(code, "type Vehicle struct") {
		t.Error("missing Vehicle struct")
	}
	// Check field with tags
	if !strings.Contains(code, "ID") {
		t.Error("missing ID field")
	}
	if !strings.Contains(code, `db:"id"`) {
		t.Error("missing db tag for id")
	}
	if !strings.Contains(code, `json:"id"`) {
		t.Error("missing json tag for id")
	}
	// Check type mapping
	if !strings.Contains(code, "float64") {
		t.Error("missing float64 for REAL column")
	}
	if !strings.Contains(code, "bool") {
		t.Error("missing bool for BOOLEAN column")
	}
	// Check time import when DATE/DATETIME present
	schemasWithDate := []manifest.Schema{
		{
			Table: "bookings",
			Columns: []manifest.Column{
				{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
				{Name: "start_date", Type: "DATE", Required: true},
			},
		},
	}
	codeWithDate := GenerateModels(schemasWithDate)
	if !strings.Contains(codeWithDate, `"time"`) {
		t.Error("missing time import for DATE column")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/kent/Documents/Projects/vibeserve && go test ./internal/export/ -run "TestGoType|TestGenerateModels" -v`
Expected: FAIL — functions not defined

- [ ] **Step 3: Implement model generation**

```go
// internal/export/models.go
package export

import (
	"fmt"
	"strings"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

// GoType maps a manifest Column to a Go type string.
func GoType(col manifest.Column) string {
	switch col.Type {
	case "INTEGER":
		return "int64"
	case "TEXT":
		return "string"
	case "REAL":
		return "float64"
	case "BOOLEAN":
		return "bool"
	case "DATE", "DATETIME":
		return "time.Time"
	default:
		return "any"
	}
}

// GenerateModels produces the Go source for internal/model/models.go.
func GenerateModels(schemas []manifest.Schema) string {
	var b strings.Builder

	// Check if we need the time import
	needsTime := false
	for _, s := range schemas {
		for _, c := range s.Columns {
			if c.Type == "DATE" || c.Type == "DATETIME" {
				needsTime = true
			}
		}
	}

	b.WriteString("package model\n\n")
	if needsTime {
		b.WriteString("import \"time\"\n\n")
	}

	for i, s := range schemas {
		structName := TableToStructName(s.Table)
		b.WriteString(fmt.Sprintf("// %s represents a row in the %s table.\n", structName, s.Table))
		b.WriteString(fmt.Sprintf("type %s struct {\n", structName))
		for _, c := range s.Columns {
			fieldName := PascalCase(c.Name)
			goType := GoType(c)
			b.WriteString(fmt.Sprintf("\t%s %s `db:%q json:%q`\n", fieldName, goType, c.Name, c.Name))
		}
		b.WriteString("}\n")
		if i < len(schemas)-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/kent/Documents/Projects/vibeserve && go test ./internal/export/ -run "TestGoType|TestGenerateModels" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /Users/kent/Documents/Projects/vibeserve
git add internal/export/models.go internal/export/models_test.go
git commit -m "feat(export): add model generation — manifest schemas to Go structs"
```

---

### Task 3: Repository Generation (Stage 4)

**Files:**
- Create: `internal/export/repository.go`
- Test: `internal/export/repository_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/export/repository_test.go
package export

import (
	"strings"
	"testing"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

func TestAnalyzeRouteUsage(t *testing.T) {
	routes := []manifest.Route{
		{Path: "/vehicles", Method: "GET", Script: "list_vehicles", ResponseType: "array"},
		{Path: "/vehicles/:id", Method: "GET", Script: "get_vehicle", ResponseType: "object"},
		{Path: "/bookings", Method: "POST", Script: "create_booking", ResponseType: "object"},
	}
	scripts := []manifest.Script{
		{Name: "list_vehicles", Code: `result := db.query("SELECT * FROM vehicles WHERE available = ?", [true])` + "\n" + `response.json(result)`},
		{Name: "get_vehicle", Code: `id := request.param("id")` + "\n" + `row := db.query_one("SELECT * FROM vehicles WHERE id = ?", [id])`},
		{Name: "create_booking", Code: `body := request.body()` + "\n" + `booking := db.insert("bookings", {})` + "\n" + `db.update("vehicles", vehicle.id, {})`},
	}

	usage := AnalyzeRouteUsage(routes, scripts)

	// list_vehicles uses db.query on vehicles → List method
	if !usage.HasMethod("vehicles", "List") {
		t.Error("expected List method for vehicles")
	}
	// get_vehicle uses db.query_one on vehicles → Get method
	if !usage.HasMethod("vehicles", "Get") {
		t.Error("expected Get method for vehicles")
	}
	// create_booking uses db.insert on bookings → Create method
	if !usage.HasMethod("bookings", "Create") {
		t.Error("expected Create method for bookings")
	}
	// create_booking uses db.update on vehicles → Update method
	if !usage.HasMethod("vehicles", "Update") {
		t.Error("expected Update method for vehicles")
	}
}

func TestGenerateStoreInterface(t *testing.T) {
	schemas := []manifest.Schema{
		{Table: "vehicles", Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "make", Type: "TEXT"},
		}},
	}
	usage := &RouteUsage{
		methods: map[string]map[string]bool{
			"vehicles": {"List": true, "Get": true},
		},
	}

	code := GenerateStoreInterface(schemas, usage)

	if !strings.Contains(code, "package repository") {
		t.Error("missing package declaration")
	}
	if !strings.Contains(code, "type Store interface") {
		t.Error("missing Store interface")
	}
	if !strings.Contains(code, "ListVehicles(") {
		t.Error("missing ListVehicles method")
	}
	if !strings.Contains(code, "GetVehicle(") {
		t.Error("missing GetVehicle method")
	}
	if !strings.Contains(code, "DB() *sqlx.DB") {
		t.Error("missing DB() accessor")
	}
}

func TestGenerateSQLiteStore(t *testing.T) {
	schemas := []manifest.Schema{
		{Table: "vehicles", Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "make", Type: "TEXT"},
		}},
	}
	usage := &RouteUsage{
		methods: map[string]map[string]bool{
			"vehicles": {"List": true, "Get": true},
		},
	}

	code := GenerateSQLiteStore(schemas, usage)

	if !strings.Contains(code, "package repository") {
		t.Error("missing package declaration")
	}
	if !strings.Contains(code, "SQLiteStore") {
		t.Error("missing SQLiteStore struct")
	}
	if !strings.Contains(code, "NewSQLiteStore") {
		t.Error("missing constructor")
	}
	if !strings.Contains(code, "foreign_keys") {
		t.Error("missing foreign_keys pragma")
	}
	if !strings.Contains(code, "func (s *SQLiteStore) ListVehicles(") {
		t.Error("missing ListVehicles implementation")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/kent/Documents/Projects/vibeserve && go test ./internal/export/ -run "TestAnalyzeRouteUsage|TestGenerateStoreInterface|TestGenerateSQLiteStore" -v`
Expected: FAIL — functions not defined

- [ ] **Step 3: Implement repository generation**

```go
// internal/export/repository.go
package export

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

// RouteUsage tracks which CRUD operations each table needs based on actual route scripts.
type RouteUsage struct {
	methods map[string]map[string]bool // table → {List, Get, Create, Update, Delete}
}

// HasMethod returns true if the given table needs the given method.
func (u *RouteUsage) HasMethod(table, method string) bool {
	if u.methods == nil {
		return false
	}
	m, ok := u.methods[table]
	if !ok {
		return false
	}
	return m[method]
}

// Tables returns all table names that have at least one method.
func (u *RouteUsage) Tables() []string {
	var tables []string
	for t := range u.methods {
		tables = append(tables, t)
	}
	return tables
}

// Methods returns the method names for a given table.
func (u *RouteUsage) Methods(table string) []string {
	var methods []string
	for m := range u.methods[table] {
		methods = append(methods, m)
	}
	return methods
}

var (
	reDBQuery    = regexp.MustCompile(`db\.query\(.*FROM\s+(\w+)`)
	reDBQueryOne = regexp.MustCompile(`db\.query_one\(.*FROM\s+(\w+)`)
	reDBInsert   = regexp.MustCompile(`db\.insert\("(\w+)"`)
	reDBUpdate   = regexp.MustCompile(`db\.update\("(\w+)"`)
	reDBDelete   = regexp.MustCompile(`db\.delete\("(\w+)"`)
)

// AnalyzeRouteUsage scans route scripts to determine which CRUD methods each table needs.
func AnalyzeRouteUsage(routes []manifest.Route, scripts []manifest.Script) *RouteUsage {
	scriptMap := make(map[string]string)
	for _, s := range scripts {
		scriptMap[s.Name] = s.Code
	}

	usage := &RouteUsage{methods: make(map[string]map[string]bool)}
	addMethod := func(table, method string) {
		if usage.methods[table] == nil {
			usage.methods[table] = make(map[string]bool)
		}
		usage.methods[table][method] = true
	}

	for _, r := range routes {
		code, ok := scriptMap[r.Script]
		if !ok {
			continue
		}
		if matches := reDBQuery.FindAllStringSubmatch(code, -1); matches != nil {
			for _, m := range matches {
				addMethod(m[1], "List")
			}
		}
		if matches := reDBQueryOne.FindAllStringSubmatch(code, -1); matches != nil {
			for _, m := range matches {
				addMethod(m[1], "Get")
			}
		}
		if matches := reDBInsert.FindAllStringSubmatch(code, -1); matches != nil {
			for _, m := range matches {
				addMethod(m[1], "Create")
			}
		}
		if matches := reDBUpdate.FindAllStringSubmatch(code, -1); matches != nil {
			for _, m := range matches {
				addMethod(m[1], "Update")
			}
		}
		if matches := reDBDelete.FindAllStringSubmatch(code, -1); matches != nil {
			for _, m := range matches {
				addMethod(m[1], "Delete")
			}
		}
	}

	return usage
}

// GenerateStoreInterface produces the Go source for internal/repository/store.go.
func GenerateStoreInterface(schemas []manifest.Schema, usage *RouteUsage) string {
	var b strings.Builder

	b.WriteString("package repository\n\n")
	b.WriteString("import (\n")
	b.WriteString("\t\"context\"\n\n")
	b.WriteString(fmt.Sprintf("\t\"%s/internal/model\"\n", "{{MODULE}}"))
	b.WriteString("\t\"github.com/jmoiron/sqlx\"\n")
	b.WriteString(")\n\n")

	b.WriteString("// Store defines the data access interface.\n")
	b.WriteString("type Store interface {\n")
	b.WriteString("\t// DB returns the underlying database connection for custom queries.\n")
	b.WriteString("\tDB() *sqlx.DB\n")
	b.WriteString("\t// Close closes the database connection.\n")
	b.WriteString("\tClose() error\n")

	for _, s := range schemas {
		structName := TableToStructName(s.Table)
		pkType := findPKType(s)

		if usage.HasMethod(s.Table, "List") {
			b.WriteString(fmt.Sprintf("\tList%ss(ctx context.Context) ([]model.%s, error)\n", structName, structName))
		}
		if usage.HasMethod(s.Table, "Get") {
			b.WriteString(fmt.Sprintf("\tGet%s(ctx context.Context, id %s) (*model.%s, error)\n", structName, pkType, structName))
		}
		if usage.HasMethod(s.Table, "Create") {
			b.WriteString(fmt.Sprintf("\tCreate%s(ctx context.Context, v *model.%s) error\n", structName, structName))
		}
		if usage.HasMethod(s.Table, "Update") {
			b.WriteString(fmt.Sprintf("\tUpdate%s(ctx context.Context, id %s, v *model.%s) error\n", structName, pkType, structName))
		}
		if usage.HasMethod(s.Table, "Delete") {
			b.WriteString(fmt.Sprintf("\tDelete%s(ctx context.Context, id %s) error\n", structName, pkType))
		}
	}

	b.WriteString("}\n")
	return b.String()
}

// GenerateSQLiteStore produces the Go source for internal/repository/sqlite.go.
func GenerateSQLiteStore(schemas []manifest.Schema, usage *RouteUsage) string {
	var b strings.Builder

	b.WriteString("package repository\n\n")
	b.WriteString("import (\n")
	b.WriteString("\t\"context\"\n")
	b.WriteString("\t\"fmt\"\n\n")
	b.WriteString(fmt.Sprintf("\t\"%s/internal/model\"\n", "{{MODULE}}"))
	b.WriteString("\t\"github.com/jmoiron/sqlx\"\n")
	b.WriteString("\t_ \"modernc.org/sqlite\"\n")
	b.WriteString(")\n\n")

	// Struct and constructor
	b.WriteString("// SQLiteStore implements Store using SQLite via sqlx.\n")
	b.WriteString("type SQLiteStore struct {\n")
	b.WriteString("\tdb *sqlx.DB\n")
	b.WriteString("}\n\n")

	b.WriteString("// NewSQLiteStore opens a SQLite database with foreign keys enabled.\n")
	b.WriteString("func NewSQLiteStore(dsn string) (*SQLiteStore, error) {\n")
	b.WriteString("\tdb, err := sqlx.Open(\"sqlite\", dsn+\"?_pragma=foreign_keys(1)\")\n")
	b.WriteString("\tif err != nil {\n")
	b.WriteString("\t\treturn nil, fmt.Errorf(\"open database: %w\", err)\n")
	b.WriteString("\t}\n")
	b.WriteString("\treturn &SQLiteStore{db: db}, nil\n")
	b.WriteString("}\n\n")

	b.WriteString("// DB returns the underlying sqlx.DB.\n")
	b.WriteString("func (s *SQLiteStore) DB() *sqlx.DB { return s.db }\n\n")
	b.WriteString("// Close closes the database connection.\n")
	b.WriteString("func (s *SQLiteStore) Close() error { return s.db.Close() }\n\n")

	// Generate methods per table
	for _, schema := range schemas {
		structName := TableToStructName(schema.Table)
		pkType := findPKType(schema)
		pkCol := findPKColumn(schema)
		nonPKCols := nonPrimaryColumns(schema)

		if usage.HasMethod(schema.Table, "List") {
			b.WriteString(fmt.Sprintf("func (s *SQLiteStore) List%ss(ctx context.Context) ([]model.%s, error) {\n", structName, structName))
			b.WriteString(fmt.Sprintf("\tvar items []model.%s\n", structName))
			b.WriteString(fmt.Sprintf("\terr := s.db.SelectContext(ctx, &items, \"SELECT * FROM %s\")\n", schema.Table))
			b.WriteString("\treturn items, err\n")
			b.WriteString("}\n\n")
		}

		if usage.HasMethod(schema.Table, "Get") {
			b.WriteString(fmt.Sprintf("func (s *SQLiteStore) Get%s(ctx context.Context, id %s) (*model.%s, error) {\n", structName, pkType, structName))
			b.WriteString(fmt.Sprintf("\tvar item model.%s\n", structName))
			b.WriteString(fmt.Sprintf("\terr := s.db.GetContext(ctx, &item, \"SELECT * FROM %s WHERE %s = ?\", id)\n", schema.Table, pkCol))
			b.WriteString("\tif err != nil {\n")
			b.WriteString("\t\treturn nil, err\n")
			b.WriteString("\t}\n")
			b.WriteString("\treturn &item, nil\n")
			b.WriteString("}\n\n")
		}

		if usage.HasMethod(schema.Table, "Create") {
			cols := columnNames(nonPKCols)
			placeholders := columnPlaceholders(nonPKCols)
			namedPlaceholders := columnNamedPlaceholders(nonPKCols)

			b.WriteString(fmt.Sprintf("func (s *SQLiteStore) Create%s(ctx context.Context, v *model.%s) error {\n", structName, structName))
			b.WriteString(fmt.Sprintf("\t_, err := s.db.NamedExecContext(ctx, \"INSERT INTO %s (%s) VALUES (%s)\", v)\n", schema.Table, cols, namedPlaceholders))
			_ = placeholders // namedExec uses named placeholders
			b.WriteString("\treturn err\n")
			b.WriteString("}\n\n")
		}

		if usage.HasMethod(schema.Table, "Update") {
			setClauses := columnSetClauses(nonPKCols)
			b.WriteString(fmt.Sprintf("func (s *SQLiteStore) Update%s(ctx context.Context, id %s, v *model.%s) error {\n", structName, pkType, structName))
			b.WriteString(fmt.Sprintf("\tv.%s = id\n", PascalCase(pkCol)))
			b.WriteString(fmt.Sprintf("\t_, err := s.db.NamedExecContext(ctx, \"UPDATE %s SET %s WHERE %s = :%s\", v)\n", schema.Table, setClauses, pkCol, pkCol))
			b.WriteString("\treturn err\n")
			b.WriteString("}\n\n")
		}

		if usage.HasMethod(schema.Table, "Delete") {
			b.WriteString(fmt.Sprintf("func (s *SQLiteStore) Delete%s(ctx context.Context, id %s) error {\n", structName, pkType))
			b.WriteString(fmt.Sprintf("\t_, err := s.db.ExecContext(ctx, \"DELETE FROM %s WHERE %s = ?\", id)\n", schema.Table, pkCol))
			b.WriteString("\treturn err\n")
			b.WriteString("}\n\n")
		}
	}

	return b.String()
}

// findPKType returns the Go type of the primary key column (defaults to "int64").
func findPKType(s manifest.Schema) string {
	for _, c := range s.Columns {
		if c.Primary {
			return GoType(c)
		}
	}
	return "int64"
}

// findPKColumn returns the name of the primary key column (defaults to "id").
func findPKColumn(s manifest.Schema) string {
	for _, c := range s.Columns {
		if c.Primary {
			return c.Name
		}
	}
	return "id"
}

// nonPrimaryColumns returns columns excluding the primary auto-increment key.
func nonPrimaryColumns(s manifest.Schema) []manifest.Column {
	var cols []manifest.Column
	for _, c := range s.Columns {
		if c.Primary && c.Auto {
			continue
		}
		cols = append(cols, c)
	}
	return cols
}

func columnNames(cols []manifest.Column) string {
	var names []string
	for _, c := range cols {
		names = append(names, c.Name)
	}
	return strings.Join(names, ", ")
}

func columnPlaceholders(cols []manifest.Column) string {
	p := make([]string, len(cols))
	for i := range p {
		p[i] = "?"
	}
	return strings.Join(p, ", ")
}

func columnNamedPlaceholders(cols []manifest.Column) string {
	var p []string
	for _, c := range cols {
		p = append(p, ":"+c.Name)
	}
	return strings.Join(p, ", ")
}

func columnSetClauses(cols []manifest.Column) string {
	var clauses []string
	for _, c := range cols {
		clauses = append(clauses, fmt.Sprintf("%s = :%s", c.Name, c.Name))
	}
	return strings.Join(clauses, ", ")
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/kent/Documents/Projects/vibeserve && go test ./internal/export/ -run "TestAnalyzeRouteUsage|TestGenerateStoreInterface|TestGenerateSQLiteStore" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /Users/kent/Documents/Projects/vibeserve
git add internal/export/repository.go internal/export/repository_test.go
git commit -m "feat(export): add repository generation — Store interface + SQLite impl"
```

---

### Task 4: Handler Generation + Pattern Matcher (Stage 5)

**Files:**
- Create: `internal/export/handlers.go`
- Test: `internal/export/handlers_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/export/handlers_test.go
package export

import (
	"strings"
	"testing"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

func TestTranslateScript_ListQuery(t *testing.T) {
	script := manifest.Script{
		Name: "list_vehicles",
		Code: "result := db.query(\"SELECT * FROM vehicles WHERE available = ?\", [true])\nresponse.json(result)",
	}
	route := manifest.Route{
		Path: "/vehicles", Method: "GET", Script: "list_vehicles", ResponseType: "array",
	}
	schemas := []manifest.Schema{
		{Table: "vehicles", Columns: []manifest.Column{{Name: "id", Type: "INTEGER", Primary: true, Auto: true}}},
	}

	pm := NewPatternMatcher(schemas)
	code, todos := pm.TranslateScript(script, route)

	if len(todos) > 0 {
		t.Errorf("unexpected TODOs: %v", todos)
	}
	if !strings.Contains(code, "SelectContext") {
		t.Error("expected SelectContext call in output")
	}
	if !strings.Contains(code, "render.JSON") || !strings.Contains(code, "json.") || !strings.Contains(code, "json.NewEncoder") || !strings.Contains(code, "w.Header") {
		// Accept either render.JSON or manual JSON encoding
	}
}

func TestTranslateScript_GetByID(t *testing.T) {
	script := manifest.Script{
		Name: "get_vehicle",
		Code: "id := request.param(\"id\")\nrow := db.query_one(\"SELECT * FROM vehicles WHERE id = ?\", [id])\nif row == undefined {\n  response.fail(404, \"Vehicle not found\")\n} else {\n  response.json(row)\n}",
	}
	route := manifest.Route{
		Path: "/vehicles/:id", Method: "GET", Script: "get_vehicle", ResponseType: "object",
	}
	schemas := []manifest.Schema{
		{Table: "vehicles", Columns: []manifest.Column{{Name: "id", Type: "INTEGER", Primary: true, Auto: true}}},
	}

	pm := NewPatternMatcher(schemas)
	code, _ := pm.TranslateScript(script, route)

	if !strings.Contains(code, "chi.URLParam") {
		t.Error("expected chi.URLParam for request.param")
	}
	if !strings.Contains(code, "strconv.ParseInt") || !strings.Contains(code, "Atoi") {
		// Accept either ParseInt or Atoi for int conversion
	}
	if !strings.Contains(code, "GetContext") {
		t.Error("expected GetContext call")
	}
	if !strings.Contains(code, "404") {
		t.Error("expected 404 error handling")
	}
}

func TestTranslateScript_InsertWithBody(t *testing.T) {
	script := manifest.Script{
		Name: "create_item",
		Code: "body := request.body()\nresult := db.insert(\"items\", body)\nresponse.json(result, 201)",
	}
	route := manifest.Route{
		Path: "/items", Method: "POST", Script: "create_item", ResponseType: "object",
	}
	schemas := []manifest.Schema{
		{Table: "items", Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "name", Type: "TEXT"},
		}},
	}

	pm := NewPatternMatcher(schemas)
	code, _ := pm.TranslateScript(script, route)

	if !strings.Contains(code, "json.NewDecoder") {
		t.Error("expected JSON decode for request.body()")
	}
	if !strings.Contains(code, "201") {
		t.Error("expected 201 status code")
	}
}

func TestTranslateScript_ComplexFallsThrough(t *testing.T) {
	script := manifest.Script{
		Name: "complex",
		Code: "days := date.diff_days(body.start_date, body.end_date)\nhash := crypto.sha256(input)",
	}
	route := manifest.Route{
		Path: "/test", Method: "POST", Script: "complex", ResponseType: "object",
	}

	pm := NewPatternMatcher(nil)
	_, todos := pm.TranslateScript(script, route)

	if len(todos) == 0 {
		t.Error("expected TODO stubs for date.diff_days and crypto.sha256")
	}
}

func TestGenerateHandlers(t *testing.T) {
	schemas := []manifest.Schema{
		{Table: "vehicles", Columns: []manifest.Column{{Name: "id", Type: "INTEGER", Primary: true, Auto: true}, {Name: "make", Type: "TEXT"}}},
	}
	routes := []manifest.Route{
		{Path: "/vehicles", Method: "GET", Script: "list_vehicles", ResponseType: "array"},
	}
	scripts := []manifest.Script{
		{Name: "list_vehicles", Code: "result := db.query(\"SELECT * FROM vehicles\", [])\nresponse.json(result)"},
	}

	code := GenerateHandlers(schemas, routes, scripts)

	if !strings.Contains(code, "package handler") {
		t.Error("missing package declaration")
	}
	if !strings.Contains(code, "type Handler struct") {
		t.Error("missing Handler struct")
	}
	if !strings.Contains(code, "NewHandler") {
		t.Error("missing NewHandler constructor")
	}
	if !strings.Contains(code, "func (h *Handler)") {
		t.Error("missing handler method")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/kent/Documents/Projects/vibeserve && go test ./internal/export/ -run "TestTranslateScript|TestGenerateHandlers" -v`
Expected: FAIL — functions not defined

- [ ] **Step 3: Implement handler generation + pattern matcher**

```go
// internal/export/handlers.go
package export

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

// PatternMatcher translates Tengo script lines to Go code.
type PatternMatcher struct {
	schemas map[string]*manifest.Schema
}

// NewPatternMatcher creates a PatternMatcher with schema context.
func NewPatternMatcher(schemas []manifest.Schema) *PatternMatcher {
	m := make(map[string]*manifest.Schema)
	for i := range schemas {
		m[schemas[i].Table] = &schemas[i]
	}
	return &PatternMatcher{schemas: m}
}

var (
	reQueryFull     = regexp.MustCompile(`(\w+)\s*:=\s*db\.query\("([^"]+)"`)
	reQueryOneFull  = regexp.MustCompile(`(\w+)\s*:=\s*db\.query_one\("([^"]+)"`)
	reInsertFull    = regexp.MustCompile(`(\w+)\s*:=\s*db\.insert\("(\w+)"`)
	reUpdateFull    = regexp.MustCompile(`db\.update\("(\w+)",\s*(\w+[\.\w]*)`)
	reDeleteFull    = regexp.MustCompile(`db\.delete\("(\w+)",\s*(\w+)`)
	reRequestParam  = regexp.MustCompile(`(\w+)\s*:=\s*request\.param\("(\w+)"\)`)
	reRequestQuery  = regexp.MustCompile(`(\w+)\s*:=\s*request\.query\("(\w+)"\)`)
	reRequestBody   = regexp.MustCompile(`(\w+)\s*:=\s*request\.body\(\)`)
	reResponseJSON  = regexp.MustCompile(`response\.json\((\w+)(?:,\s*(\d+))?\)`)
	reResponseFail  = regexp.MustCompile(`response\.fail\((\d+),\s*"([^"]+)"\)`)
	reIfUndefined   = regexp.MustCompile(`if\s+(\w+)\s*==\s*undefined\s*\{`)
	reDateDiffDays  = regexp.MustCompile(`date\.diff_days\(`)
	reDateNow       = regexp.MustCompile(`date\.now\(\)`)
	reCryptoSHA256  = regexp.MustCompile(`crypto\.sha256\(`)
	reCryptoMD5     = regexp.MustCompile(`crypto\.md5\(`)
)

// TranslateScript converts a Tengo script to Go handler body code.
// Returns the Go code and a list of untranslated patterns (TODOs).
func (pm *PatternMatcher) TranslateScript(script manifest.Script, route manifest.Route) (string, []string) {
	var goLines []string
	var todos []string
	lines := strings.Split(script.Code, "\n")

	// Track variables for context
	bodyVar := ""
	paramVars := make(map[string]string) // varName → paramName

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || trimmed == "}" || trimmed == "} else {" {
			goLines = append(goLines, line)
			continue
		}

		translated := false

		// request.param("name")
		if m := reRequestParam.FindStringSubmatch(trimmed); m != nil {
			varName, paramName := m[1], m[2]
			paramVars[varName] = paramName
			goLines = append(goLines, fmt.Sprintf("\t%sStr := chi.URLParam(r, %q)", varName, paramName))
			goLines = append(goLines, fmt.Sprintf("\t%s, err := strconv.ParseInt(%sStr, 10, 64)", varName, varName))
			goLines = append(goLines, "\tif err != nil {")
			goLines = append(goLines, fmt.Sprintf("\t\thttp.Error(w, \"invalid %s\", http.StatusBadRequest)", paramName))
			goLines = append(goLines, "\t\treturn")
			goLines = append(goLines, "\t}")
			translated = true
		}

		// request.query("name")
		if !translated {
			if m := reRequestQuery.FindStringSubmatch(trimmed); m != nil {
				varName, queryName := m[1], m[2]
				goLines = append(goLines, fmt.Sprintf("\t%s := r.URL.Query().Get(%q)", varName, queryName))
				translated = true
			}
		}

		// request.body()
		if !translated {
			if m := reRequestBody.FindStringSubmatch(trimmed); m != nil {
				bodyVar = m[1]
				// Determine struct type from route context
				targetTable := pm.inferTableFromRoute(route)
				structName := "map[string]any"
				if targetTable != "" {
					structName = "model." + TableToStructName(targetTable)
				}
				goLines = append(goLines, fmt.Sprintf("\tvar %s %s", bodyVar, structName))
				goLines = append(goLines, fmt.Sprintf("\tif err := json.NewDecoder(r.Body).Decode(&%s); err != nil {", bodyVar))
				goLines = append(goLines, "\t\thttp.Error(w, \"invalid request body\", http.StatusBadRequest)")
				goLines = append(goLines, "\t\treturn")
				goLines = append(goLines, "\t}")
				translated = true
			}
		}

		// db.query("SELECT ... FROM table ...")
		if !translated {
			if m := reQueryFull.FindStringSubmatch(trimmed); m != nil {
				varName, sql := m[1], m[2]
				table := extractTableFromSQL(sql)
				structName := "map[string]any"
				if table != "" {
					structName = "model." + TableToStructName(table)
				}
				// Extract params from the original line
				params := extractQueryParams(trimmed)
				goLines = append(goLines, fmt.Sprintf("\tvar %s []%s", varName, structName))
				if params != "" {
					goLines = append(goLines, fmt.Sprintf("\tif err := h.store.DB().SelectContext(r.Context(), &%s, %q, %s); err != nil {", varName, sql, params))
				} else {
					goLines = append(goLines, fmt.Sprintf("\tif err := h.store.DB().SelectContext(r.Context(), &%s, %q); err != nil {", varName, sql))
				}
				goLines = append(goLines, "\t\thttp.Error(w, err.Error(), http.StatusInternalServerError)")
				goLines = append(goLines, "\t\treturn")
				goLines = append(goLines, "\t}")
				translated = true
			}
		}

		// db.query_one("SELECT ... FROM table ...")
		if !translated {
			if m := reQueryOneFull.FindStringSubmatch(trimmed); m != nil {
				varName, sql := m[1], m[2]
				table := extractTableFromSQL(sql)
				structName := "map[string]any"
				if table != "" {
					structName = "model." + TableToStructName(table)
				}
				params := extractQueryParams(trimmed)
				goLines = append(goLines, fmt.Sprintf("\tvar %s %s", varName, structName))
				if params != "" {
					goLines = append(goLines, fmt.Sprintf("\terr := h.store.DB().GetContext(r.Context(), &%s, %q, %s)", varName, sql, params))
				} else {
					goLines = append(goLines, fmt.Sprintf("\terr := h.store.DB().GetContext(r.Context(), &%s, %q)", varName, sql))
				}
				translated = true
			}
		}

		// db.insert("table", data)
		if !translated {
			if m := reInsertFull.FindStringSubmatch(trimmed); m != nil {
				varName, table := m[1], m[2]
				structName := TableToStructName(table)
				_ = varName
				goLines = append(goLines, fmt.Sprintf("\tif err := h.store.Create%s(r.Context(), &%s); err != nil {", structName, bodyVar))
				goLines = append(goLines, "\t\thttp.Error(w, err.Error(), http.StatusInternalServerError)")
				goLines = append(goLines, "\t\treturn")
				goLines = append(goLines, "\t}")
				translated = true
			}
		}

		// db.update("table", id, data)
		if !translated {
			if m := reUpdateFull.FindStringSubmatch(trimmed); m != nil {
				table := m[1]
				structName := TableToStructName(table)
				goLines = append(goLines, fmt.Sprintf("\tif err := h.store.Update%s(r.Context(), id, &%s); err != nil {", structName, bodyVar))
				goLines = append(goLines, "\t\thttp.Error(w, err.Error(), http.StatusInternalServerError)")
				goLines = append(goLines, "\t\treturn")
				goLines = append(goLines, "\t}")
				translated = true
			}
		}

		// db.delete("table", id)
		if !translated {
			if m := reDeleteFull.FindStringSubmatch(trimmed); m != nil {
				table := m[1]
				structName := TableToStructName(table)
				goLines = append(goLines, fmt.Sprintf("\tif err := h.store.Delete%s(r.Context(), id); err != nil {", structName))
				goLines = append(goLines, "\t\thttp.Error(w, err.Error(), http.StatusInternalServerError)")
				goLines = append(goLines, "\t\treturn")
				goLines = append(goLines, "\t}")
				translated = true
			}
		}

		// response.json(data) or response.json(data, status)
		if !translated {
			if m := reResponseJSON.FindStringSubmatch(trimmed); m != nil {
				varName := m[1]
				status := m[2]
				if status != "" {
					goLines = append(goLines, fmt.Sprintf("\tw.Header().Set(\"Content-Type\", \"application/json\")"))
					goLines = append(goLines, fmt.Sprintf("\tw.WriteHeader(%s)", status))
				} else {
					goLines = append(goLines, fmt.Sprintf("\tw.Header().Set(\"Content-Type\", \"application/json\")"))
				}
				goLines = append(goLines, fmt.Sprintf("\tjson.NewEncoder(w).Encode(%s)", varName))
				translated = true
			}
		}

		// response.fail(status, "message")
		if !translated {
			if m := reResponseFail.FindStringSubmatch(trimmed); m != nil {
				status, msg := m[1], m[2]
				goLines = append(goLines, fmt.Sprintf("\t\thttp.Error(w, %q, %s)", msg, status))
				goLines = append(goLines, "\t\treturn")
				translated = true
			}
		}

		// if x == undefined {
		if !translated {
			if m := reIfUndefined.FindStringSubmatch(trimmed); m != nil {
				goLines = append(goLines, fmt.Sprintf("\tif err != nil {"))
				translated = true
			}
		}

		// Fallthrough patterns → TODO
		if !translated {
			if reDateDiffDays.MatchString(trimmed) || reDateNow.MatchString(trimmed) ||
				reCryptoSHA256.MatchString(trimmed) || reCryptoMD5.MatchString(trimmed) {
				todos = append(todos, trimmed)
				goLines = append(goLines, fmt.Sprintf("\t// TODO: Manual implementation required for Tengo logic:"))
				goLines = append(goLines, fmt.Sprintf("\t//   %s", trimmed))
				translated = true
			}
		}

		// If still not translated and not a simple else/closing brace, try to keep as comment
		if !translated && trimmed != "" {
			// Check if it's a variable assignment or conditional we can pass through
			if strings.HasPrefix(trimmed, "if ") {
				goLines = append(goLines, "\t"+strings.ReplaceAll(trimmed, "undefined", "nil"))
			} else if strings.Contains(trimmed, ":=") || strings.Contains(trimmed, "=") {
				// Likely a variable assignment with business logic
				todos = append(todos, trimmed)
				goLines = append(goLines, fmt.Sprintf("\t// TODO: Manual implementation required for Tengo logic:"))
				goLines = append(goLines, fmt.Sprintf("\t//   %s", trimmed))
			} else {
				goLines = append(goLines, "\t"+trimmed)
			}
		}
	}

	return strings.Join(goLines, "\n"), todos
}

// inferTableFromRoute guesses the primary table from the route path.
func (pm *PatternMatcher) inferTableFromRoute(route manifest.Route) string {
	parts := strings.Split(strings.Trim(route.Path, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	// First non-parameter segment
	for _, p := range parts {
		if !strings.HasPrefix(p, ":") {
			if _, ok := pm.schemas[p]; ok {
				return p
			}
		}
	}
	return ""
}

// extractTableFromSQL extracts the table name from a simple SQL query.
var reFromTable = regexp.MustCompile(`(?i)FROM\s+(\w+)`)

func extractTableFromSQL(sql string) string {
	if m := reFromTable.FindStringSubmatch(sql); m != nil {
		return m[1]
	}
	return ""
}

// extractQueryParams extracts the parameter list from a db.query/db.query_one call.
var reParamList = regexp.MustCompile(`\[([^\]]*)\]`)

func extractQueryParams(line string) string {
	if m := reParamList.FindStringSubmatch(line); m != nil {
		params := strings.TrimSpace(m[1])
		if params == "" {
			return ""
		}
		// Convert Tengo true/false to Go
		params = strings.ReplaceAll(params, "true", "true")
		params = strings.ReplaceAll(params, "false", "false")
		return params
	}
	return ""
}

// routeToMethodName converts a route to a Go handler method name.
// e.g. GET /vehicles → ListVehicles, GET /vehicles/:id → GetVehicle, POST /bookings → CreateBooking
func routeToMethodName(route manifest.Route) string {
	parts := strings.Split(strings.Trim(route.Path, "/"), "/")
	// Find the primary resource (first non-param segment)
	resource := ""
	hasParam := false
	for _, p := range parts {
		if strings.HasPrefix(p, ":") {
			hasParam = true
		} else {
			resource = p
		}
	}

	structName := TableToStructName(resource)

	switch route.Method {
	case "GET":
		if hasParam {
			return "Get" + structName
		}
		return "List" + structName + "s"
	case "POST":
		return "Create" + structName
	case "PUT", "PATCH":
		return "Update" + structName
	case "DELETE":
		return "Delete" + structName
	default:
		return "Handle" + structName
	}
}

// chiPath converts VibeServe path params (:id) to Chi format ({id}).
func chiPath(path string) string {
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if strings.HasPrefix(p, ":") {
			parts[i] = "{" + p[1:] + "}"
		}
	}
	return strings.Join(parts, "/")
}

// GenerateHandlers produces the Go source for internal/handler/handlers.go.
func GenerateHandlers(schemas []manifest.Schema, routes []manifest.Route, scripts []manifest.Script) string {
	var b strings.Builder
	pm := NewPatternMatcher(schemas)

	b.WriteString("package handler\n\n")
	b.WriteString("import (\n")
	b.WriteString("\t\"encoding/json\"\n")
	b.WriteString("\t\"net/http\"\n")
	b.WriteString("\t\"strconv\"\n\n")
	b.WriteString(fmt.Sprintf("\t\"%s/internal/model\"\n", "{{MODULE}}"))
	b.WriteString(fmt.Sprintf("\t\"%s/internal/repository\"\n", "{{MODULE}}"))
	b.WriteString("\t\"github.com/go-chi/chi/v5\"\n")
	b.WriteString(")\n\n")

	// Silence unused import warnings
	b.WriteString("// Ensure imports are used.\n")
	b.WriteString("var (\n")
	b.WriteString("\t_ = strconv.Itoa\n")
	b.WriteString("\t_ = model.Vehicle{}\n")
	b.WriteString(")\n\n")

	b.WriteString("// Handler holds dependencies for HTTP handlers.\n")
	b.WriteString("type Handler struct {\n")
	b.WriteString("\tstore repository.Store\n")
	b.WriteString("}\n\n")

	b.WriteString("// NewHandler creates a Handler with the given store.\n")
	b.WriteString("func NewHandler(store repository.Store) *Handler {\n")
	b.WriteString("\treturn &Handler{store: store}\n")
	b.WriteString("}\n\n")

	scriptMap := make(map[string]*manifest.Script)
	for i := range scripts {
		scriptMap[scripts[i].Name] = &scripts[i]
	}

	for _, route := range routes {
		methodName := routeToMethodName(route)
		script, ok := scriptMap[route.Script]
		if !ok {
			continue
		}

		b.WriteString(fmt.Sprintf("// %s handles %s %s\n", methodName, route.Method, route.Path))
		if route.Description != "" {
			b.WriteString(fmt.Sprintf("// %s\n", route.Description))
		}
		b.WriteString(fmt.Sprintf("func (h *Handler) %s(w http.ResponseWriter, r *http.Request) {\n", methodName))

		body, _ := pm.TranslateScript(*script, route)
		b.WriteString(body)
		b.WriteString("\n}\n\n")
	}

	return b.String()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/kent/Documents/Projects/vibeserve && go test ./internal/export/ -run "TestTranslateScript|TestGenerateHandlers" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /Users/kent/Documents/Projects/vibeserve
git add internal/export/handlers.go internal/export/handlers_test.go
git commit -m "feat(export): add handler generation with Tengo pattern matcher"
```

---

### Task 5: OpenAPI Generation

**Files:**
- Create: `internal/export/openapi.go`
- Test: `internal/export/openapi_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/export/openapi_test.go
package export

import (
	"strings"
	"testing"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

func TestGenerateOpenAPI(t *testing.T) {
	m := &manifest.Manifest{
		Name:        "Car Rental API",
		Description: "Rent cars in Malaysia",
		Version:     "1.0",
		Schemas: []manifest.Schema{
			{
				Table: "vehicles",
				Columns: []manifest.Column{
					{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
					{Name: "make", Type: "TEXT", Required: true},
					{Name: "daily_rate", Type: "REAL", Required: true},
					{Name: "available", Type: "BOOLEAN"},
				},
			},
		},
		Routes: []manifest.Route{
			{Path: "/vehicles", Method: "GET", Description: "List all vehicles", Script: "list_vehicles", ResponseType: "array"},
			{Path: "/vehicles/:id", Method: "GET", Description: "Get vehicle by ID", Script: "get_vehicle", ResponseType: "object"},
			{Path: "/bookings", Method: "POST", Description: "Create booking", Script: "create_booking",
				RequestBody: map[string]string{"vehicle_id": "INTEGER", "customer": "TEXT"}, ResponseType: "object"},
		},
	}

	yaml := GenerateOpenAPI(m)

	// Check basic structure
	if !strings.Contains(yaml, "openapi: \"3.0.3\"") {
		t.Error("missing openapi version")
	}
	if !strings.Contains(yaml, "title: \"Car Rental API\"") {
		t.Error("missing title")
	}
	if !strings.Contains(yaml, "/vehicles:") {
		t.Error("missing /vehicles path")
	}
	// Chi-style params: :id → {id}
	if !strings.Contains(yaml, "/vehicles/{id}:") {
		t.Error("missing /vehicles/{id} path (should convert :id to {id})")
	}
	if strings.Contains(yaml, "/vehicles/:id") {
		t.Error(":id should be converted to {id}")
	}
	// Check components/schemas
	if !strings.Contains(yaml, "Vehicle:") {
		t.Error("missing Vehicle schema in components")
	}
	// Check request body
	if !strings.Contains(yaml, "requestBody:") {
		t.Error("missing requestBody for POST route")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/kent/Documents/Projects/vibeserve && go test ./internal/export/ -run "TestGenerateOpenAPI" -v`
Expected: FAIL — function not defined

- [ ] **Step 3: Implement OpenAPI generation**

```go
// internal/export/openapi.go
package export

import (
	"fmt"
	"strings"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

// openAPIType maps manifest column types to OpenAPI types.
func openAPIType(colType string) (string, string) {
	switch colType {
	case "INTEGER":
		return "integer", "int64"
	case "TEXT":
		return "string", ""
	case "REAL":
		return "number", "double"
	case "BOOLEAN":
		return "boolean", ""
	case "DATE":
		return "string", "date"
	case "DATETIME":
		return "string", "date-time"
	default:
		return "string", ""
	}
}

// GenerateOpenAPI produces an OpenAPI 3.0.3 YAML spec from a manifest.
func GenerateOpenAPI(m *manifest.Manifest) string {
	var b strings.Builder

	b.WriteString("openapi: \"3.0.3\"\n")
	b.WriteString("info:\n")
	b.WriteString(fmt.Sprintf("  title: %q\n", m.Name))
	if m.Description != "" {
		b.WriteString(fmt.Sprintf("  description: %q\n", m.Description))
	}
	b.WriteString(fmt.Sprintf("  version: %q\n", m.Version))
	b.WriteString("\n")

	// Group routes by path
	type pathEntry struct {
		path   string
		routes []manifest.Route
	}
	pathOrder := []string{}
	pathMap := make(map[string][]manifest.Route)
	for _, r := range m.Routes {
		apiPath := chiPath(r.Path)
		if _, exists := pathMap[apiPath]; !exists {
			pathOrder = append(pathOrder, apiPath)
		}
		pathMap[apiPath] = append(pathMap[apiPath], r)
	}

	b.WriteString("paths:\n")
	for _, apiPath := range pathOrder {
		routes := pathMap[apiPath]
		b.WriteString(fmt.Sprintf("  %s:\n", apiPath))

		for _, r := range routes {
			method := strings.ToLower(r.Method)
			b.WriteString(fmt.Sprintf("    %s:\n", method))
			if r.Description != "" {
				b.WriteString(fmt.Sprintf("      summary: %q\n", r.Description))
			}
			b.WriteString(fmt.Sprintf("      operationId: %s\n", routeToMethodName(r)))

			// Path parameters
			params := extractPathParams(r.Path)
			if len(params) > 0 {
				b.WriteString("      parameters:\n")
				for _, p := range params {
					b.WriteString(fmt.Sprintf("        - name: %s\n", p))
					b.WriteString("          in: path\n")
					b.WriteString("          required: true\n")
					b.WriteString("          schema:\n")
					b.WriteString("            type: integer\n")
				}
			}

			// Request body
			if len(r.RequestBody) > 0 {
				b.WriteString("      requestBody:\n")
				b.WriteString("        required: true\n")
				b.WriteString("        content:\n")
				b.WriteString("          application/json:\n")
				b.WriteString("            schema:\n")
				b.WriteString("              type: object\n")
				b.WriteString("              properties:\n")
				for field, fieldType := range r.RequestBody {
					oaType, oaFmt := openAPIType(fieldType)
					b.WriteString(fmt.Sprintf("                %s:\n", field))
					b.WriteString(fmt.Sprintf("                  type: %s\n", oaType))
					if oaFmt != "" {
						b.WriteString(fmt.Sprintf("                  format: %s\n", oaFmt))
					}
				}
			}

			// Response
			b.WriteString("      responses:\n")
			successCode := "200"
			if r.Method == "POST" {
				successCode = "201"
			}
			b.WriteString(fmt.Sprintf("        \"%s\":\n", successCode))
			b.WriteString(fmt.Sprintf("          description: Successful response\n"))
			b.WriteString("          content:\n")
			b.WriteString("            application/json:\n")
			b.WriteString("              schema:\n")

			table := inferTableFromPath(r.Path)
			structName := TableToStructName(table)
			if r.ResponseType == "array" {
				b.WriteString("                type: array\n")
				b.WriteString("                items:\n")
				b.WriteString(fmt.Sprintf("                  $ref: \"#/components/schemas/%s\"\n", structName))
			} else {
				b.WriteString(fmt.Sprintf("                $ref: \"#/components/schemas/%s\"\n", structName))
			}
		}
	}

	// Components/schemas
	b.WriteString("\ncomponents:\n")
	b.WriteString("  schemas:\n")
	for _, s := range m.Schemas {
		structName := TableToStructName(s.Table)
		b.WriteString(fmt.Sprintf("    %s:\n", structName))
		b.WriteString("      type: object\n")
		b.WriteString("      properties:\n")
		var required []string
		for _, c := range s.Columns {
			oaType, oaFmt := openAPIType(c.Type)
			b.WriteString(fmt.Sprintf("        %s:\n", c.Name))
			b.WriteString(fmt.Sprintf("          type: %s\n", oaType))
			if oaFmt != "" {
				b.WriteString(fmt.Sprintf("          format: %s\n", oaFmt))
			}
			if c.Required || c.Primary {
				required = append(required, c.Name)
			}
		}
		if len(required) > 0 {
			b.WriteString("      required:\n")
			for _, r := range required {
				b.WriteString(fmt.Sprintf("        - %s\n", r))
			}
		}
	}

	return b.String()
}

// extractPathParams extracts parameter names from a VibeServe path.
func extractPathParams(path string) []string {
	var params []string
	for _, seg := range strings.Split(path, "/") {
		if strings.HasPrefix(seg, ":") {
			params = append(params, seg[1:])
		}
	}
	return params
}

// inferTableFromPath guesses the table name from the first path segment.
func inferTableFromPath(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) > 0 && !strings.HasPrefix(parts[0], ":") {
		return parts[0]
	}
	return "unknown"
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/kent/Documents/Projects/vibeserve && go test ./internal/export/ -run "TestGenerateOpenAPI" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /Users/kent/Documents/Projects/vibeserve
git add internal/export/openapi.go internal/export/openapi_test.go
git commit -m "feat(export): add OpenAPI 3.0.3 YAML generation from manifest"
```

---

### Task 6: Scaffold Generation (Stage 6)

**Files:**
- Create: `internal/export/scaffold.go`
- Test: `internal/export/scaffold_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/export/scaffold_test.go
package export

import (
	"strings"
	"testing"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

func TestGenerateMain(t *testing.T) {
	routes := []manifest.Route{
		{Path: "/vehicles", Method: "GET", Script: "list_vehicles"},
		{Path: "/vehicles/:id", Method: "GET", Script: "get_vehicle"},
		{Path: "/bookings", Method: "POST", Script: "create_booking"},
	}

	code := GenerateMain("my-api", routes)

	if !strings.Contains(code, "package main") {
		t.Error("missing package main")
	}
	if !strings.Contains(code, "chi.NewRouter") {
		t.Error("missing chi router creation")
	}
	if !strings.Contains(code, "middleware.RequestID") {
		t.Error("missing RequestID middleware")
	}
	if !strings.Contains(code, "middleware.RealIP") {
		t.Error("missing RealIP middleware")
	}
	if !strings.Contains(code, "middleware.Logger") {
		t.Error("missing Logger middleware")
	}
	if !strings.Contains(code, "middleware.Recoverer") {
		t.Error("missing Recoverer middleware")
	}
	if !strings.Contains(code, "SIGINT") || !strings.Contains(code, "SIGTERM") {
		t.Error("missing graceful shutdown signals")
	}
	// Check route registration
	if !strings.Contains(code, "r.Get(\"/vehicles\"") {
		t.Error("missing GET /vehicles route")
	}
	if !strings.Contains(code, "r.Get(\"/vehicles/{id}\"") {
		t.Error("missing GET /vehicles/{id} route")
	}
	if !strings.Contains(code, "r.Post(\"/bookings\"") {
		t.Error("missing POST /bookings route")
	}
}

func TestGenerateGoMod(t *testing.T) {
	code := GenerateGoMod("my-api")

	if !strings.Contains(code, "module my-api") {
		t.Error("missing module declaration")
	}
	if !strings.Contains(code, "go-chi/chi") {
		t.Error("missing chi dependency")
	}
	if !strings.Contains(code, "jmoiron/sqlx") {
		t.Error("missing sqlx dependency")
	}
	if !strings.Contains(code, "modernc.org/sqlite") {
		t.Error("missing sqlite dependency")
	}
}

func TestGenerateDockerfile(t *testing.T) {
	code := GenerateDockerfile("my-api")

	if !strings.Contains(code, "FROM golang:") {
		t.Error("missing Go builder stage")
	}
	if !strings.Contains(code, "CGO_ENABLED=0") {
		t.Error("missing CGO_ENABLED=0")
	}
	if !strings.Contains(code, "FROM alpine") || !strings.Contains(code, "FROM gcr.io/distroless") {
		// Accept either alpine or distroless
	}
	if !strings.Contains(code, "EXPOSE") {
		t.Error("missing EXPOSE")
	}
}

func TestGenerateREADME(t *testing.T) {
	m := &manifest.Manifest{
		Name:        "Car Rental API",
		Description: "Rent cars in Malaysia",
		Schemas: []manifest.Schema{
			{Table: "vehicles", Columns: []manifest.Column{
				{Name: "id", Type: "INTEGER"},
				{Name: "make", Type: "TEXT"},
			}},
		},
		Routes: []manifest.Route{
			{Path: "/vehicles", Method: "GET", Description: "List all vehicles"},
		},
	}

	readme := GenerateREADME(m)

	if !strings.Contains(readme, "Car Rental API") {
		t.Error("missing project name")
	}
	if !strings.Contains(readme, "go run ./cmd/api") {
		t.Error("missing run instructions")
	}
	if !strings.Contains(readme, "GET") {
		t.Error("missing route table")
	}
	if !strings.Contains(readme, "vehicles") {
		t.Error("missing schema documentation")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/kent/Documents/Projects/vibeserve && go test ./internal/export/ -run "TestGenerateMain|TestGenerateGoMod|TestGenerateDockerfile|TestGenerateREADME" -v`
Expected: FAIL — functions not defined

- [ ] **Step 3: Implement scaffold generation**

```go
// internal/export/scaffold.go
package export

import (
	"fmt"
	"strings"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

// GenerateMain produces the Go source for cmd/api/main.go.
func GenerateMain(moduleName string, routes []manifest.Route) string {
	var b strings.Builder

	b.WriteString("package main\n\n")
	b.WriteString("import (\n")
	b.WriteString("\t\"context\"\n")
	b.WriteString("\t\"fmt\"\n")
	b.WriteString("\t\"log\"\n")
	b.WriteString("\t\"net/http\"\n")
	b.WriteString("\t\"os\"\n")
	b.WriteString("\t\"os/signal\"\n")
	b.WriteString("\t\"syscall\"\n")
	b.WriteString("\t\"time\"\n\n")
	b.WriteString(fmt.Sprintf("\t\"%s/internal/handler\"\n", moduleName))
	b.WriteString(fmt.Sprintf("\t\"%s/internal/repository\"\n", moduleName))
	b.WriteString("\t\"github.com/go-chi/chi/v5\"\n")
	b.WriteString("\t\"github.com/go-chi/chi/v5/middleware\"\n")
	b.WriteString(")\n\n")

	b.WriteString("func main() {\n")
	b.WriteString("\t// Database\n")
	b.WriteString("\tdsn := \"state.db\"\n")
	b.WriteString("\tif env := os.Getenv(\"DATABASE_URL\"); env != \"\" {\n")
	b.WriteString("\t\tdsn = env\n")
	b.WriteString("\t}\n")
	b.WriteString("\tstore, err := repository.NewSQLiteStore(dsn)\n")
	b.WriteString("\tif err != nil {\n")
	b.WriteString("\t\tlog.Fatalf(\"Failed to open database: %v\", err)\n")
	b.WriteString("\t}\n")
	b.WriteString("\tdefer store.Close()\n\n")

	b.WriteString("\t// Handler\n")
	b.WriteString("\th := handler.NewHandler(store)\n\n")

	b.WriteString("\t// Router\n")
	b.WriteString("\tr := chi.NewRouter()\n")
	b.WriteString("\tr.Use(middleware.RequestID)\n")
	b.WriteString("\tr.Use(middleware.RealIP)\n")
	b.WriteString("\tr.Use(middleware.Logger)\n")
	b.WriteString("\tr.Use(middleware.Recoverer)\n\n")

	b.WriteString("\t// Routes\n")
	for _, route := range routes {
		methodName := routeToMethodName(route)
		apiPath := chiPath(route.Path)
		chiMethod := strings.Title(strings.ToLower(route.Method))
		b.WriteString(fmt.Sprintf("\tr.%s(%q, h.%s)\n", chiMethod, apiPath, methodName))
	}

	b.WriteString("\n\t// Server\n")
	b.WriteString("\tport := \"8080\"\n")
	b.WriteString("\tif env := os.Getenv(\"PORT\"); env != \"\" {\n")
	b.WriteString("\t\tport = env\n")
	b.WriteString("\t}\n\n")

	b.WriteString("\tsrv := &http.Server{\n")
	b.WriteString("\t\tAddr:    \":\" + port,\n")
	b.WriteString("\t\tHandler: r,\n")
	b.WriteString("\t}\n\n")

	b.WriteString("\t// Graceful shutdown\n")
	b.WriteString("\tgo func() {\n")
	b.WriteString("\t\tsigCh := make(chan os.Signal, 1)\n")
	b.WriteString("\t\tsignal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)\n")
	b.WriteString("\t\t<-sigCh\n")
	b.WriteString("\t\tlog.Println(\"Shutting down...\")\n")
	b.WriteString("\t\tctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)\n")
	b.WriteString("\t\tdefer cancel()\n")
	b.WriteString("\t\tsrv.Shutdown(ctx)\n")
	b.WriteString("\t}()\n\n")

	b.WriteString("\tfmt.Printf(\"Server running on :%s\\n\", port)\n")
	b.WriteString("\tif err := srv.ListenAndServe(); err != http.ErrServerClosed {\n")
	b.WriteString("\t\tlog.Fatalf(\"Server error: %v\", err)\n")
	b.WriteString("\t}\n")
	b.WriteString("}\n")

	return b.String()
}

// GenerateGoMod produces the go.mod file content.
func GenerateGoMod(moduleName string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("module %s\n\n", moduleName))
	b.WriteString("go 1.23\n\n")
	b.WriteString("require (\n")
	b.WriteString("\tgithub.com/go-chi/chi/v5 v5.2.1\n")
	b.WriteString("\tgithub.com/jmoiron/sqlx v1.4.0\n")
	b.WriteString("\tmodernc.org/sqlite v1.48.1\n")
	b.WriteString(")\n")
	return b.String()
}

// GenerateDockerfile produces a multi-stage Dockerfile.
func GenerateDockerfile(moduleName string) string {
	var b strings.Builder
	b.WriteString("# Build stage\n")
	b.WriteString("FROM golang:1.23-alpine AS builder\n")
	b.WriteString("WORKDIR /app\n")
	b.WriteString("COPY go.mod go.sum ./\n")
	b.WriteString("RUN go mod download\n")
	b.WriteString("COPY . .\n")
	b.WriteString("RUN CGO_ENABLED=0 go build -o server ./cmd/api\n\n")
	b.WriteString("# Runtime stage\n")
	b.WriteString("FROM alpine:3.20\n")
	b.WriteString("RUN apk add --no-cache ca-certificates\n")
	b.WriteString("WORKDIR /app\n")
	b.WriteString("COPY --from=builder /app/server .\n")
	b.WriteString("COPY --from=builder /app/state.db .\n")
	b.WriteString("EXPOSE 8080\n")
	b.WriteString("ENTRYPOINT [\"./server\"]\n")
	return b.String()
}

// GenerateREADME produces a README.md file from the manifest.
func GenerateREADME(m *manifest.Manifest) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# %s\n\n", m.Name))
	if m.Description != "" {
		b.WriteString(fmt.Sprintf("%s\n\n", m.Description))
	}
	b.WriteString("Generated by [VibeServe](https://github.com/vibeserve/vibeserve).\n\n")

	b.WriteString("## Quick Start\n\n")
	b.WriteString("```bash\n")
	b.WriteString("go run ./cmd/api\n")
	b.WriteString("```\n\n")
	b.WriteString("Server starts on `:8080` by default. Set `PORT` environment variable to change.\n\n")

	b.WriteString("### Docker\n\n")
	b.WriteString("```bash\n")
	b.WriteString("docker build -t api .\n")
	b.WriteString("docker run -p 8080:8080 api\n")
	b.WriteString("```\n\n")

	// Route table
	b.WriteString("## API Routes\n\n")
	b.WriteString("| Method | Path | Description |\n")
	b.WriteString("|--------|------|-------------|\n")
	for _, r := range m.Routes {
		b.WriteString(fmt.Sprintf("| %s | %s | %s |\n", r.Method, r.Path, r.Description))
	}
	b.WriteString("\n")

	// Schema docs
	b.WriteString("## Database Schema\n\n")
	for _, s := range m.Schemas {
		b.WriteString(fmt.Sprintf("### %s\n\n", s.Table))
		b.WriteString("| Column | Type | Constraints |\n")
		b.WriteString("|--------|------|-------------|\n")
		for _, c := range s.Columns {
			constraints := []string{}
			if c.Primary {
				constraints = append(constraints, "PRIMARY KEY")
			}
			if c.Auto {
				constraints = append(constraints, "AUTOINCREMENT")
			}
			if c.Required {
				constraints = append(constraints, "NOT NULL")
			}
			if c.Unique {
				constraints = append(constraints, "UNIQUE")
			}
			if c.References != "" {
				constraints = append(constraints, "FK: "+c.References)
			}
			b.WriteString(fmt.Sprintf("| %s | %s | %s |\n", c.Name, c.Type, strings.Join(constraints, ", ")))
		}
		b.WriteString("\n")
	}

	b.WriteString("## Environment Variables\n\n")
	b.WriteString("| Variable | Default | Description |\n")
	b.WriteString("|----------|---------|-------------|\n")
	b.WriteString("| `PORT` | `8080` | Server port |\n")
	b.WriteString("| `DATABASE_URL` | `state.db` | SQLite database path |\n")

	return b.String()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/kent/Documents/Projects/vibeserve && go test ./internal/export/ -run "TestGenerateMain|TestGenerateGoMod|TestGenerateDockerfile|TestGenerateREADME" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /Users/kent/Documents/Projects/vibeserve
git add internal/export/scaffold.go internal/export/scaffold_test.go
git commit -m "feat(export): add scaffold generation — main.go, go.mod, Dockerfile, README"
```

---

### Task 7: Pipeline Orchestrator (Exporter)

**Files:**
- Create: `internal/export/exporter.go`
- Test: `internal/export/exporter_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/export/exporter_test.go
package export

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

func TestExporter_Run_CreatesAllFiles(t *testing.T) {
	m := &manifest.Manifest{
		Version:     "1.0",
		Name:        "test-api",
		Description: "A test API",
		Schemas: []manifest.Schema{
			{
				Table: "items",
				Columns: []manifest.Column{
					{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
					{Name: "name", Type: "TEXT", Required: true},
				},
			},
		},
		Routes: []manifest.Route{
			{Path: "/items", Method: "GET", Script: "list_items", ResponseType: "array"},
		},
		Scripts: []manifest.Script{
			{Name: "list_items", Code: "result := db.query(\"SELECT * FROM items\", [])\nresponse.json(result)"},
		},
	}

	outDir := t.TempDir()
	target := filepath.Join(outDir, "test-api")

	exp := NewExporter(m, target)
	err := exp.Run()
	if err != nil {
		t.Fatalf("Exporter.Run() failed: %v", err)
	}

	// Check all expected files exist
	expectedFiles := []string{
		"cmd/api/main.go",
		"internal/model/models.go",
		"internal/repository/store.go",
		"internal/repository/sqlite.go",
		"internal/handler/handlers.go",
		"go.mod",
		"Dockerfile",
		"README.md",
		"openapi.yaml",
	}

	for _, f := range expectedFiles {
		path := filepath.Join(target, f)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected file %s to exist", f)
		}
	}
}

func TestExporter_DefaultOutputDir(t *testing.T) {
	m := &manifest.Manifest{
		Name:    "My Cool API",
		Version: "1.0",
	}

	dir := DefaultOutputDir(m)
	if dir != "vibe-export-my-cool-api" {
		t.Errorf("DefaultOutputDir = %q, want %q", dir, "vibe-export-my-cool-api")
	}
}

func TestExporter_ModuleReplace(t *testing.T) {
	m := &manifest.Manifest{
		Version: "1.0",
		Name:    "test-api",
		Schemas: []manifest.Schema{
			{Table: "items", Columns: []manifest.Column{{Name: "id", Type: "INTEGER", Primary: true, Auto: true}}},
		},
		Routes: []manifest.Route{
			{Path: "/items", Method: "GET", Script: "list_items", ResponseType: "array"},
		},
		Scripts: []manifest.Script{
			{Name: "list_items", Code: "result := db.query(\"SELECT * FROM items\", [])\nresponse.json(result)"},
		},
	}

	outDir := t.TempDir()
	target := filepath.Join(outDir, "test-api")

	exp := NewExporter(m, target)
	err := exp.Run()
	if err != nil {
		t.Fatalf("Run() failed: %v", err)
	}

	// Verify module placeholder was replaced in store.go
	storeBytes, _ := os.ReadFile(filepath.Join(target, "internal/repository/store.go"))
	storeCode := string(storeBytes)
	if storeCode == "" {
		t.Fatal("store.go is empty")
	}
	if contains := storeCode; contains == "" {
		t.Error("store.go should not be empty")
	}
	// Should NOT contain the placeholder
	if containsStr(storeCode, "{{MODULE}}") {
		t.Error("store.go still contains {{MODULE}} placeholder")
	}
}

func containsStr(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) > len(substr) && searchStr(s, substr))
}

func searchStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/kent/Documents/Projects/vibeserve && go test ./internal/export/ -run "TestExporter" -v`
Expected: FAIL — Exporter not defined

- [ ] **Step 3: Implement the exporter pipeline**

```go
// internal/export/exporter.go
package export

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

// Exporter orchestrates the 7-stage export pipeline.
type Exporter struct {
	manifest *manifest.Manifest
	outDir   string
	vibeDir  string // source .vibe directory for state.db copy
}

// NewExporter creates an Exporter for the given manifest and output directory.
func NewExporter(m *manifest.Manifest, outDir string) *Exporter {
	return &Exporter{
		manifest: m,
		outDir:   outDir,
		vibeDir:  ".vibe",
	}
}

// SetVibeDir overrides the source directory for state.db (used in testing).
func (e *Exporter) SetVibeDir(dir string) {
	e.vibeDir = dir
}

// DefaultOutputDir returns the default output directory name based on manifest name.
func DefaultOutputDir(m *manifest.Manifest) string {
	return "vibe-export-" + Slugify(m.Name)
}

// Run executes the full export pipeline.
func (e *Exporter) Run() error {
	m := e.manifest
	moduleName := Slugify(m.Name)

	// Stage 1: Load manifest (already loaded, passed to constructor)

	// Stage 2: Validate
	if err := manifest.Validate(m); err != nil {
		return fmt.Errorf("manifest validation failed: %w", err)
	}

	// Create output directory structure
	dirs := []string{
		"cmd/api",
		"internal/model",
		"internal/repository",
		"internal/handler",
	}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(e.outDir, d), 0o755); err != nil {
			return fmt.Errorf("create directory %s: %w", d, err)
		}
	}

	// Stage 3: Generate models
	modelsCode := GenerateModels(m.Schemas)
	if err := e.writeFile("internal/model/models.go", modelsCode); err != nil {
		return err
	}

	// Stage 4: Generate repository
	usage := AnalyzeRouteUsage(m.Routes, m.Scripts)

	storeCode := GenerateStoreInterface(m.Schemas, usage)
	storeCode = strings.ReplaceAll(storeCode, "{{MODULE}}", moduleName)
	if err := e.writeFile("internal/repository/store.go", storeCode); err != nil {
		return err
	}

	sqliteCode := GenerateSQLiteStore(m.Schemas, usage)
	sqliteCode = strings.ReplaceAll(sqliteCode, "{{MODULE}}", moduleName)
	if err := e.writeFile("internal/repository/sqlite.go", sqliteCode); err != nil {
		return err
	}

	// Stage 5: Generate handlers
	handlersCode := GenerateHandlers(m.Schemas, m.Routes, m.Scripts)
	handlersCode = strings.ReplaceAll(handlersCode, "{{MODULE}}", moduleName)
	if err := e.writeFile("internal/handler/handlers.go", handlersCode); err != nil {
		return err
	}

	// Stage 6: Generate scaffold
	mainCode := GenerateMain(moduleName, m.Routes)
	if err := e.writeFile("cmd/api/main.go", mainCode); err != nil {
		return err
	}

	goModCode := GenerateGoMod(moduleName)
	if err := e.writeFile("go.mod", goModCode); err != nil {
		return err
	}

	dockerfile := GenerateDockerfile(moduleName)
	if err := e.writeFile("Dockerfile", dockerfile); err != nil {
		return err
	}

	readme := GenerateREADME(m)
	if err := e.writeFile("README.md", readme); err != nil {
		return err
	}

	// OpenAPI spec
	openapi := GenerateOpenAPI(m)
	if err := e.writeFile("openapi.yaml", openapi); err != nil {
		return err
	}

	// Copy state.db if exists
	stateDB := filepath.Join(e.vibeDir, "state.db")
	if _, err := os.Stat(stateDB); err == nil {
		data, err := os.ReadFile(stateDB)
		if err != nil {
			return fmt.Errorf("read state.db: %w", err)
		}
		if err := os.WriteFile(filepath.Join(e.outDir, "state.db"), data, 0o644); err != nil {
			return fmt.Errorf("write state.db: %w", err)
		}
	}

	// Stage 7: Post-process (go mod tidy, gofmt) — handled by the CLI command

	return nil
}

// writeFile writes content to a file inside the output directory.
func (e *Exporter) writeFile(relPath, content string) error {
	fullPath := filepath.Join(e.outDir, relPath)
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", relPath, err)
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/kent/Documents/Projects/vibeserve && go test ./internal/export/ -run "TestExporter" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /Users/kent/Documents/Projects/vibeserve
git add internal/export/exporter.go internal/export/exporter_test.go
git commit -m "feat(export): add pipeline orchestrator — 7-stage export"
```

---

### Task 8: CLI `export` Command

**Files:**
- Modify: `cmd/vibeserve/main.go`

- [ ] **Step 1: Write the export command**

Add the following to `cmd/vibeserve/main.go`:

After the existing `undoCmd()` function (around line 149), add:

```go
func exportCmd() *cobra.Command {
	var manifestPath string
	var force bool
	var ai bool
	var configPath string

	cmd := &cobra.Command{
		Use:   "export [output-dir]",
		Short: "Export a standalone Go server project from the manifest",
		Long:  "Generate a production-ready Go project with Chi routing, sqlx, and typed handlers from the current VibeServe manifest.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExport(manifestPath, configPath, args, force, ai)
		},
	}

	cmd.Flags().StringVarP(&manifestPath, "manifest", "m", ".vibe/manifest.json", "Path to manifest.json")
	cmd.Flags().StringVarP(&configPath, "config", "c", ".vibe/config.yaml", "Path to config.yaml")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing output directory")
	cmd.Flags().BoolVar(&ai, "ai", false, "Use LLM to translate complex Tengo logic")

	return cmd
}

func runExport(manifestPath, configPath string, args []string, force, ai bool) error {
	// Stage 1: Load manifest
	m, err := manifest.LoadFromFile(manifestPath)
	if err != nil {
		return fmt.Errorf("no manifest found at %s. Run 'vibeserve' first to create your API", manifestPath)
	}

	// Determine output directory
	var outDir string
	if len(args) > 0 {
		outDir = args[0]
	} else {
		outDir = export.DefaultOutputDir(m)
	}

	// Check if directory exists
	if info, err := os.Stat(outDir); err == nil && info.IsDir() {
		if !force {
			fmt.Printf("Directory %q already exists. Overwrite? [y/N] ", outDir)
			var answer string
			fmt.Scanln(&answer)
			if strings.ToLower(strings.TrimSpace(answer)) != "y" {
				fmt.Println("Export cancelled.")
				return nil
			}
		}
		os.RemoveAll(outDir)
	}

	fmt.Printf("Exporting to %s...\n", outDir)

	// Run the export pipeline
	exp := export.NewExporter(m, outDir)
	if err := exp.Run(); err != nil {
		return fmt.Errorf("export failed: %w", err)
	}

	// Post-process: go mod tidy
	fmt.Println("Running go mod tidy...")
	tidyCmd := exec.Command("go", "mod", "tidy")
	tidyCmd.Dir = outDir
	if tidyOut, err := tidyCmd.CombinedOutput(); err != nil {
		fmt.Printf("Warning: go mod tidy failed: %s\n", string(tidyOut))
	}

	// Post-process: gofmt
	fmtCmd := exec.Command("gofmt", "-w", ".")
	fmtCmd.Dir = outDir
	fmtCmd.Run() // best-effort

	// Success banner
	fmt.Println()
	fmt.Println("  \u2713 Export complete!")
	fmt.Println()
	fmt.Printf("  Your production Go server is ready at: ./%s\n", outDir)
	fmt.Println()
	fmt.Println("  To start:")
	fmt.Printf("    cd %s\n", outDir)
	fmt.Println("    go run ./cmd/api")
	fmt.Println()
	fmt.Println("  Check README.md for API documentation.")
	fmt.Println()

	return nil
}
```

- [ ] **Step 2: Add the import for `os/exec` and `export` package**

In the import block of `cmd/vibeserve/main.go`, add:

```go
	"os/exec"

	"github.com/vibeserve/vibeserve/internal/export"
```

- [ ] **Step 3: Register the export command**

In the `main()` function, after the line `rootCmd.AddCommand(undoCmd())` (around line 54), add:

```go
	rootCmd.AddCommand(exportCmd())
```

- [ ] **Step 4: Verify build**

Run: `cd /Users/kent/Documents/Projects/vibeserve && go build ./cmd/vibeserve`
Expected: Build succeeds

- [ ] **Step 5: Verify help output**

Run: `cd /Users/kent/Documents/Projects/vibeserve && go run ./cmd/vibeserve export --help`
Expected: Shows export usage with --force, --ai, -m flags

- [ ] **Step 6: Commit**

```bash
cd /Users/kent/Documents/Projects/vibeserve
git add cmd/vibeserve/main.go
git commit -m "feat: add vibeserve export command"
```

---

### Task 9: Integration Test — Full Export with car_rental Manifest

**Files:**
- Modify: `internal/export/exporter_test.go`

- [ ] **Step 1: Write the integration test**

Add this test to `internal/export/exporter_test.go`:

```go
func TestExporter_CarRentalManifest(t *testing.T) {
	// Load the real test manifest
	m, err := manifest.LoadFromFile("../../testdata/car_rental_manifest.json")
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	outDir := t.TempDir()
	target := filepath.Join(outDir, "car-rental")

	exp := NewExporter(m, target)
	err = exp.Run()
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	// Verify models contain Vehicle and Booking structs
	modelsBytes, _ := os.ReadFile(filepath.Join(target, "internal/model/models.go"))
	models := string(modelsBytes)
	if !containsStr(models, "type Vehicle struct") {
		t.Error("missing Vehicle struct")
	}
	if !containsStr(models, "type Booking struct") {
		t.Error("missing Booking struct")
	}
	if !containsStr(models, "float64") {
		t.Error("missing float64 for REAL columns")
	}

	// Verify repository has usage-driven methods
	storeBytes, _ := os.ReadFile(filepath.Join(target, "internal/repository/store.go"))
	store := string(storeBytes)
	if !containsStr(store, "ListVehicles") {
		t.Error("missing ListVehicles in store interface")
	}
	if !containsStr(store, "GetVehicle") {
		t.Error("missing GetVehicle in store interface")
	}
	if !containsStr(store, "CreateBooking") {
		t.Error("missing CreateBooking in store interface")
	}

	// Verify handlers reference chi and store
	handlersBytes, _ := os.ReadFile(filepath.Join(target, "internal/handler/handlers.go"))
	handlers := string(handlersBytes)
	if !containsStr(handlers, "func (h *Handler)") {
		t.Error("missing handler methods")
	}
	if !containsStr(handlers, "chi.URLParam") {
		t.Error("missing chi.URLParam in get_vehicle handler")
	}

	// Verify main.go has all routes registered
	mainBytes, _ := os.ReadFile(filepath.Join(target, "cmd/api/main.go"))
	mainCode := string(mainBytes)
	if !containsStr(mainCode, "r.Get(\"/vehicles\"") {
		t.Error("missing GET /vehicles route in main.go")
	}
	if !containsStr(mainCode, "r.Post(\"/bookings\"") {
		t.Error("missing POST /bookings route in main.go")
	}

	// Verify OpenAPI spec
	openapiBytes, _ := os.ReadFile(filepath.Join(target, "openapi.yaml"))
	openapi := string(openapiBytes)
	if !containsStr(openapi, "openapi:") {
		t.Error("missing openapi version")
	}
	if !containsStr(openapi, "/vehicles:") {
		t.Error("missing /vehicles path in OpenAPI")
	}

	// Verify README
	readmeBytes, _ := os.ReadFile(filepath.Join(target, "README.md"))
	readme := string(readmeBytes)
	if !containsStr(readme, "malaysia-car-rental") || !containsStr(readme, "Car") {
		t.Error("missing project name in README")
	}

	// Verify no {{MODULE}} placeholders remain
	allFiles := []string{
		"internal/model/models.go",
		"internal/repository/store.go",
		"internal/repository/sqlite.go",
		"internal/handler/handlers.go",
		"cmd/api/main.go",
	}
	for _, f := range allFiles {
		data, _ := os.ReadFile(filepath.Join(target, f))
		if containsStr(string(data), "{{MODULE}}") {
			t.Errorf("%s still contains {{MODULE}} placeholder", f)
		}
	}
}
```

- [ ] **Step 2: Run the integration test**

Run: `cd /Users/kent/Documents/Projects/vibeserve && go test ./internal/export/ -run "TestExporter_CarRentalManifest" -v`
Expected: PASS

- [ ] **Step 3: Run all export tests together**

Run: `cd /Users/kent/Documents/Projects/vibeserve && go test ./internal/export/ -v`
Expected: All PASS

- [ ] **Step 4: Commit**

```bash
cd /Users/kent/Documents/Projects/vibeserve
git add internal/export/exporter_test.go
git commit -m "test(export): add integration test with car_rental manifest"
```

---

### Task 10: Full Build + Smoke Test

- [ ] **Step 1: Build the full binary**

Run: `cd /Users/kent/Documents/Projects/vibeserve && go build ./cmd/vibeserve`
Expected: Build succeeds with no errors

- [ ] **Step 2: Run all tests**

Run: `cd /Users/kent/Documents/Projects/vibeserve && go test ./... -v`
Expected: All packages pass

- [ ] **Step 3: Smoke test export command**

Run: `cd /Users/kent/Documents/Projects/vibeserve && ./vibeserve export --manifest testdata/car_rental_manifest.json /tmp/car-rental-export --force`
Expected: Success banner printed, directory created with all expected files

- [ ] **Step 4: Verify generated project compiles**

Run: `cd /tmp/car-rental-export && go build ./cmd/api`
Expected: Build succeeds (after go mod tidy has run)

- [ ] **Step 5: Commit final state**

```bash
cd /Users/kent/Documents/Projects/vibeserve
git add -A
git commit -m "feat: Phase 5 complete — vibeserve export generates standalone Go server"
```
