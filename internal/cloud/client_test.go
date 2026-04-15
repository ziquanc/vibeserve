package cloud

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_AttachesBearerToken(t *testing.T) {
	var gotAuth string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		_ = json.NewEncoder(w).Encode(map[string]string{"ok": "yes"})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test-token-xyz")

	resp, err := c.Post("/api/projects", map[string]any{"name": "Test"})
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	defer resp.Body.Close()

	if gotAuth != "Bearer test-token-xyz" {
		t.Errorf("auth header: got %q, want Bearer test-token-xyz", gotAuth)
	}
	if !bytes.Contains(gotBody, []byte(`"name":"Test"`)) {
		t.Errorf("body: got %s, want contains name:Test", gotBody)
	}
	if resp.StatusCode != 201 {
		t.Errorf("status: got %d, want 201", resp.StatusCode)
	}
}

func TestClient_ErrorOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"error":"Unauthorized"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "bad-token")
	_, err := c.Get("/api/projects")
	if err == nil {
		t.Fatal("want error for 401, got nil")
	}
}
