package runtime

import (
	"testing"

	"github.com/vibeserve/vibeserve/internal/engine"
	"github.com/vibeserve/vibeserve/internal/manifest"
	"github.com/vibeserve/vibeserve/internal/store"
)

// newTestDeps creates in-memory store, bus, a zero RequestContext, and a fresh ResponseCapture.
func newTestDeps(t *testing.T) (engine.DataStore, *engine.Bus, *RequestContext, *ResponseCapture) {
	t.Helper()
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	schema := manifest.Schema{
		Table: "items",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "title", Type: "TEXT"},
			{Name: "price", Type: "REAL"},
		},
	}
	if err := s.ApplySchemas([]manifest.Schema{schema}); err != nil {
		t.Fatalf("apply schemas: %v", err)
	}
	if err := s.Seed("items", []map[string]any{
		{"title": "Widget", "price": 9.99},
		{"title": "Gadget", "price": 19.99},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	bus := engine.NewBus()
	rc := &RequestContext{}
	capture := &ResponseCapture{}
	return s, bus, rc, capture
}

// newTestRuntime creates a Runtime backed by an in-memory store with 2 seed rows.
func newTestRuntime(t *testing.T) (*Runtime, engine.DataStore) {
	t.Helper()
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	schema := manifest.Schema{
		Table: "items",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "title", Type: "TEXT"},
			{Name: "price", Type: "REAL"},
		},
	}
	if err := s.ApplySchemas([]manifest.Schema{schema}); err != nil {
		t.Fatalf("apply schemas: %v", err)
	}
	if err := s.Seed("items", []map[string]any{
		{"title": "Widget", "price": 9.99},
		{"title": "Gadget", "price": 19.99},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rt := New(s, engine.NewBus())
	return rt, s
}

func TestExecuteSimpleQuery(t *testing.T) {
	rt, _ := newTestRuntime(t)
	rc := &RequestContext{Method: "GET"}
	code := `
result := db.query("SELECT * FROM items", [])
response.json(result)
`
	status, body, _, err := rt.Execute(code, rc)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
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
	rt, _ := newTestRuntime(t)
	rc := &RequestContext{
		Method: "POST",
		Body:   map[string]any{"title": "Thingamajig", "price": 4.99},
	}
	code := `
data := request.body()
row := db.insert("items", data)
response.json(row, 201)
`
	status, body, _, err := rt.Execute(code, rc)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if status != 201 {
		t.Errorf("expected status 201, got %d", status)
	}
	m, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected map body, got %T", body)
	}
	if m["title"] != "Thingamajig" {
		t.Errorf("expected title=Thingamajig, got %v", m["title"])
	}
}

func TestExecuteErrorResponse(t *testing.T) {
	rt, _ := newTestRuntime(t)
	rc := &RequestContext{Method: "GET"}
	code := `
response.fail(404, "not found")
`
	status, body, _, err := rt.Execute(code, rc)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if status != 404 {
		t.Errorf("expected status 404, got %d", status)
	}
	m, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected map body, got %T", body)
	}
	if m["error"] != "not found" {
		t.Errorf("expected error='not found', got %v", m["error"])
	}
}

func TestExecutePathParams(t *testing.T) {
	rt, _ := newTestRuntime(t)
	rc := &RequestContext{
		Method:     "GET",
		PathParams: map[string]string{"id": "1"},
	}
	code := `
id := request.param("id")
row := db.query_one("SELECT * FROM items WHERE id = ?", [id])
if is_undefined(row) {
  response.fail(404, "not found")
} else {
  response.json(row)
}
`
	status, body, _, err := rt.Execute(code, rc)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if status != 200 {
		t.Errorf("expected status 200, got %d", status)
	}
	m, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected map body, got %T", body)
	}
	if m["title"] != "Widget" {
		t.Errorf("expected title=Widget, got %v", m["title"])
	}
}

func TestExecuteNoResponse(t *testing.T) {
	rt, _ := newTestRuntime(t)
	rc := &RequestContext{Method: "GET"}
	code := `
x := 1 + 1
_ := x
`
	_, _, _, err := rt.Execute(code, rc)
	if err == nil {
		t.Fatal("expected error for script with no response")
	}
}

func TestExecuteDateDiffDays(t *testing.T) {
	rt, _ := newTestRuntime(t)
	rc := &RequestContext{Method: "GET"}
	code := `
diff := date.diff_days("2026-04-01", "2026-04-10")
response.json(diff)
`
	status, body, _, err := rt.Execute(code, rc)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if status != 200 {
		t.Errorf("expected status 200, got %d", status)
	}
	// body should be 9 (as int64 from ToInterface)
	var diff int64
	switch v := body.(type) {
	case int64:
		diff = v
	case int:
		diff = int64(v)
	default:
		t.Fatalf("expected numeric body, got %T (%v)", body, body)
	}
	if diff != 9 {
		t.Errorf("expected diff=9, got %d", diff)
	}
}
