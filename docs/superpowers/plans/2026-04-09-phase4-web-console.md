# Phase 4: The Web Console — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an embedded web console at `localhost:<port>/_console` — a browser-based UI for inspecting and testing the running VibeServe API. Five tabs: API Explorer (Postman-like request builder), Database Browser, Script Viewer, HTTP Trace (live WebSocket feed), and Manifest viewer. Everything is a single self-contained HTML file with inline CSS/JS, embedded via Go's `embed` package.

**Architecture:** A `http.ServeMux` sits in front of the existing trie handler. Console paths (`/_console/`, `/_api/`, `/_ws`) are handled by the new web package; everything else falls through to the existing `router.NewHandler()`. The console backend reads from `engine.Engine.Manifest()` and `store.Store.Query()`. Live HTTP traces are pushed over WebSocket via the event Bus.

**Tech Stack:** Go 1.26+, `nhooyr.io/websocket` (pure Go WebSocket), `embed` (stdlib), vanilla HTML + CSS + JavaScript (zero external dependencies in the frontend)

---

## File Map

```
vibeserve/
├── cmd/vibeserve/
│   └── main.go                          # MODIFIED — wrap handler with console mux
├── internal/
│   ├── web/
│   │   ├── console.go                   # NEW — REST API handlers for /_api/*
│   │   ├── console_test.go              # NEW — unit tests for REST endpoints
│   │   ├── websocket.go                 # NEW — WebSocket upgrade handler + Bus bridge
│   │   ├── embed.go                     # NEW — //go:embed static/* + fs setup
│   │   └── static/
│   │       └── index.html               # NEW — entire SPA (inline CSS + JS)
│   ├── engine/                          # EXISTING (unchanged)
│   ├── manifest/                        # EXISTING (unchanged)
│   ├── store/
│   │   └── store.go                     # MODIFIED — add Tables() method
│   ├── router/                          # EXISTING (unchanged)
│   └── ...
├── go.mod                               # MODIFIED — add nhooyr.io/websocket
└── go.sum                               # MODIFIED
```

---

### Task 1: Console Backend — Mux + REST API + Store.Tables()

**Files:**
- Modify: `internal/store/store.go` — add `Tables()` method
- Create: `internal/web/console.go`
- Create: `internal/web/console_test.go`

Add a `Tables()` method to the Store so the console can list all user tables. Create the REST API handlers that serve `/_api/*` endpoints. These handlers receive the Engine and Store at construction time.

- [ ] **Step 1: Add `Tables()` to Store**

Append to `internal/store/store.go`:

```go
// TableInfo describes a database table and its row count.
type TableInfo struct {
	Name     string           `json:"name"`
	Columns  []ColumnInfo     `json:"columns"`
	RowCount int              `json:"row_count"`
}

// ColumnInfo describes a single column in a table.
type ColumnInfo struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	NotNull bool   `json:"not_null"`
	PK      bool   `json:"pk"`
}

// Tables returns metadata for all user-created tables (excludes sqlite_ internals).
func (s *Store) Tables() ([]TableInfo, error) {
	rows, err := s.db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}
	defer rows.Close()

	var tables []TableInfo
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan table name: %w", err)
		}

		// Get column info via PRAGMA
		ti := TableInfo{Name: name}
		pragmaRows, err := s.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", name))
		if err != nil {
			return nil, fmt.Errorf("pragma table_info(%s): %w", name, err)
		}
		for pragmaRows.Next() {
			var cid int
			var colName, colType string
			var notNull, pk int
			var dfltValue sql.NullString
			if err := pragmaRows.Scan(&cid, &colName, &colType, &notNull, &dfltValue, &pk); err != nil {
				pragmaRows.Close()
				return nil, fmt.Errorf("scan column info: %w", err)
			}
			ti.Columns = append(ti.Columns, ColumnInfo{
				Name:    colName,
				Type:    colType,
				NotNull: notNull == 1,
				PK:      pk == 1,
			})
		}
		pragmaRows.Close()

		// Get row count
		count, err := s.Count(name)
		if err != nil {
			count = 0
		}
		ti.RowCount = count

		tables = append(tables, ti)
	}
	return tables, rows.Err()
}
```

- [ ] **Step 2: Define ConsoleStore interface in the web package**

The `web` package defines its own interface to avoid import cycles (engine cannot import store). The interface will be defined in `console.go` (Step 3) and satisfied by `*store.Store`.

- [ ] **Step 3: Create `internal/web/console.go`**

```bash
mkdir -p /Users/kent/Documents/Projects/vibeserve/internal/web/static
```

Create `internal/web/console.go`:

```go
package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/vibeserve/vibeserve/internal/engine"
	"github.com/vibeserve/vibeserve/internal/store"
)

// ConsoleStore provides the read operations needed by the web console.
type ConsoleStore interface {
	Tables() ([]store.TableInfo, error)
	Query(sql string, params []any) ([]map[string]any, error)
	Count(table string) (int, error)
}

// Console provides HTTP handlers for the /_api/* REST endpoints.
type Console struct {
	engine *engine.Engine
	store  ConsoleStore
}

// NewConsole creates a Console with access to the Engine and Store.
func NewConsole(eng *engine.Engine, s ConsoleStore) *Console {
	return &Console{engine: eng, store: s}
}

// RegisterRoutes registers all /_api/* handlers on the given mux.
func (c *Console) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /_api/manifest", c.handleManifest)
	mux.HandleFunc("GET /_api/routes", c.handleRoutes)
	mux.HandleFunc("GET /_api/tables", c.handleTables)
	mux.HandleFunc("GET /_api/tables/{name}/rows", c.handleTableRows)
	mux.HandleFunc("GET /_api/scripts", c.handleScripts)
	mux.HandleFunc("GET /_api/scripts/{name}", c.handleScript)
}

// handleManifest returns the full manifest JSON.
// GET /_api/manifest
func (c *Console) handleManifest(w http.ResponseWriter, r *http.Request) {
	m := c.engine.Manifest()
	if m == nil {
		writeConsoleJSON(w, http.StatusOK, map[string]any{
			"version": "1.0",
			"name":    "(no manifest loaded)",
			"schemas": []any{},
			"routes":  []any{},
			"scripts": []any{},
			"seeds":   []any{},
		})
		return
	}
	writeConsoleJSON(w, http.StatusOK, m)
}

// handleRoutes returns the route table.
// GET /_api/routes
func (c *Console) handleRoutes(w http.ResponseWriter, r *http.Request) {
	m := c.engine.Manifest()
	if m == nil {
		writeConsoleJSON(w, http.StatusOK, []any{})
		return
	}

	type routeInfo struct {
		Method      string `json:"method"`
		Path        string `json:"path"`
		Script      string `json:"script"`
		Description string `json:"description"`
	}

	routes := make([]routeInfo, len(m.Routes))
	for i, rt := range m.Routes {
		routes[i] = routeInfo{
			Method:      rt.Method,
			Path:        rt.Path,
			Script:      rt.Script,
			Description: rt.Description,
		}
	}
	writeConsoleJSON(w, http.StatusOK, routes)
}

// handleTables returns metadata for all database tables.
// GET /_api/tables
func (c *Console) handleTables(w http.ResponseWriter, r *http.Request) {
	tables, err := c.store.Tables()
	if err != nil {
		writeConsoleJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if tables == nil {
		tables = []store.TableInfo{}
	}
	writeConsoleJSON(w, http.StatusOK, tables)
}

// handleTableRows returns rows from a specific table with pagination.
// GET /_api/tables/{name}/rows?limit=50&offset=0
func (c *Console) handleTableRows(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeConsoleJSON(w, http.StatusBadRequest, map[string]string{"error": "table name required"})
		return
	}

	// Validate table name — alphanumeric + underscores only to prevent SQL injection.
	for _, ch := range name {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_') {
			writeConsoleJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid table name"})
			return
		}
	}

	limit := 50
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	query := "SELECT * FROM " + name + " LIMIT ? OFFSET ?"
	rows, err := c.store.Query(query, []any{limit, offset})
	if err != nil {
		writeConsoleJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	total, _ := c.store.Count(name)

	writeConsoleJSON(w, http.StatusOK, map[string]any{
		"rows":   rows,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// handleScripts returns all scripts.
// GET /_api/scripts
func (c *Console) handleScripts(w http.ResponseWriter, r *http.Request) {
	m := c.engine.Manifest()
	if m == nil {
		writeConsoleJSON(w, http.StatusOK, []any{})
		return
	}

	type scriptInfo struct {
		Name string `json:"name"`
		Code string `json:"code"`
	}

	scripts := make([]scriptInfo, len(m.Scripts))
	for i, s := range m.Scripts {
		scripts[i] = scriptInfo{Name: s.Name, Code: s.Code}
	}
	writeConsoleJSON(w, http.StatusOK, scripts)
}

// handleScript returns a single script by name.
// GET /_api/scripts/{name}
func (c *Console) handleScript(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeConsoleJSON(w, http.StatusBadRequest, map[string]string{"error": "script name required"})
		return
	}

	m := c.engine.Manifest()
	if m == nil {
		writeConsoleJSON(w, http.StatusNotFound, map[string]string{"error": "no manifest loaded"})
		return
	}

	// Strip .tengo suffix if present for matching convenience.
	lookupName := name
	for _, s := range m.Scripts {
		if s.Name == lookupName || s.Name == lookupName+".tengo" || strings.TrimSuffix(s.Name, ".tengo") == lookupName {
			writeConsoleJSON(w, http.StatusOK, map[string]any{"name": s.Name, "code": s.Code})
			return
		}
	}

	writeConsoleJSON(w, http.StatusNotFound, map[string]string{"error": "script not found: " + name})
}

// writeConsoleJSON serializes v as JSON and writes it to w.
func writeConsoleJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
```

