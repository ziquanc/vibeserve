package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

const (
	ollamaDefaultPath = "/api/chat"
)

// OllamaProvider calls a local Ollama instance to generate manifests.
type OllamaProvider struct {
	host   string // e.g. "http://localhost:11434"
	model  string
	client *http.Client
}

// OllamaOption configures an OllamaProvider.
type OllamaOption func(*OllamaProvider)

// WithOllamaClient overrides the HTTP client.
func WithOllamaClient(client *http.Client) OllamaOption {
	return func(o *OllamaProvider) { o.client = client }
}

// NewOllamaProvider creates a Provider that calls a local Ollama instance.
func NewOllamaProvider(host, model string, opts ...OllamaOption) *OllamaProvider {
	p := &OllamaProvider{
		host:   host,
		model:  model,
		client: http.DefaultClient,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// ollamaRequest is the JSON body sent to the Ollama chat API.
type ollamaRequest struct {
	Model    string      `json:"model"`
	Messages []ollamaMsg `json:"messages"`
	Stream   bool        `json:"stream"`
}

type ollamaMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ollamaResponse is the JSON response from the Ollama chat API.
type ollamaResponse struct {
	Message ollamaRespMsg `json:"message"`
	Error   string        `json:"error,omitempty"`
}

type ollamaRespMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Generate implements Provider.
func (o *OllamaProvider) Generate(ctx context.Context, current *manifest.Manifest, prompt string, history []Message) (*manifest.Manifest, error) {
	systemPrompt := BuildSystemPrompt(current)

	// Build messages: system + history + current prompt
	var msgs []ollamaMsg
	msgs = append(msgs, ollamaMsg{Role: "system", Content: systemPrompt})
	for _, h := range history {
		msgs = append(msgs, ollamaMsg{Role: h.Role, Content: h.Content})
	}
	msgs = append(msgs, ollamaMsg{Role: "user", Content: prompt})

	reqBody := ollamaRequest{
		Model:    o.model,
		Messages: msgs,
		Stream:   false,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := o.host + ollamaDefaultPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama API request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var ollamaResp ollamaResponse
	if err := json.Unmarshal(respBody, &ollamaResp); err != nil {
		return nil, fmt.Errorf("parse ollama response: %w", err)
	}

	if ollamaResp.Error != "" {
		return nil, fmt.Errorf("ollama error: %s", ollamaResp.Error)
	}

	text := ollamaResp.Message.Content
	if text == "" {
		return nil, fmt.Errorf("ollama returned empty content")
	}

	return ParseManifestResponse(text)
}
