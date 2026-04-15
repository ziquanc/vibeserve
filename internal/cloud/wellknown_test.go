package cloud

import (
	"encoding/json"
	"testing"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

func TestGenerateWellKnown(t *testing.T) {
	m := &manifest.Manifest{
		Name:        "Coffee Corner",
		Description: "Local coffee shop with delivery",
		Routes: []manifest.Route{
			{Method: "GET", Path: "/menu"},
			{Method: "POST", Path: "/orders"},
			{Method: "GET", Path: "/orders/:id"},
		},
	}

	got := GenerateWellKnown(m)

	if got.Name != "Coffee Corner" {
		t.Errorf("Name: got %q, want Coffee Corner", got.Name)
	}
	if got.Description != "Local coffee shop with delivery" {
		t.Errorf("Description: got %q", got.Description)
	}
	if got.OpenAPI != "/_api/openapi.yaml" {
		t.Errorf("OpenAPI: got %q, want /_api/openapi.yaml", got.OpenAPI)
	}
	if len(got.Endpoints) != 3 {
		t.Fatalf("Endpoints: got %d, want 3", len(got.Endpoints))
	}
	if got.Endpoints[0].Method != "GET" || got.Endpoints[0].Path != "/menu" {
		t.Errorf("Endpoint[0]: got %+v", got.Endpoints[0])
	}

	// JSON shape
	data, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !contains(string(data), `"endpoints":[{"method":"GET","path":"/menu"}`) {
		t.Errorf("JSON missing expected endpoints field: %s", data)
	}
}

func TestGenerateWellKnown_EmptyManifest(t *testing.T) {
	got := GenerateWellKnown(&manifest.Manifest{})
	if got.Name != "" {
		t.Errorf("empty manifest should yield empty name, got %q", got.Name)
	}
	if got.Endpoints == nil {
		t.Error("Endpoints should be empty slice, not nil (for stable JSON)")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