- [ ] **Step 4: Create `internal/web/console_test.go`**

Create `internal/web/console_test.go`:

```go
package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vibeserve/vibeserve/internal/engine"
	"github.com/vibeserve/vibeserve/internal/manifest"
	"github.com/vibeserve/vibeserve/internal/store"
)

// testManifest returns a minimal manifest for tests.
func testManifest() *manifest.Manifest {
	return &manifest.Manifest{
		Version:     "1.0",
		Name:        "test-api",
		Description: "Test API",
		Schemas: []manifest.Schema{
			{
				Table: "users",
				Columns: []manifest.Column{
					{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
					{Name: "name", Type: "TEXT", Required: true},
					{Name: "email", Type: "TEXT", Required: true, Unique: true},
				},
			},
		},
		Routes: []manifest.Route{
			{Path: "/users", Method: "GET", Script: "list_users.tengo", Description: "List all users"},
			{Path: "/users", Method: "POST", Script: "create_user.tengo", Description: "Create a user"},
			{Path: "/users/:id", Method: "GET", Script: "get_user.tengo", Description: "Get user by ID"},
		},
		Scripts: []manifest.Script{
			{Name: "list_users.tengo", Code: "result := db.query(\"SELECT * FROM users\", [])\nresponse.json(result)"},
			{Name: "create_user.tengo", Code: "body := request.body()\nrow := db.insert(\"users\", body)\nresponse.json(row, 201)"},
			{Name: "get_user.tengo", Code: "id := request.param(\"id\")\nrow := db.query_one(\"SELECT * FROM users WHERE id = ?\", [id])\nresponse.json(row)"},
		},
		Seeds: []manifest.Seed{
			{Table: "users", Rows: []map[string]any{{"name": "Alice", "email": "alice@example.com"}}},
		},
	}
}

// setupTestConsole creates an in-memory store + engine for testing.
func setupTestConsole(t *testing.T) (*Console, *http.ServeMux) {
	t.Helper()

	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	m := testManifest()
	if err := s.ApplySchemas(m.Schemas); err != nil {
		t.Fatalf("apply schemas: %v", err)
	}
	if err := s.Seed("users", m.Seeds[0].Rows); err != nil {
		t.Fatalf("seed: %v", err)
	}

	bus := engine.NewBus()
	eng := engine.NewEngine(engine.EngineConfig{
		Bus:      bus,
		Store:    s,
		Manifest: m,
		VibeDir:  "",
	})

	console := NewConsole(eng, s)
	mux := http.NewServeMux()
	console.RegisterRoutes(mux)

	return console, mux
}

func TestHandleManifest(t *testing.T) {
	_, mux := setupTestConsole(t)

	req := httptest.NewRequest(http.MethodGet, "/_api/manifest", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var m manifest.Manifest
	if err := json.NewDecoder(w.Body).Decode(&m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if m.Name != "test-api" {
		t.Errorf("expected name=test-api, got %q", m.Name)
	}
	if len(m.Routes) != 3 {
		t.Errorf("expected 3 routes, got %d", len(m.Routes))
	}
}

func TestHandleRoutes(t *testing.T) {
	_, mux := setupTestConsole(t)

	req := httptest.NewRequest(http.MethodGet, "/_api/routes", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var routes []map[string]string
	if err := json.NewDecoder(w.Body).Decode(&routes); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(routes) != 3 {
		t.Errorf("expected 3 routes, got %d", len(routes))
	}
}

func TestHandleTables(t *testing.T) {
	_, mux := setupTestConsole(t)

	req := httptest.NewRequest(http.MethodGet, "/_api/tables", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var tables []store.TableInfo
	if err := json.NewDecoder(w.Body).Decode(&tables); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	if tables[0].Name != "users" {
		t.Errorf("expected table name=users, got %q", tables[0].Name)
	}
	if tables[0].RowCount != 1 {
		t.Errorf("expected 1 row, got %d", tables[0].RowCount)
	}
	if len(tables[0].Columns) != 3 {
		t.Errorf("expected 3 columns, got %d", len(tables[0].Columns))
	}
}

func TestHandleTableRows(t *testing.T) {
	_, mux := setupTestConsole(t)

	req := httptest.NewRequest(http.MethodGet, "/_api/tables/users/rows", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var result struct {
		Rows   []map[string]any `json:"rows"`
		Total  int              `json:"total"`
		Limit  int              `json:"limit"`
		Offset int              `json:"offset"`
	}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("expected total=1, got %d", result.Total)
	}
	if len(result.Rows) != 1 {
		t.Errorf("expected 1 row, got %d", len(result.Rows))
	}
}

func TestHandleTableRowsPagination(t *testing.T) {
	_, mux := setupTestConsole(t)

	req := httptest.NewRequest(http.MethodGet, "/_api/tables/users/rows?limit=10&offset=0", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var result struct {
		Limit  int `json:"limit"`
		Offset int `json:"offset"`
	}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Limit != 10 {
		t.Errorf("expected limit=10, got %d", result.Limit)
	}
	if result.Offset != 0 {
		t.Errorf("expected offset=0, got %d", result.Offset)
	}
}

func TestHandleTableRowsInvalidName(t *testing.T) {
	_, mux := setupTestConsole(t)

	req := httptest.NewRequest(http.MethodGet, "/_api/tables/users;DROP TABLE/rows", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleScripts(t *testing.T) {
	_, mux := setupTestConsole(t)

	req := httptest.NewRequest(http.MethodGet, "/_api/scripts", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var scripts []map[string]string
	if err := json.NewDecoder(w.Body).Decode(&scripts); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(scripts) != 3 {
		t.Errorf("expected 3 scripts, got %d", len(scripts))
	}
}

func TestHandleScriptByName(t *testing.T) {
	_, mux := setupTestConsole(t)

	req := httptest.NewRequest(http.MethodGet, "/_api/scripts/list_users.tengo", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var script map[string]string
	if err := json.NewDecoder(w.Body).Decode(&script); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if script["name"] != "list_users.tengo" {
		t.Errorf("expected name=list_users.tengo, got %q", script["name"])
	}
}

func TestHandleScriptNotFound(t *testing.T) {
	_, mux := setupTestConsole(t)

	req := httptest.NewRequest(http.MethodGet, "/_api/scripts/nonexistent", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
```

- [ ] **Step 5: Verify compilation and run tests**

```bash
cd /Users/kent/Documents/Projects/vibeserve
go vet ./internal/store/...
go vet ./internal/web/...
go test ./internal/web/... -v -count=1
go test ./internal/store/... -v -count=1 -run TestTables
```

---

