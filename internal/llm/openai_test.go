package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAIProvider_Generate(t *testing.T) {
	manifestJSON := `{"version":"1.0","name":"test","description":"test","schemas":[],"routes":[],"scripts":[],"seeds":[]}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request format
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Bearer auth, got %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected json content type")
		}

		var req openaiRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Model != "glm-4" {
			t.Errorf("expected model 'glm-4', got %q", req.Model)
		}
		// Should have system + user messages
		if len(req.Messages) < 2 {
			t.Errorf("expected at least 2 messages, got %d", len(req.Messages))
		}
		if req.Messages[0].Role != "system" {
			t.Errorf("expected first message role 'system', got %q", req.Messages[0].Role)
		}

		resp := openaiResponse{
			Choices: []openaiChoice{{
				Message: openaiMsg{Role: "assistant", Content: manifestJSON},
			}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewOpenAIProvider("test-key", "glm-4", server.URL)
	m, err := p.Generate(context.Background(), nil, "create a test API", nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if m.Name != "test" {
		t.Errorf("expected name 'test', got %q", m.Name)
	}
}

func TestOpenAIProvider_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"rate limited","type":"rate_limit"}}`))
	}))
	defer server.Close()

	p := NewOpenAIProvider("test-key", "glm-4", server.URL)
	_, err := p.Generate(context.Background(), nil, "test", nil)
	if err == nil {
		t.Error("expected error for 429 response")
	}
}

func TestOpenAIProvider_EmptyChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(openaiResponse{Choices: []openaiChoice{}})
	}))
	defer server.Close()

	p := NewOpenAIProvider("test-key", "glm-4", server.URL)
	_, err := p.Generate(context.Background(), nil, "test", nil)
	if err == nil {
		t.Error("expected error for empty choices")
	}
}

func TestOpenAIProvider_WithHistory(t *testing.T) {
	var receivedMsgs []openaiMsg

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req openaiRequest
		json.NewDecoder(r.Body).Decode(&req)
		receivedMsgs = req.Messages

		manifestJSON := `{"version":"1.0","name":"test","description":"","schemas":[],"routes":[],"scripts":[],"seeds":[]}`
		resp := openaiResponse{
			Choices: []openaiChoice{{Message: openaiMsg{Content: manifestJSON}}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewOpenAIProvider("test-key", "glm-4", server.URL)
	history := []Message{
		{Role: "user", Content: "create users API"},
		{Role: "assistant", Content: "done"},
	}
	p.Generate(context.Background(), nil, "add posts", history)

	// system + 2 history + 1 new = 4 messages
	if len(receivedMsgs) != 4 {
		t.Errorf("expected 4 messages, got %d", len(receivedMsgs))
	}
}
