package export

import (
	"strings"
	"testing"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

func TestGenerateExpressRoutes_PathParamValidation(t *testing.T) {
	schemas := []manifest.Schema{{
		Table: "users",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "slug", Type: "TEXT"},
		},
	}}
	routes := []manifest.Route{{
		Path: "/users/:slug", Method: "GET", Script: "get_user",
	}}
	scripts := []manifest.Script{{
		Name: "get_user",
		Code: `user := db.query_one("SELECT * FROM users WHERE slug = ?", [slug])
response.json(user)`,
	}}

	files := GenerateExpressRoutes(schemas, routes, scripts)
	content := files["users.js"]

	if strings.Contains(content, "param('slug').isInt()") {
		t.Error("TEXT path param 'slug' should not be validated as isInt()")
	}
}

func TestGenerateExpressRoutes_NoStringMatchAuth(t *testing.T) {
	schemas := []manifest.Schema{}
	routes := []manifest.Route{{
		Path: "/unprotected-data", Method: "GET", Script: "get_data",
	}}
	scripts := []manifest.Script{{
		Name: "get_data",
		Code: `response.json("ok")`,
	}}

	files := GenerateExpressRoutes(schemas, routes, scripts)
	content := files["unprotected-data.js"]

	if strings.Contains(content, "require('../middleware/auth')") {
		t.Error("should not add auth middleware based on path string matching")
	}
}

func TestGenerateExpressRoutes_TryCatch(t *testing.T) {
	schemas := []manifest.Schema{{
		Table: "users",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
		},
	}}
	routes := []manifest.Route{{
		Path: "/users", Method: "GET", Script: "list_users",
	}}
	scripts := []manifest.Script{{
		Name: "list_users",
		Code: `rows := db.query("SELECT * FROM users", [])
response.json(rows)`,
	}}

	files := GenerateExpressRoutes(schemas, routes, scripts)
	content := files["users.js"]

	if !strings.Contains(content, "try {") {
		t.Error("route handlers should have try/catch for error handling")
	}
	if !strings.Contains(content, "next(err)") {
		t.Error("catch block should call next(err) to use Express error handler")
	}
}

func TestGenerateExpressRoutes_AsyncPostgres(t *testing.T) {
	schemas := []manifest.Schema{{
		Table: "users",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "name", Type: "TEXT"},
		},
	}}
	routes := []manifest.Route{{
		Path: "/users", Method: "GET", Script: "list_users",
	}}
	scripts := []manifest.Script{{
		Name: "list_users",
		Code: `rows := db.query("SELECT * FROM users", [])
response.json(rows)`,
	}}

	files := GenerateExpressRoutesWithDB(schemas, routes, scripts, "postgres")
	content := files["users.js"]

	if !strings.Contains(content, "async (req, res, next)") {
		t.Error("postgres route handlers should be async")
	}
	if !strings.Contains(content, "await") {
		t.Error("postgres DB calls should use await")
	}
	if !strings.Contains(content, "pool.query") {
		t.Error("postgres should use pool.query not database.prepare")
	}
}

func TestGenerateExpressRoutes_SqliteUnchanged(t *testing.T) {
	schemas := []manifest.Schema{{
		Table: "users",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "name", Type: "TEXT"},
		},
	}}
	routes := []manifest.Route{{
		Path: "/users", Method: "GET", Script: "list_users",
	}}
	scripts := []manifest.Script{{
		Name: "list_users",
		Code: `rows := db.query("SELECT * FROM users", [])
response.json(rows)`,
	}}

	files := GenerateExpressRoutesWithDB(schemas, routes, scripts, "sqlite")
	content := files["users.js"]

	if strings.Contains(content, "async (req, res, next)") {
		t.Error("sqlite route handlers should NOT be async")
	}
	if strings.Contains(content, "pool.query") {
		t.Error("sqlite should use database.prepare not pool.query")
	}
	if !strings.Contains(content, "database.prepare") {
		t.Error("sqlite should use database.prepare")
	}
}