### Task 2: WebSocket Event Bridge

**Files:**
- Modify: `go.mod` — add `nhooyr.io/websocket`
- Create: `internal/web/websocket.go`

Install the WebSocket library. Create a handler at `/_ws` that upgrades to WebSocket and subscribes to Bus events, pushing them as JSON to the connected browser.

- [ ] **Step 1: Install WebSocket dependency**

```bash
cd /Users/kent/Documents/Projects/vibeserve
go get nhooyr.io/websocket@latest
```

- [ ] **Step 2: Create `internal/web/websocket.go`**

Create `internal/web/websocket.go`:

```go
package web

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"nhooyr.io/websocket"

	"github.com/vibeserve/vibeserve/internal/engine"
)

// wsMessage is the JSON envelope sent to WebSocket clients.
type wsMessage struct {
	Type string `json:"type"`
	Data any    `json:"data"`
	Time string `json:"time"`
}

// WSHub manages WebSocket connections and broadcasts Bus events.
type WSHub struct {
	bus   *engine.Bus
	mu    sync.RWMutex
	conns map[*websocket.Conn]context.CancelFunc
}

// NewWSHub creates a hub and subscribes to relevant Bus events.
func NewWSHub(bus *engine.Bus) *WSHub {
	h := &WSHub{
		bus:   bus,
		conns: make(map[*websocket.Conn]context.CancelFunc),
	}

	// Subscribe to HTTP events for the live trace tab.
	bus.Subscribe(engine.EventHTTPRequestReceived, func(e engine.Event) {
		h.broadcast(wsMessage{
			Type: "HTTP_REQUEST",
			Data: e.Data,
			Time: time.Now().UTC().Format(time.RFC3339Nano),
		})
	})

	bus.Subscribe(engine.EventHTTPResponseSent, func(e engine.Event) {
		h.broadcast(wsMessage{
			Type: "HTTP_RESPONSE",
			Data: e.Data,
			Time: time.Now().UTC().Format(time.RFC3339Nano),
		})
	})

	// Subscribe to route/schema change events for live updates.
	for _, et := range []engine.EventType{
		engine.EventRouteAdded,
		engine.EventRouteUpdated,
		engine.EventRouteRemoved,
		engine.EventSchemaAltered,
		engine.EventScriptLoaded,
		engine.EventDataSeeded,
		engine.EventSnapshotCreated,
		engine.EventSnapshotRestored,
	} {
		eventType := et // capture loop variable
		bus.Subscribe(eventType, func(e engine.Event) {
			h.broadcast(wsMessage{
				Type: string(eventType),
				Data: e.Data,
				Time: time.Now().UTC().Format(time.RFC3339Nano),
			})
		})
	}

	return h
}

// HandleWS is the HTTP handler that upgrades to WebSocket.
func (h *WSHub) HandleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // Allow connections from any origin (local dev tool).
	})
	if err != nil {
		log.Printf("[console/ws] accept failed: %v", err)
		return
	}

	ctx, cancel := context.WithCancel(r.Context())

	h.mu.Lock()
	h.conns[conn] = cancel
	h.mu.Unlock()

	log.Printf("[console/ws] client connected (%d total)", h.count())

	// Send a welcome message.
	welcome := wsMessage{
		Type: "CONNECTED",
		Data: map[string]any{"message": "VibeServe console connected"},
		Time: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if data, err := json.Marshal(welcome); err == nil {
		_ = conn.Write(ctx, websocket.MessageText, data)
	}

	// Read loop — keeps the connection alive and detects disconnects.
	// We do not expect messages from the client, but we must drain reads.
	for {
		_, _, err := conn.Read(ctx)
		if err != nil {
			break
		}
	}

	h.remove(conn)
	conn.Close(websocket.StatusNormalClosure, "bye")
	log.Printf("[console/ws] client disconnected (%d remaining)", h.count())
}

// broadcast sends a message to all connected WebSocket clients.
func (h *WSHub) broadcast(msg wsMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for conn := range h.conns {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
			cancel()
			// Connection is dead — schedule removal.
			go h.remove(conn)
			continue
		}
		cancel()
	}
}

// remove cleans up a connection from the hub.
func (h *WSHub) remove(conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if cancel, ok := h.conns[conn]; ok {
		cancel()
		delete(h.conns, conn)
	}
}

// count returns the number of active connections.
func (h *WSHub) count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.conns)
}
```

- [ ] **Step 3: Verify compilation**

```bash
cd /Users/kent/Documents/Projects/vibeserve
go vet ./internal/web/...
```

---

### Task 3: Frontend — Single HTML File

**Files:**
- Create: `internal/web/static/index.html`

Build the entire web console as a single self-contained HTML file. Inline CSS and JS — zero external dependencies. Five tabs: API Explorer, Database Browser, Script Viewer, HTTP Trace, Manifest. Dark theme matching the TUI color scheme.

- [ ] **Step 1: Create the static directory**

```bash
mkdir -p /Users/kent/Documents/Projects/vibeserve/internal/web/static
```

- [ ] **Step 2: Create `internal/web/static/index.html`**

Create `internal/web/static/index.html`:

