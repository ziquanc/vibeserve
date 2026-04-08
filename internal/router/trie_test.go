package router

import (
	"testing"
)

func TestTrieStaticRoutes(t *testing.T) {
	tr := NewTrie()
	tr.Insert("GET", "/users", "list_users")
	tr.Insert("POST", "/users", "create_user")

	script, params, found := tr.Search("GET", "/users")
	if !found {
		t.Fatal("expected GET /users to be found")
	}
	if script != "list_users" {
		t.Errorf("expected script=list_users, got %q", script)
	}
	if len(params) != 0 {
		t.Errorf("expected no params, got %v", params)
	}

	script, params, found = tr.Search("POST", "/users")
	if !found {
		t.Fatal("expected POST /users to be found")
	}
	if script != "create_user" {
		t.Errorf("expected script=create_user, got %q", script)
	}
	if len(params) != 0 {
		t.Errorf("expected no params, got %v", params)
	}
}

func TestTrieParameterRoute(t *testing.T) {
	tr := NewTrie()
	tr.Insert("GET", "/users/:id", "get_user")

	script, params, found := tr.Search("GET", "/users/42")
	if !found {
		t.Fatal("expected GET /users/42 to be found")
	}
	if script != "get_user" {
		t.Errorf("expected script=get_user, got %q", script)
	}
	if params["id"] != "42" {
		t.Errorf("expected params[id]=42, got %q", params["id"])
	}
}

func TestTrieNestedParameterRoute(t *testing.T) {
	tr := NewTrie()
	tr.Insert("GET", "/users/:id/posts/:post_id", "get_user_post")

	script, params, found := tr.Search("GET", "/users/7/posts/99")
	if !found {
		t.Fatal("expected GET /users/7/posts/99 to be found")
	}
	if script != "get_user_post" {
		t.Errorf("expected script=get_user_post, got %q", script)
	}
	if params["id"] != "7" {
		t.Errorf("expected params[id]=7, got %q", params["id"])
	}
	if params["post_id"] != "99" {
		t.Errorf("expected params[post_id]=99, got %q", params["post_id"])
	}
}

func TestTrieExactMatchBeforeParam(t *testing.T) {
	tr := NewTrie()
	tr.Insert("GET", "/users/:id", "get_user")
	tr.Insert("GET", "/users/me", "get_me")

	script, params, found := tr.Search("GET", "/users/me")
	if !found {
		t.Fatal("expected GET /users/me to be found")
	}
	if script != "get_me" {
		t.Errorf("expected script=get_me (exact match), got %q", script)
	}
	if len(params) != 0 {
		t.Errorf("expected no params for exact match, got %v", params)
	}

	// Parameterized fallback should still work
	script, params, found = tr.Search("GET", "/users/123")
	if !found {
		t.Fatal("expected GET /users/123 to be found")
	}
	if script != "get_user" {
		t.Errorf("expected script=get_user, got %q", script)
	}
	if params["id"] != "123" {
		t.Errorf("expected params[id]=123, got %q", params["id"])
	}
}

func TestTrieNotFound(t *testing.T) {
	tr := NewTrie()
	tr.Insert("GET", "/users", "list_users")

	_, _, found := tr.Search("GET", "/nonexistent")
	if found {
		t.Error("expected GET /nonexistent to not be found")
	}

	_, _, found = tr.Search("DELETE", "/users")
	if found {
		t.Error("expected DELETE /users to not be found")
	}
}

func TestTrieRemove(t *testing.T) {
	tr := NewTrie()
	tr.Insert("GET", "/users", "list_users")
	tr.Insert("POST", "/users", "create_user")

	// Confirm both exist
	_, _, found := tr.Search("GET", "/users")
	if !found {
		t.Fatal("expected GET /users before removal")
	}

	tr.Remove("GET", "/users")

	_, _, found = tr.Search("GET", "/users")
	if found {
		t.Error("expected GET /users to be removed")
	}

	// POST should still exist
	script, _, found := tr.Search("POST", "/users")
	if !found {
		t.Fatal("expected POST /users to still exist after removing GET")
	}
	if script != "create_user" {
		t.Errorf("expected script=create_user, got %q", script)
	}
}

func TestTrieRoutes(t *testing.T) {
	tr := NewTrie()
	tr.Insert("GET", "/users", "list_users")
	tr.Insert("POST", "/users", "create_user")
	tr.Insert("GET", "/users/:id", "get_user")

	routes := tr.Routes()
	if len(routes) != 3 {
		t.Errorf("expected 3 routes, got %d", len(routes))
	}

	// Build a lookup for verification
	type key struct{ method, path string }
	lookup := make(map[key]string)
	for _, r := range routes {
		lookup[key{r.Method, r.Path}] = r.Script
	}

	if s := lookup[key{"GET", "/users"}]; s != "list_users" {
		t.Errorf("expected GET /users → list_users, got %q", s)
	}
	if s := lookup[key{"POST", "/users"}]; s != "create_user" {
		t.Errorf("expected POST /users → create_user, got %q", s)
	}
	if s := lookup[key{"GET", "/:id"}]; s == "get_user" {
		t.Errorf("unexpected: GET /:id returned get_user (expected /users/:id path)")
	}
}

func TestTrieMethodCaseInsensitive(t *testing.T) {
	tr := NewTrie()
	tr.Insert("get", "/ping", "ping_handler")

	script, _, found := tr.Search("GET", "/ping")
	if !found {
		t.Fatal("expected GET /ping to be found")
	}
	if script != "ping_handler" {
		t.Errorf("expected script=ping_handler, got %q", script)
	}
}
