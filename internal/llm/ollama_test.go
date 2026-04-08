package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

func TestOllamaProvider_Generate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.URL.Path != "/api/chat" {
			t.Errorf("expected path /api/chat, got %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		var reqBody ollamaRequest
		json.NewDecoder(r.Body).Decode(&reqBody)

		if reqBody.Model != "llama3" {
			t.Errorf("expected model 'llama3', got %q", reqBody.Model)
		}
		if reqBody.Stream != false {
			t.Error("expected stream=false")
		}
		// Should have: system + user = 2 messages
		if len(reqBody.Messages) < 2 {
			t.Errorf("expected at least 2 messages, got %d", len(reqBody.Messages))
		}
		if reqBody.Messages[0].Role != "system" {
			t.Errorf("expected first message role 'system', got %q", reqBody.Messages[0].Role)
		}

		resp := ollamaResponse{
			Message: ollamaRespMsg{
				Role:    "assistant",
				Content: `{"version": "1.0", "name": "test-api", "description": "Test", "schemas": [], "routes": [], "scripts": [], "seeds": []}`,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewOllamaProvider(server.URL, "llama3")

	m, err := provider.Generate(context.Background(), nil, "Create a todo API", nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if m.Name != "test-api" {
		t.Errorf("expected name 'test-api', got %q", m.Name)
	}
}

func TestOllamaProvider_Generate_WithHistory(t *testing.T) {
	var receivedMessages int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody ollamaRequest
		json.NewDecoder(r.Body).Decode(&reqBody)
		receivedMessages = len(reqBody.Messages)

		resp := ollamaResponse{
			Message: ollamaRespMsg{
				Role:    "assistant",
				Content: `{"version": "1.0", "name": "test", "description": "", "schemas": [], "routes": [], "scripts": [], "seeds": []}`,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewOllamaProvider(server.URL, "llama3")

	history := []Message{
		{Role: "user", Content: "Create a todo API"},
		{Role: "assistant", Content: `{"version": "1.0"}`},
	}

	_, err := provider.Generate(context.Background(), nil, "Add a description field", history)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	// system (1) + history (2) + current prompt (1) = 4 messages
	if receivedMessages != 4 {
		t.Errorf("expected 4 messages, got %d", receivedMessages)
	}
}

func TestOllamaProvider_Generate_WithCurrentManifest(t *testing.T) {
	var receivedSystem string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody ollamaRequest
		json.NewDecoder(r.Body).Decode(&reqBody)
		if len(reqBody.Messages) > 0 && reqBody.Messages[0].Role == "system" {
			receivedSystem = reqBody.Messages[0].Content
		}

		resp := ollamaResponse{
			Message: ollamaRespMsg{
				Content: `{"version": "1.0", "name": "test", "description": "", "schemas": [], "routes": [], "scripts": [], "seeds": []}`,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewOllamaProvider(server.URL, "llama3")
	current := &manifest.Manifest{Version: "1.0", Name: "existing-api"}

	_, err := provider.Generate(context.Background(), current, "Add users", nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if receivedSystem == "" {
		t.Fatal("expected non-empty system prompt")
	}
}

func TestOllamaProvider_Generate_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "model not found"}`))
	}))
	defer server.Close()

	provider := NewOllamaProvider(server.URL, "nonexistent")

	_, err := provider.Generate(context.Background(), nil, "test", nil)
	if err == nil {
		t.Error("expected error for API error response")
	}
}

func TestOllamaProvider_Generate_OllamaError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ollamaResponse{Error: "model 'foo' not found"}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewOllamaProvider(server.URL, "foo")

	_, err := provider.Generate(context.Background(), nil, "test", nil)
	if err == nil {
		t.Error("expected error for ollama error response")
	}
}

func TestOllamaProvider_Generate_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ollamaResponse{
			Message: ollamaRespMsg{Content: "I am not able to produce JSON right now."},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewOllamaProvider(server.URL, "llama3")

	_, err := provider.Generate(context.Background(), nil, "test", nil)
	if err == nil {
		t.Error("expected error for non-JSON LLM response")
	}
}

func TestOllamaProvider_Generate_EmptyContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ollamaResponse{Message: ollamaRespMsg{Content: ""}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewOllamaProvider(server.URL, "llama3")

	_, err := provider.Generate(context.Background(), nil, "test", nil)
	if err == nil {
		t.Error("expected error for empty content")
	}
}

func TestOllamaProvider_Generate_MarkdownWrappedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ollamaResponse{
			Message: ollamaRespMsg{
				Content: "Here is the manifest:\n```json\n{\"version\": \"1.0\", \"name\": \"wrapped\", \"description\": \"\", \"schemas\": [], \"routes\": [], \"scripts\": [], \"seeds\": []}\n```",
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewOllamaProvider(server.URL, "llama3")

	m, err := provider.Generate(context.Background(), nil, "test", nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if m.Name != "wrapped" {
		t.Errorf("expected name 'wrapped', got %q", m.Name)
	}
}