```html
<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>VibeServe Console</title>
<style>
/* ============================================================
   Reset + Variables
   ============================================================ */
*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }

:root {
  --bg:         #111827;
  --bg-surface: #1F2937;
  --bg-raised:  #374151;
  --bg-input:   #0F172A;
  --text:       #E5E7EB;
  --text-muted: #6B7280;
  --text-dim:   #9CA3AF;
  --primary:    #7C3AED;
  --primary-hover: #6D28D9;
  --secondary:  #06B6D4;
  --success:    #22C55E;
  --error:      #EF4444;
  --warning:    #F59E0B;
  --border:     #374151;
  --border-focus: #7C3AED;
  --radius:     6px;
  --font-mono:  'SF Mono', 'Cascadia Code', 'Fira Code', 'JetBrains Mono', Consolas, monospace;
  --font-sans:  -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
}

html, body {
  height: 100%;
  background: var(--bg);
  color: var(--text);
  font-family: var(--font-sans);
  font-size: 14px;
  line-height: 1.5;
  overflow: hidden;
}

/* ============================================================
   Layout
   ============================================================ */
#app {
  display: flex;
  flex-direction: column;
  height: 100vh;
}

/* Header */
header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 20px;
  height: 52px;
  background: var(--bg-surface);
  border-bottom: 1px solid var(--border);
  flex-shrink: 0;
}

header .logo {
  display: flex;
  align-items: center;
  gap: 10px;
  font-weight: 700;
  font-size: 16px;
  color: var(--text);
}

header .logo span.accent {
  color: var(--primary);
}

header .status {
  font-size: 12px;
  color: var(--text-muted);
}

header .status .dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--success);
  margin-right: 6px;
  vertical-align: middle;
}

/* Tab Bar */
nav {
  display: flex;
  gap: 0;
  background: var(--bg-surface);
  border-bottom: 1px solid var(--border);
  padding: 0 20px;
  flex-shrink: 0;
}

nav button {
  background: none;
  border: none;
  color: var(--text-muted);
  font-family: var(--font-sans);
  font-size: 13px;
  font-weight: 500;
  padding: 10px 16px;
  cursor: pointer;
  border-bottom: 2px solid transparent;
  transition: color 0.15s, border-color 0.15s;
}

nav button:hover {
  color: var(--text);
}

nav button.active {
  color: var(--primary);
  border-bottom-color: var(--primary);
}

/* Tab Content */
main {
  flex: 1;
  overflow: hidden;
  position: relative;
}

.tab-panel {
  display: none;
  position: absolute;
  inset: 0;
  overflow: auto;
  padding: 20px;
}

.tab-panel.active {
  display: block;
}

/* ============================================================
   Shared Components
   ============================================================ */
.card {
  background: var(--bg-surface);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: 16px;
  margin-bottom: 16px;
}

.card h3 {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-dim);
  text-transform: uppercase;
  letter-spacing: 0.05em;
  margin-bottom: 12px;
}

table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
}

table th {
  text-align: left;
  padding: 8px 12px;
  font-weight: 600;
  color: var(--text-dim);
  border-bottom: 1px solid var(--border);
  white-space: nowrap;
}

table td {
  padding: 8px 12px;
  border-bottom: 1px solid var(--border);
  max-width: 300px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

table tr:hover td {
  background: rgba(124, 58, 237, 0.05);
}

.badge {
  display: inline-block;
  padding: 2px 8px;
  border-radius: 3px;
  font-size: 11px;
  font-weight: 700;
  font-family: var(--font-mono);
  text-transform: uppercase;
}

.badge-get    { background: rgba(34, 197, 94, 0.15); color: var(--success); }
.badge-post   { background: rgba(245, 158, 11, 0.15); color: var(--warning); }
.badge-put    { background: rgba(6, 182, 212, 0.15); color: var(--secondary); }
.badge-patch  { background: rgba(124, 58, 237, 0.15); color: var(--primary); }
.badge-delete { background: rgba(239, 68, 68, 0.15); color: var(--error); }

.btn {
  background: var(--primary);
  color: #fff;
  border: none;
  padding: 8px 16px;
  border-radius: var(--radius);
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  transition: background 0.15s;
  font-family: var(--font-sans);
}

.btn:hover { background: var(--primary-hover); }
.btn:disabled { opacity: 0.5; cursor: not-allowed; }

.btn-sm {
  padding: 4px 10px;
  font-size: 12px;
}

input, select, textarea {
  background: var(--bg-input);
  color: var(--text);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: 8px 12px;
  font-size: 13px;
  font-family: var(--font-sans);
  outline: none;
  transition: border-color 0.15s;
}

input:focus, select:focus, textarea:focus {
  border-color: var(--border-focus);
}

textarea {
  font-family: var(--font-mono);
  resize: vertical;
  min-height: 100px;
}

.empty-state {
  text-align: center;
  padding: 40px 20px;
  color: var(--text-muted);
}

.empty-state h4 {
  font-size: 16px;
  margin-bottom: 8px;
  color: var(--text-dim);
}

.loading {
  color: var(--text-muted);
  font-style: italic;
}

pre.code-block {
  background: var(--bg-input);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: 16px;
  font-family: var(--font-mono);
  font-size: 13px;
  line-height: 1.6;
  overflow-x: auto;
  white-space: pre;
  color: var(--text);
  tab-size: 2;
}

/* Status code coloring */
.status-2xx { color: var(--success); font-weight: 700; }
.status-3xx { color: var(--secondary); font-weight: 700; }
.status-4xx { color: var(--warning); font-weight: 700; }
.status-5xx { color: var(--error); font-weight: 700; }

/* ============================================================
   Tab 1: API Explorer
   ============================================================ */
#tab-api .api-layout {
  display: grid;
  grid-template-columns: 280px 1fr;
  gap: 16px;
  height: calc(100vh - 120px);
}

#tab-api .route-list {
  overflow-y: auto;
  border-right: 1px solid var(--border);
  padding-right: 16px;
}

#tab-api .route-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  border-radius: var(--radius);
  cursor: pointer;
  transition: background 0.1s;
  font-size: 13px;
}

#tab-api .route-item:hover {
  background: var(--bg-raised);
}

#tab-api .route-item.selected {
  background: rgba(124, 58, 237, 0.12);
  border: 1px solid rgba(124, 58, 237, 0.3);
}

#tab-api .route-item .path {
  font-family: var(--font-mono);
  font-size: 12px;
  color: var(--text);
}

#tab-api .request-panel {
  display: flex;
  flex-direction: column;
  gap: 16px;
  overflow-y: auto;
}

#tab-api .url-bar {
  display: flex;
  gap: 8px;
  align-items: center;
}

#tab-api .url-bar select {
  width: 100px;
  font-weight: 700;
}

#tab-api .url-bar input {
  flex: 1;
}

#tab-api .response-section {
  flex: 1;
  min-height: 200px;
}

#tab-api .response-header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 8px;
  font-size: 13px;
}

#tab-api .response-header .duration {
  color: var(--text-muted);
}

/* ============================================================
   Tab 2: Database Browser
   ============================================================ */
#tab-db .db-layout {
  display: grid;
  grid-template-columns: 220px 1fr;
  gap: 16px;
  height: calc(100vh - 120px);
}

#tab-db .table-list {
  overflow-y: auto;
}

#tab-db .table-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 8px 10px;
  border-radius: var(--radius);
  cursor: pointer;
  transition: background 0.1s;
  font-size: 13px;
  font-family: var(--font-mono);
}

#tab-db .table-item:hover {
  background: var(--bg-raised);
}

#tab-db .table-item.selected {
  background: rgba(124, 58, 237, 0.12);
  border: 1px solid rgba(124, 58, 237, 0.3);
}

#tab-db .table-item .row-count {
  font-size: 11px;
  color: var(--text-muted);
  background: var(--bg-raised);
  padding: 1px 6px;
  border-radius: 3px;
}

#tab-db .table-detail {
  overflow-y: auto;
}

#tab-db .pagination {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-top: 12px;
  font-size: 13px;
  color: var(--text-muted);
}

/* ============================================================
   Tab 3: Script Viewer
   ============================================================ */
#tab-scripts .scripts-layout {
  display: grid;
  grid-template-columns: 240px 1fr;
  gap: 16px;
  height: calc(100vh - 120px);
}

#tab-scripts .script-list {
  overflow-y: auto;
}

#tab-scripts .script-item {
  padding: 8px 10px;
  border-radius: var(--radius);
  cursor: pointer;
  transition: background 0.1s;
  font-size: 13px;
  font-family: var(--font-mono);
  color: var(--text);
}

#tab-scripts .script-item:hover {
  background: var(--bg-raised);
}

#tab-scripts .script-item.selected {
  background: rgba(124, 58, 237, 0.12);
  border: 1px solid rgba(124, 58, 237, 0.3);
}

#tab-scripts .script-detail {
  overflow-y: auto;
}

/* ============================================================
   Tab 4: HTTP Trace
   ============================================================ */
#tab-trace .trace-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;
}

#tab-trace .trace-header .ws-status {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--text-muted);
}

#tab-trace .trace-header .ws-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--error);
}

#tab-trace .trace-header .ws-dot.connected {
  background: var(--success);
}

#tab-trace .trace-list {
  height: calc(100vh - 200px);
  overflow-y: auto;
  font-family: var(--font-mono);
  font-size: 12px;
}

#tab-trace .trace-entry {
  display: grid;
  grid-template-columns: 80px 60px 1fr 80px 80px;
  gap: 8px;
  padding: 6px 10px;
  border-bottom: 1px solid var(--border);
  align-items: center;
}

#tab-trace .trace-entry:hover {
  background: rgba(124, 58, 237, 0.05);
}

#tab-trace .trace-time {
  color: var(--text-muted);
  font-size: 11px;
}

#tab-trace .trace-duration {
  color: var(--text-muted);
  text-align: right;
  font-size: 11px;
}

/* ============================================================
   Tab 5: Manifest
   ============================================================ */
#tab-manifest .manifest-container {
  height: calc(100vh - 140px);
  overflow: auto;
}

/* Scrollbar styling */
::-webkit-scrollbar { width: 8px; height: 8px; }
::-webkit-scrollbar-track { background: var(--bg); }
::-webkit-scrollbar-thumb { background: var(--bg-raised); border-radius: 4px; }
::-webkit-scrollbar-thumb:hover { background: var(--text-muted); }
</style>
</head>
<body>
<div id="app">

  <!-- Header -->
  <header>
    <div class="logo">
      <svg width="24" height="24" viewBox="0 0 24 24" fill="none">
        <rect x="2" y="2" width="20" height="20" rx="4" fill="#7C3AED"/>
        <path d="M7 12l3 3 7-7" stroke="#fff" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"/>
      </svg>
      <span><span class="accent">Vibe</span>Serve Console</span>
    </div>
    <div class="status">
      <span class="dot"></span>
      <span id="header-status">Connected</span>
    </div>
  </header>

  <!-- Tab Bar -->
  <nav id="tab-bar">
    <button class="active" data-tab="tab-api">API Explorer</button>
    <button data-tab="tab-db">Database</button>
    <button data-tab="tab-scripts">Scripts</button>
    <button data-tab="tab-trace">HTTP Trace</button>
    <button data-tab="tab-manifest">Manifest</button>
  </nav>

  <!-- Tab Content -->
  <main>

    <!-- Tab 1: API Explorer -->
    <div id="tab-api" class="tab-panel active">
      <div class="api-layout">
        <div class="route-list" id="api-route-list">
          <div class="loading">Loading routes...</div>
        </div>
        <div class="request-panel">
          <div class="url-bar">
            <select id="api-method">
              <option>GET</option>
              <option>POST</option>
              <option>PUT</option>
              <option>PATCH</option>
              <option>DELETE</option>
            </select>
            <input type="text" id="api-url" placeholder="/path" value="/">
            <button class="btn" id="api-send" onclick="sendRequest()">Send</button>
          </div>
          <div id="api-body-section" style="display:none;">
            <div class="card">
              <h3>Request Body (JSON)</h3>
              <textarea id="api-body" rows="6" placeholder='{"key": "value"}'></textarea>
            </div>
          </div>
          <div class="response-section">
            <div class="card">
              <h3>Response</h3>
              <div id="api-response-header" class="response-header" style="display:none;">
                <span id="api-response-status"></span>
                <span class="duration" id="api-response-duration"></span>
              </div>
              <pre class="code-block" id="api-response-body" style="min-height:150px;">Send a request to see the response here.</pre>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Tab 2: Database Browser -->
    <div id="tab-db" class="tab-panel">
      <div class="db-layout">
        <div class="table-list" id="db-table-list">
          <div class="loading">Loading tables...</div>
        </div>
        <div class="table-detail" id="db-table-detail">
          <div class="empty-state">
            <h4>Select a table</h4>
            <p>Choose a table from the left to browse rows and view the schema.</p>
          </div>
        </div>
      </div>
    </div>

    <!-- Tab 3: Script Viewer -->
    <div id="tab-scripts" class="tab-panel">
      <div class="scripts-layout">
        <div class="script-list" id="scripts-list">
          <div class="loading">Loading scripts...</div>
        </div>
        <div class="script-detail" id="scripts-detail">
          <div class="empty-state">
            <h4>Select a script</h4>
            <p>Choose a script from the left to view its Tengo source code.</p>
          </div>
        </div>
      </div>
    </div>

    <!-- Tab 4: HTTP Trace -->
    <div id="tab-trace" class="tab-panel">
      <div class="trace-header">
        <h3 style="color: var(--text-dim); font-size: 13px; text-transform: uppercase; letter-spacing: 0.05em;">
          Live HTTP Trace
        </h3>
        <div style="display: flex; align-items: center; gap: 12px;">
          <div class="ws-status">
            <span class="ws-dot" id="ws-dot"></span>
            <span id="ws-status-text">Disconnected</span>
          </div>
          <button class="btn btn-sm" onclick="clearTrace()">Clear</button>
        </div>
      </div>
      <div class="trace-list" id="trace-list">
        <div class="trace-entry" style="font-weight: 600; color: var(--text-dim); border-bottom: 2px solid var(--border);">
          <span>Time</span>
          <span>Method</span>
          <span>Path</span>
          <span style="text-align:right;">Status</span>
          <span style="text-align:right;">Duration</span>
        </div>
        <div class="empty-state" id="trace-empty">
          <p>Waiting for HTTP requests... Send a request to your API to see it appear here.</p>
        </div>
      </div>
    </div>

    <!-- Tab 5: Manifest -->
    <div id="tab-manifest" class="tab-panel">
      <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px;">
        <h3 style="color: var(--text-dim); font-size: 13px; text-transform: uppercase; letter-spacing: 0.05em;">
          Raw Manifest JSON
        </h3>
        <button class="btn btn-sm" onclick="loadManifest()">Reload</button>
      </div>
      <div class="manifest-container">
        <pre class="code-block" id="manifest-json">Loading...</pre>
      </div>
    </div>

  </main>
</div>

<script>
/* =============================================================
   State
   ============================================================= */
var routes = [];
var tables = [];
var scripts = [];
var selectedRoute = null;
var selectedTable = null;
var selectedScript = null;
var ws = null;
var traceEntries = [];
var MAX_TRACE_ENTRIES = 500;

/* =============================================================
   Tab Switching
   ============================================================= */
document.querySelectorAll('#tab-bar button').forEach(function(btn) {
  btn.addEventListener('click', function() {
    document.querySelectorAll('#tab-bar button').forEach(function(b) { b.classList.remove('active'); });
    document.querySelectorAll('.tab-panel').forEach(function(p) { p.classList.remove('active'); });
    btn.classList.add('active');
    document.getElementById(btn.dataset.tab).classList.add('active');

    // Lazy-load data when switching to a tab
    var tab = btn.dataset.tab;
    if (tab === 'tab-api' && routes.length === 0) loadRoutes();
    if (tab === 'tab-db' && tables.length === 0) loadTables();
    if (tab === 'tab-scripts' && scripts.length === 0) loadScripts();
    if (tab === 'tab-manifest') loadManifest();
    if (tab === 'tab-trace' && !ws) connectWebSocket();
  });
});

/* =============================================================
   Utility
   ============================================================= */
function escapeHtml(str) {
  var div = document.createElement('div');
  div.textContent = String(str);
  return div.innerHTML;
}

function methodBadge(method) {
  return '<span class="badge badge-' + method.toLowerCase() + '">' + method + '</span>';
}

function statusClass(code) {
  if (code >= 200 && code < 300) return 'status-2xx';
  if (code >= 300 && code < 400) return 'status-3xx';
  if (code >= 400 && code < 500) return 'status-4xx';
  return 'status-5xx';
}

function api(path) {
  return fetch('/_api' + path).then(function(resp) { return resp.json(); });
}

/* =============================================================
   Safe DOM builders — avoid raw innerHTML with user data
   ============================================================= */
function createEl(tag, attrs, children) {
  var el = document.createElement(tag);
  if (attrs) {
    Object.keys(attrs).forEach(function(k) {
      if (k === 'className') el.className = attrs[k];
      else if (k === 'textContent') el.textContent = attrs[k];
      else if (k.indexOf('on') === 0) el.addEventListener(k.slice(2).toLowerCase(), attrs[k]);
      else el.setAttribute(k, attrs[k]);
    });
  }
  if (children) {
    children.forEach(function(c) {
      if (typeof c === 'string') el.appendChild(document.createTextNode(c));
      else if (c) el.appendChild(c);
    });
  }
  return el;
}

/* =============================================================
   Tab 1: API Explorer
   ============================================================= */
function loadRoutes() {
  api('/routes').then(function(data) {
    routes = data;
    renderRouteList();
  }).catch(function() {
    var el = document.getElementById('api-route-list');
    el.textContent = '';
    el.appendChild(createEl('div', {className: 'empty-state'}, [
      createEl('p', {textContent: 'Failed to load routes.'})
    ]));
  });
}

function renderRouteList() {
  var el = document.getElementById('api-route-list');
  el.textContent = '';
  if (routes.length === 0) {
    el.appendChild(createEl('div', {className: 'empty-state'}, [
      createEl('h4', {textContent: 'No routes'}),
      createEl('p', {textContent: 'Describe your API in the TUI to create routes.'})
    ]));
    return;
  }
  routes.forEach(function(r, i) {
    var item = createEl('div', {
      className: 'route-item' + (selectedRoute === i ? ' selected' : ''),
      onClick: function() { selectRoute(i); }
    });
    var badgeSpan = document.createElement('span');
    badgeSpan.className = 'badge badge-' + r.method.toLowerCase();
    badgeSpan.textContent = r.method;
    item.appendChild(badgeSpan);
    item.appendChild(createEl('span', {className: 'path', textContent: r.path}));
    el.appendChild(item);
  });
}

function selectRoute(index) {
  selectedRoute = index;
  var r = routes[index];
  document.getElementById('api-method').value = r.method;
  document.getElementById('api-url').value = r.path;

  // Show/hide body editor based on method
  var showBody = ['POST', 'PUT', 'PATCH'].indexOf(r.method) !== -1;
  document.getElementById('api-body-section').style.display = showBody ? 'block' : 'none';

  // Try to auto-fill body from route request_body schema
  if (showBody && r.request_body) {
    var body = {};
    Object.keys(r.request_body).forEach(function(key) {
      var type = r.request_body[key];
      if (type === 'integer' || type === 'INTEGER') body[key] = 0;
      else if (type === 'boolean' || type === 'BOOLEAN') body[key] = false;
      else if (type === 'real' || type === 'REAL') body[key] = 0.0;
      else body[key] = '';
    });
    document.getElementById('api-body').value = JSON.stringify(body, null, 2);
  }

  renderRouteList();
}

// Show/hide body section when method changes manually
document.getElementById('api-method').addEventListener('change', function() {
  var showBody = ['POST', 'PUT', 'PATCH'].indexOf(this.value) !== -1;
  document.getElementById('api-body-section').style.display = showBody ? 'block' : 'none';
});

function sendRequest() {
  var method = document.getElementById('api-method').value;
  var url = document.getElementById('api-url').value;
  var bodyEl = document.getElementById('api-body');
  var statusEl = document.getElementById('api-response-status');
  var durationEl = document.getElementById('api-response-duration');
  var bodyOutEl = document.getElementById('api-response-body');
  var headerEl = document.getElementById('api-response-header');

  // Replace path params with placeholder values if still present
  var resolvedUrl = url;
  var paramMatches = url.match(/:(\w+)/g);
  if (paramMatches) {
    paramMatches.forEach(function(p) {
      var name = p.slice(1);
      var val = prompt('Value for ' + name + ':', '1');
      if (val !== null) {
        resolvedUrl = resolvedUrl.replace(p, val);
      }
    });
  }

  var opts = { method: method, headers: {} };
  if (['POST', 'PUT', 'PATCH'].indexOf(method) !== -1 && bodyEl.value.trim()) {
    opts.headers['Content-Type'] = 'application/json';
    opts.body = bodyEl.value;
  }

  bodyOutEl.textContent = 'Sending...';
  headerEl.style.display = 'none';

  var start = performance.now();
  fetch(resolvedUrl, opts).then(function(resp) {
    var elapsed = (performance.now() - start).toFixed(1);
    return resp.text().then(function(text) {
      headerEl.style.display = 'flex';
      statusEl.className = statusClass(resp.status);
      statusEl.textContent = resp.status + ' ' + resp.statusText;
      durationEl.textContent = elapsed + ' ms';

      // Try to pretty-print JSON
      try {
        var json = JSON.parse(text);
        bodyOutEl.textContent = JSON.stringify(json, null, 2);
      } catch(e) {
        bodyOutEl.textContent = text;
      }
    });
  }).catch(function(err) {
    headerEl.style.display = 'flex';
    statusEl.className = 'status-5xx';
    statusEl.textContent = 'Error';
    durationEl.textContent = '';
    bodyOutEl.textContent = err.message;
  });
}

/* =============================================================
   Tab 2: Database Browser
   ============================================================= */
function loadTables() {
  api('/tables').then(function(data) {
    tables = data;
    renderTableList();
  }).catch(function() {
    var el = document.getElementById('db-table-list');
    el.textContent = '';
    el.appendChild(createEl('div', {className: 'empty-state'}, [
      createEl('p', {textContent: 'Failed to load tables.'})
    ]));
  });
}

function renderTableList() {
  var el = document.getElementById('db-table-list');
  el.textContent = '';
  if (tables.length === 0) {
    el.appendChild(createEl('div', {className: 'empty-state'}, [
      createEl('h4', {textContent: 'No tables'})
    ]));
    return;
  }
  tables.forEach(function(t) {
    var item = createEl('div', {
      className: 'table-item' + (selectedTable === t.name ? ' selected' : ''),
      onClick: function() { selectTable(t.name); }
    }, [
      createEl('span', {textContent: t.name}),
      createEl('span', {className: 'row-count', textContent: t.row_count + ' rows'})
    ]);
    el.appendChild(item);
  });
}

function selectTable(name) {
  selectedTable = name;
  renderTableList();

  var detail = document.getElementById('db-table-detail');
  detail.textContent = '';
  detail.appendChild(createEl('div', {className: 'loading', textContent: 'Loading...'}));

  // Find table metadata
  var table = tables.find(function(t) { return t.name === name; });

  api('/tables/' + encodeURIComponent(name) + '/rows?limit=50&offset=0').then(function(data) {
    detail.textContent = '';

    // Schema card
    if (table && table.columns) {
      var schemaCard = createEl('div', {className: 'card'});
      schemaCard.appendChild(createEl('h3', {textContent: 'Schema: ' + name}));
      var schemaTable = createEl('table');
      var thead = createEl('tr');
      ['Column', 'Type', 'PK', 'Not Null'].forEach(function(h) {
        thead.appendChild(createEl('th', {textContent: h}));
      });
      schemaTable.appendChild(thead);
      table.columns.forEach(function(col) {
        var row = createEl('tr');
        var nameCell = createEl('td', {style: 'font-family: var(--font-mono);', textContent: col.name});
        var typeCell = createEl('td');
        typeCell.appendChild(createEl('span', {
          className: 'badge',
          style: 'background: var(--bg-raised); color: var(--secondary);',
          textContent: col.type
        }));
        row.appendChild(nameCell);
        row.appendChild(typeCell);
        row.appendChild(createEl('td', {textContent: col.pk ? 'Yes' : ''}));
        row.appendChild(createEl('td', {textContent: col.not_null ? 'Yes' : ''}));
        schemaTable.appendChild(row);
      });
      schemaCard.appendChild(schemaTable);
      detail.appendChild(schemaCard);
    }

    // Data card
    var dataCard = createEl('div', {className: 'card'});
    dataCard.appendChild(createEl('h3', {textContent: 'Data (' + data.total + ' rows)'}));

    if (data.rows && data.rows.length > 0) {
      var cols = Object.keys(data.rows[0]);
      var scrollDiv = createEl('div', {style: 'overflow-x: auto;'});
      var dataTable = createEl('table');
      var dhead = createEl('tr');
      cols.forEach(function(c) { dhead.appendChild(createEl('th', {textContent: c})); });
      dataTable.appendChild(dhead);
      data.rows.forEach(function(row) {
        var tr = createEl('tr');
        cols.forEach(function(c) {
          var val = row[c];
          var td = createEl('td');
          if (val === null) {
            td.appendChild(createEl('span', {style: 'color: var(--text-muted);', textContent: 'null'}));
          } else {
            td.textContent = String(val);
          }
          tr.appendChild(td);
        });
        dataTable.appendChild(tr);
      });
      scrollDiv.appendChild(dataTable);
      dataCard.appendChild(scrollDiv);

      // Pagination
      var pagination = createEl('div', {className: 'pagination'}, [
        createEl('span', {textContent: 'Showing ' + data.rows.length + ' of ' + data.total})
      ]);
      if (data.total > 50) {
        pagination.appendChild(createEl('button', {
          className: 'btn btn-sm',
          textContent: 'Next 50',
          onClick: function() { loadTablePage(name, 50); }
        }));
      }
      dataCard.appendChild(pagination);
    } else {
      dataCard.appendChild(createEl('div', {className: 'empty-state'}, [
        createEl('p', {textContent: 'No rows in this table.'})
      ]));
    }

    detail.appendChild(dataCard);
  }).catch(function(e) {
    detail.textContent = '';
    detail.appendChild(createEl('div', {className: 'empty-state'}, [
      createEl('p', {textContent: 'Error loading table: ' + e.message})
    ]));
  });
}

function loadTablePage(name, offset) {
  // Simplified: reload table from offset (full re-render)
  selectTable(name);
}

/* =============================================================
   Tab 3: Script Viewer
   ============================================================= */
function loadScripts() {
  api('/scripts').then(function(data) {
    scripts = data;
    renderScriptList();
  }).catch(function() {
    var el = document.getElementById('scripts-list');
    el.textContent = '';
    el.appendChild(createEl('div', {className: 'empty-state'}, [
      createEl('p', {textContent: 'Failed to load scripts.'})
    ]));
  });
}

function renderScriptList() {
  var el = document.getElementById('scripts-list');
  el.textContent = '';
  if (scripts.length === 0) {
    el.appendChild(createEl('div', {className: 'empty-state'}, [
      createEl('h4', {textContent: 'No scripts'})
    ]));
    return;
  }
  scripts.forEach(function(s) {
    el.appendChild(createEl('div', {
      className: 'script-item' + (selectedScript === s.name ? ' selected' : ''),
      textContent: s.name,
      onClick: function() { selectScript(s.name); }
    }));
  });
}

function selectScript(name) {
  selectedScript = name;
  renderScriptList();

  var s = scripts.find(function(sc) { return sc.name === name; });
  if (!s) return;

  var detail = document.getElementById('scripts-detail');
  detail.textContent = '';
  var card = createEl('div', {className: 'card'});
  card.appendChild(createEl('h3', {textContent: s.name}));
  card.appendChild(createEl('pre', {className: 'code-block', textContent: s.code}));
  detail.appendChild(card);
}

/* =============================================================
   Tab 4: HTTP Trace (WebSocket)
   ============================================================= */
function connectWebSocket() {
  var proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  var url = proto + '//' + location.host + '/_ws';

  ws = new WebSocket(url);

  ws.onopen = function() {
    document.getElementById('ws-dot').classList.add('connected');
    document.getElementById('ws-status-text').textContent = 'Connected';
  };

  ws.onclose = function() {
    document.getElementById('ws-dot').classList.remove('connected');
    document.getElementById('ws-status-text').textContent = 'Disconnected';
    ws = null;
    // Auto-reconnect after 3 seconds
    setTimeout(function() {
      var traceBtn = document.querySelector('[data-tab="tab-trace"]');
      if (traceBtn && traceBtn.classList.contains('active')) {
        connectWebSocket();
      }
    }, 3000);
  };

  ws.onerror = function() {
    document.getElementById('ws-dot').classList.remove('connected');
    document.getElementById('ws-status-text').textContent = 'Error';
  };

  ws.onmessage = function(event) {
    try {
      var msg = JSON.parse(event.data);
      handleWSMessage(msg);
    } catch (e) {
      // Ignore non-JSON messages
    }
  };
}

// Buffer to match request/response pairs for duration calculation.
var pendingRequests = {};

function handleWSMessage(msg) {
  if (msg.type === 'CONNECTED') return;

  if (msg.type === 'HTTP_REQUEST') {
    var data = msg.data || {};
    var key = (data.method || '?') + ' ' + (data.path || '?');
    pendingRequests[key] = { time: msg.time, method: data.method, path: data.path };
    return; // Wait for the response to render both together.
  }

  if (msg.type === 'HTTP_RESPONSE') {
    var data = msg.data || {};
    var key = (data.method || '?') + ' ' + (data.path || '?');
    delete pendingRequests[key];

    addTraceEntry({
      time: msg.time,
      method: data.method || '?',
      path: data.path || '?',
      status: data.status || 0,
      duration: data.duration_ms != null ? data.duration_ms + ' ms' : '-'
    });
    return;
  }

  // Other events — show as system notification in trace
  var detail = typeof msg.data === 'string' ? msg.data : JSON.stringify(msg.data);
  addTraceEntry({
    time: msg.time,
    method: 'SYS',
    path: msg.type + ': ' + detail,
    status: '-',
    duration: '-'
  });
}

function addTraceEntry(entry) {
  // Hide empty state
  var emptyEl = document.getElementById('trace-empty');
  if (emptyEl) emptyEl.style.display = 'none';

  traceEntries.unshift(entry);
  if (traceEntries.length > MAX_TRACE_ENTRIES) {
    traceEntries.pop();
  }

  var list = document.getElementById('trace-list');

  // Build the new row using safe DOM methods
  var timeStr = entry.time ? new Date(entry.time).toLocaleTimeString() : '-';
  var isSys = entry.method === 'SYS';
  var statusStr = String(entry.status);

  var row = createEl('div', {className: 'trace-entry'});

  row.appendChild(createEl('span', {className: 'trace-time', textContent: timeStr}));

  var methodCell = document.createElement('span');
  if (isSys) {
    methodCell.appendChild(createEl('span', {style: 'color:var(--text-muted)', textContent: 'SYS'}));
  } else {
    var badge = createEl('span', {className: 'badge badge-' + entry.method.toLowerCase(), textContent: entry.method});
    methodCell.appendChild(badge);
  }
  row.appendChild(methodCell);

  var pathCell = createEl('span', {textContent: entry.path});
  if (isSys) pathCell.style.cssText = 'color:var(--text-muted);font-style:italic;';
  row.appendChild(pathCell);

  var statusCell = createEl('span', {style: 'text-align:right;', textContent: statusStr});
  if (typeof entry.status === 'number') statusCell.className = statusClass(entry.status);
  row.appendChild(statusCell);

  row.appendChild(createEl('span', {className: 'trace-duration', textContent: String(entry.duration)}));

  // Insert after the header row (first child)
  var header = list.firstElementChild;
  if (header && header.nextSibling) {
    list.insertBefore(row, header.nextSibling);
  } else {
    list.appendChild(row);
  }
}

function clearTrace() {
  traceEntries = [];
  var list = document.getElementById('trace-list');
  // Keep the header row, remove everything else
  while (list.children.length > 1) {
    list.removeChild(list.lastChild);
  }
  // Re-add empty state
  list.appendChild(createEl('div', {className: 'empty-state', id: 'trace-empty'}, [
    createEl('p', {textContent: 'Waiting for HTTP requests...'})
  ]));
}

/* =============================================================
   Tab 5: Manifest
   ============================================================= */
function loadManifest() {
  api('/manifest').then(function(data) {
    document.getElementById('manifest-json').textContent = JSON.stringify(data, null, 2);
  }).catch(function(e) {
    document.getElementById('manifest-json').textContent = 'Error loading manifest: ' + e.message;
  });
}

/* =============================================================
   Keyboard Shortcuts
   ============================================================= */
document.addEventListener('keydown', function(e) {
  // Ctrl/Cmd + Enter to send request in API Explorer
  if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
    var apiTab = document.getElementById('tab-api');
    if (apiTab.classList.contains('active')) {
      e.preventDefault();
      sendRequest();
    }
  }

  // Ctrl/Cmd + 1-5 to switch tabs
  if ((e.ctrlKey || e.metaKey) && e.key >= '1' && e.key <= '5') {
    e.preventDefault();
    var tabs = document.querySelectorAll('#tab-bar button');
    var idx = parseInt(e.key) - 1;
    if (tabs[idx]) tabs[idx].click();
  }
});

/* =============================================================
   Init
   ============================================================= */
(function init() {
  // Load initial data for the default tab (API Explorer)
  loadRoutes();
  // Pre-connect WebSocket so trace is ready when user switches
  connectWebSocket();
})();
</script>
</body>
</html>
```

