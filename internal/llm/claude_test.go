package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

func TestClaudeProvider_Generate(t *testing.T) {
	// Create a mock Claude API server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request headers
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("expected x-api-key 'test-key', got %q", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Errorf("expected anthropic-version '2023-06-01', got %q", r.Header.Get("anthropic-version"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type 'application/json', got %q", r.Header.Get("Content-Type"))
		}

		// Verify request body
		var reqBody claudeRequest
		json.NewDecoder(r.Body).Decode(&reqBody)
		if reqBody.Model != "test-model" {
			t.Errorf("expected model 'test-model', got %q", reqBody.Model)
		}
		if reqBody.System == "" {
			t.Error("expected non-empty system prompt")
		}
		if len(reqBody.Messages) == 0 {
			t.Error("expected at least one message")
		}

		// Return a valid manifest
		resp := claudeResponse{
			Content: []claudeContent{
				{
					Type: "text",
					Text: `{"version": "1.0", "name": "test-api", "description": "Test", "schemas": [], "routes": [], "scripts": [], "seeds": []}`,
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewClaudeProvider("test-key", "test-model", WithClaudeBaseURL(server.URL))

	m, err := provider.Generate(context.Background(), nil, "Create a todo API", nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if m.Name != "test-api" {
		t.Errorf("expected name 'test-api', got %q", m.Name)
	}
}

func TestClaudeProvider_Generate_WithHistory(t *testing.T) {
	var receivedMessages int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody claudeRequest
		json.NewDecoder(r.Body).Decode(&reqBody)
		receivedMessages = len(reqBody.Messages)

		resp := claudeResponse{
			Content: []claudeContent{
				{Type: "text", Text: `{"version": "1.0", "name": "test", "description": "", "schemas": [], "routes": [], "scripts": [], "seeds": []}`},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewClaudeProvider("key", "model", WithClaudeBaseURL(server.URL))

	history := []Message{
		{Role: "user", Content: "Create a todo API"},
		{Role: "assistant", Content: `{"version": "1.0", "name": "test"}`},
	}

	_, err := provider.Generate(context.Background(), nil, "Add a description field", history)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	// History (2) + current prompt (1) = 3 messages
	if receivedMessages != 3 {
		t.Errorf("expected 3 messages, got %d", receivedMessages)
	}
}

func TestClaudeProvider_Generate_WithCurrentManifest(t *testing.T) {
	var receivedSystem string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody claudeRequest
		json.NewDecoder(r.Body).Decode(&reqBody)
		receivedSystem = reqBody.System

		resp := claudeResponse{
			Content: []claudeContent{
				{Type: "text", Text: `{"version": "1.0", "name": "current", "description": "", "schemas": [], "routes": [], "scripts": [], "seeds": []}`},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewClaudeProvider("key", "model", WithClaudeBaseURL(server.URL))

	current := &manifest.Manifest{
		Version: "1.0",
		Name:    "my-existing-api",
	}

	_, err := provider.Generate(context.Background(), current, "Add users", nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if receivedSystem == "" {
		t.Fatal("expected non-empty system prompt")
	}
	if len(receivedSystem) < 100 {
		t.Error("system prompt suspiciously short")
	}
}

func TestClaudeProvider_Generate_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error": {"type": "rate_limit", "message": "Too many requests"}}`))
	}))
	defer server.Close()

	provider := NewClaudeProvider("key", "model", WithClaudeBaseURL(server.URL))

	_, err := provider.Generate(context.Background(), nil, "test", nil)
	if err == nil {
		t.Error("expected error for API error response")
	}
}

func TestClaudeProvider_Generate_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := claudeResponse{
			Content: []claudeContent{
				{Type: "text", Text: "I cannot help with that request."},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewClaudeProvider("key", "model", WithClaudeBaseURL(server.URL))

	_, err := provider.Generate(context.Background(), nil, "test", nil)
	if err == nil {
		t.Error("expected error for non-JSON response")
	}
}

func TestClaudeProvider_Generate_EmptyContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := claudeResponse{Content: []claudeContent{}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewClaudeProvider("key", "model", WithClaudeBaseURL(server.URL))

	_, err := provider.Generate(context.Background(), nil, "test", nil)
	if err == nil {
		t.Error("expected error for empty content")
	}
}
