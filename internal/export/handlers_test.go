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