- [ ] **Step 3: Verify file was created**

```bash
wc -l /Users/kent/Documents/Projects/vibeserve/internal/web/static/index.html
```

---

### Task 4: Embed + Wire into Server

**Files:**
- Create: `internal/web/embed.go`
- Modify: `cmd/vibeserve/main.go` — replace direct handler with mux that routes console + API trie

Wire everything together. The `embed.go` file uses `//go:embed` to bundle the HTML file. The `main.go` changes create a `http.ServeMux` that dispatches `/_console/`, `/_api/`, and `/_ws` to the console, and everything else to the existing trie handler.

- [ ] **Step 1: Create `internal/web/embed.go`**

Create `internal/web/embed.go`:

```go
package web

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/vibeserve/vibeserve/internal/engine"
)

//go:embed static/*
var staticFiles embed.FS

// NewConsoleMux creates an http.ServeMux that routes:
//   - /_console/  -> embedded static files (the web UI)
//   - /_api/*     -> REST API endpoints
//   - /_ws        -> WebSocket event bridge
//   - everything else -> the provided fallback handler (API trie)
func NewConsoleMux(eng *engine.Engine, store ConsoleStore, bus *engine.Bus, fallback http.Handler) *http.ServeMux {
	mux := http.NewServeMux()

	// 1. Static files: serve from embedded FS.
	// Strip the "static/" prefix so /_console/index.html maps to static/index.html.
	staticFS, _ := fs.Sub(staticFiles, "static")
	fileServer := http.FileServer(http.FS(staticFS))

	// Serve /_console/ — redirect bare /_console to /_console/.
	mux.HandleFunc("/_console", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/_console/", http.StatusMovedPermanently)
	})
	mux.Handle("/_console/", http.StripPrefix("/_console/", fileServer))

	// 2. REST API endpoints.
	console := NewConsole(eng, store)
	console.RegisterRoutes(mux)

	// 3. WebSocket.
	wsHub := NewWSHub(bus)
	mux.HandleFunc("/_ws", wsHub.HandleWS)

	// 4. Everything else goes to the trie handler.
	mux.Handle("/", fallback)

	return mux
}
```

