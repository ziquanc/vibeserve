package cloud

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSyncProject_CreatesWhenNoID(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"project": map[string]any{"id": "proj-abc", "name": "My API"},
		})
	}))
	defer srv.Close()

	vibeDir := t.TempDir()
	c := NewClient(srv.URL, "tok")

	id, err := SyncProject(c, vibeDir, SyncInput{Name: "My API", TableCount: 2, RouteCount: 5})
	if err != nil {
		t.Fatalf("SyncProject: %v", err)
	}
	if id != "proj-abc" {
		t.Errorf("id: got %q, want proj-abc", id)
	}
	if gotMethod != "POST" || gotPath != "/api/projects" {
		t.Errorf("route: got %s %s, want POST /api/projects", gotMethod, gotPath)
	}
	if gotBody["name"] != "My API" {
		t.Errorf("body.name: got %v", gotBody["name"])
	}

	// Local project.json should now exist
	saved, err := LoadProjectID(vibeDir)
	if err != nil {
		t.Fatalf("LoadProjectID: %v", err)
	}
	if saved != "proj-abc" {
		t.Errorf("saved id: got %q, want proj-abc", saved)
	}
}

func TestSyncProject_PatchesWhenIDExists(t *testing.T) {
	var gotMethod, gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"project": map[string]any{"id": "proj-existing"},
		})
	}))
	defer srv.Close()

	vibeDir := t.TempDir()
	if err := SaveProjectID(vibeDir, "proj-existing"); err != nil {
		t.Fatal(err)
	}

	c := NewClient(srv.URL, "tok")
	id, err := SyncProject(c, vibeDir, SyncInput{Name: "My API", TableCount: 3, RouteCount: 6})
	if err != nil {
		t.Fatalf("SyncProject: %v", err)
	}
	if id != "proj-existing" {
		t.Errorf("id: got %q, want proj-existing", id)
	}
	if gotMethod != "PATCH" || gotPath != "/api/projects/proj-existing" {
		t.Errorf("route: got %s %s, want PATCH /api/projects/proj-existing", gotMethod, gotPath)
	}
}
