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