- [ ] **Step 2: Modify `cmd/vibeserve/main.go` — `runDev()` function**

In the `runDev()` function, replace the direct handler/server creation with the console mux. Find these lines:

```go
	rt := runtime.New(s, bus)
	handler := router.NewHandler(trie, scripts, rt, cfg.Server.CORS)
	srv := router.NewServer(cfg.Server.Host, cfg.Server.Port, handler)

	eng := engine.NewEngine(engine.EngineConfig{
```

Replace the block from `rt := runtime.New(...)` through the creation of `srv` (but before `eng :=`) with this reordered version that creates `eng` before the server:

```go
	rt := runtime.New(s, bus)
	apiHandler := router.NewHandler(trie, scripts, rt, cfg.Server.CORS)

	eng := engine.NewEngine(engine.EngineConfig{
		Bus:      bus,
		Store:    s,
		Trie:     trie,
		Scripts:  scripts,
		Provider: provider,
		Manifest: m,
		VibeDir:  ".vibe",
		StoreOpener: func(dsn string) (engine.SchemaStore, error) {
			return store.New(dsn)
		},
	})

	// Wrap the API handler with the web console mux.
	consoleMux := web.NewConsoleMux(eng, s, bus, apiHandler)
	srv := router.NewServer(cfg.Server.Host, cfg.Server.Port, consoleMux)
```

Add the import for the web package to the import block:

```go
	"github.com/vibeserve/vibeserve/internal/web"
```

- [ ] **Step 3: Modify `cmd/vibeserve/main.go` — `runUp()` function**

Similarly wrap the handler in `runUp()`. Find:

```go
	rt := runtime.New(s, bus)
	handler := router.NewHandler(trie, scripts, rt, true)
	srv := router.NewServer(host, port, handler)
```

Replace with:

```go
	rt := runtime.New(s, bus)
	apiHandler := router.NewHandler(trie, scripts, rt, true)

	// Create a minimal engine for the console (read-only, no LLM provider needed).
	eng := engine.NewEngine(engine.EngineConfig{
		Bus:      bus,
		Store:    s,
		Trie:     trie,
		Scripts:  scripts,
		Manifest: m,
		VibeDir:  ".vibe",
	})

	consoleMux := web.NewConsoleMux(eng, s, bus, apiHandler)
	srv := router.NewServer(host, port, consoleMux)
```

- [ ] **Step 4: Update the log lines to mention the console**

In `runDev()`, after the existing `log.Printf("Server running at ...")` line, add:

```go
	log.Printf("Console at http://%s:%d/_console", cfg.Server.Host, cfg.Server.Port)
```

In `runUp()`, after the server start, add before the select statement:

```go
	log.Printf("Console at http://%s:%d/_console", host, port)
```

- [ ] **Step 5: Verify compilation**

```bash
cd /Users/kent/Documents/Projects/vibeserve
go vet ./...
go build ./cmd/vibeserve/...
```

---

### Task 5: Polish + Smoke Test

**Files:** None created — this is a verification and polish task.

Run the full test suite, start the server, and verify every tab in the web console works correctly.

- [ ] **Step 1: Run all tests**

```bash
cd /Users/kent/Documents/Projects/vibeserve
go test ./... -count=1 -timeout=30s
```

Fix any compilation or test failures before proceeding.

- [ ] **Step 2: Start the server with a test manifest**

Use one of the existing testdata manifests or create a quick one:

```bash
cd /Users/kent/Documents/Projects/vibeserve
# Use vibeserve up with testdata if available, or start dev mode
go run ./cmd/vibeserve up -m testdata/car_rental_manifest.json -p 8080
```

- [ ] **Step 3: Manual smoke test checklist**

Open `http://localhost:8080/_console` in a browser and verify each tab:

**Console loads:**
- [ ] Page loads without errors in the browser console
- [ ] Header shows "VibeServe Console" with green connected dot
- [ ] All 5 tabs are visible

**API Explorer tab:**
- [ ] Routes appear in the left sidebar
- [ ] Clicking a route fills in the method and URL
- [ ] POST/PUT routes show the body editor
- [ ] Clicking "Send" on a GET route returns data with status 200
- [ ] Response shows status code, duration, and formatted JSON
- [ ] Sending a request to a non-existent route returns 404

**Database tab:**
- [ ] Tables appear in the left sidebar with row counts
- [ ] Clicking a table shows the schema (column names, types, PK, NOT NULL)
- [ ] Clicking a table shows the row data in a table grid
- [ ] Null values display as "null" in muted text
- [ ] Pagination info shows (e.g., "Showing 3 of 3")

**Scripts tab:**
- [ ] Script names appear in the left sidebar
- [ ] Clicking a script shows the Tengo source code
- [ ] Code is displayed in a monospace code block

**HTTP Trace tab:**
- [ ] WebSocket status shows "Connected" with green dot
- [ ] Sending a request from the API Explorer tab causes a trace entry to appear
- [ ] Trace entries show time, method badge, path, status, and duration
- [ ] Clear button removes all entries
- [ ] System events (route added, schema altered, etc.) appear with "SYS" label

**Manifest tab:**
- [ ] Full manifest JSON is displayed, pretty-printed
- [ ] Reload button refreshes the JSON

**Keyboard shortcuts:**
- [ ] Ctrl/Cmd+1 through Ctrl/Cmd+5 switch tabs
- [ ] Ctrl/Cmd+Enter sends request in API Explorer

- [ ] **Step 4: Verify existing API routes are unaffected**

Confirm that the API trie handler still works for non-console paths:

```bash
# These should still work exactly as before
curl http://localhost:8080/cars
curl -X POST http://localhost:8080/cars -H 'Content-Type: application/json' -d '{"make":"Toyota","model":"Camry"}'
```

- [ ] **Step 5: Verify console paths do not leak into the API trie**

```bash
# These should be handled by the console, not the trie
curl -s http://localhost:8080/_api/manifest | head -c 100
curl -s http://localhost:8080/_api/routes | head -c 100
curl -s http://localhost:8080/_api/tables | head -c 100
curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/_console/
```

The `/_console/` request should return 200 with HTML. The `/_api/*` requests should return JSON.

- [ ] **Step 6: Run full test suite one final time**

```bash
cd /Users/kent/Documents/Projects/vibeserve
go test ./... -count=1 -timeout=60s -race
```

---

## Summary

| Task | Files | What it does |
|------|-------|-------------|
| 1 | `store.go` (modify), `web/console.go` (new), `web/console_test.go` (new) | Store.Tables() + REST API handlers for /_api/* |
| 2 | `go.mod` (modify), `web/websocket.go` (new) | WebSocket event bridge at /_ws |
| 3 | `web/static/index.html` (new) | Full SPA -- 5 tabs, dark theme, vanilla HTML/CSS/JS |
| 4 | `web/embed.go` (new), `main.go` (modify) | Embed HTML + wire mux into server |
| 5 | (none) | Run tests, smoke test all tabs, verify API still works |

**New dependencies:** `nhooyr.io/websocket`

**Key design decisions:**
- Single `http.ServeMux` dispatches console vs API -- no middleware chain needed
- Console REST endpoints read from Engine.Manifest() and Store -- no duplication of data
- WebSocket hub subscribes to existing Bus events -- no new event types needed
- Single HTML file with inline CSS/JS -- no build tools, CDNs, or frameworks
- Frontend uses safe DOM construction (`createElement`/`textContent`) instead of innerHTML with user data
- `InsecureSkipVerify: true` on WebSocket accept -- appropriate for a local dev tool
- Table name validation (alphanumeric + underscore only) prevents SQL injection on the rows endpoint
