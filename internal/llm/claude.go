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
	claudeDefaultURL    = "https://api.anthropic.com/v1/messages"
	claudeAPIVersion    = "2023-06-01"
	claudeDefaultMaxTok = 4096
)

// ClaudeProvider calls the Anthropic Messages API to generate manifests.
type ClaudeProvider struct {
	apiKey    string
	model     string
	baseURL   string // overridable for testing
	client    *http.Client
	maxTokens int
}

// ClaudeOption configures a ClaudeProvider.
type ClaudeOption func(*ClaudeProvider)

// WithClaudeBaseURL overrides the API base URL (for testing with httptest).
func WithClaudeBaseURL(url string) ClaudeOption {
	return func(c *ClaudeProvider) { c.baseURL = url }
}

// WithClaudeClient overrides the HTTP client.
func WithClaudeClient(client *http.Client) ClaudeOption {
	return func(c *ClaudeProvider) { c.client = client }
}

// WithClaudeMaxTokens overrides the max_tokens parameter.
func WithClaudeMaxTokens(n int) ClaudeOption {
	return func(c *ClaudeProvider) { c.maxTokens = n }
}

// NewClaudeProvider creates a Provider that calls the Claude API.
func NewClaudeProvider(apiKey, model string, opts ...ClaudeOption) *ClaudeProvider {
	p := &ClaudeProvider{
		apiKey:    apiKey,
		model:     model,
		baseURL:   claudeDefaultURL,
		client:    http.DefaultClient,
		maxTokens: claudeDefaultMaxTok,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// claudeRequest is the JSON body sent to the Claude API.
type claudeRequest struct {
	Model     string      `json:"model"`
	MaxTokens int         `json:"max_tokens"`
	System    string      `json:"system"`
	Messages  []claudeMsg `json:"messages"`
}

type claudeMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// claudeResponse is the JSON response from the Claude API.
type claudeResponse struct {
	Content []claudeContent `json:"content"`
	Error   *claudeError    `json:"error,omitempty"`
}

type claudeContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type claudeError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// Generate implements Provider.
func (c *ClaudeProvider) Generate(ctx context.Context, current *manifest.Manifest, prompt string, history []Message) (*manifest.Manifest, error) {
	systemPrompt := BuildSystemPrompt(current)

	// Build messages: history + current prompt
	var msgs []claudeMsg
	for _, h := range history {
		msgs = append(msgs, claudeMsg{Role: h.Role, Content: h.Content})
	}
	msgs = append(msgs, claudeMsg{Role: "user", Content: prompt})

	reqBody := claudeRequest{
		Model:     c.model,
		MaxTokens: c.maxTokens,
		System:    systemPrompt,
		Messages:  msgs,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", claudeAPIVersion)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("claude API request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("claude API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var claudeResp claudeResponse
	if err := json.Unmarshal(respBody, &claudeResp); err != nil {
		return nil, fmt.Errorf("parse claude response: %w", err)
	}

	if claudeResp.Error != nil {
		return nil, fmt.Errorf("claude error: %s: %s", claudeResp.Error.Type, claudeResp.Error.Message)
	}

	if len(claudeResp.Content) == 0 {
		return nil, fmt.Errorf("claude returned empty content")
	}

	// Find the text content block
	var text string
	for _, block := range claudeResp.Content {
		if block.Type == "text" {
			text = block.Text
			break
		}
	}
	if text == "" {
		return nil, fmt.Errorf("claude returned no text content")
	}

	return ParseManifestResponse(text)
}

// Chat sends a prompt and returns the raw text response (no JSON parsing).
func (c *ClaudeProvider) Chat(ctx context.Context, systemPrompt string, prompt string) (string, error) {
	msgs := []claudeMsg{
		{Role: "user", Content: prompt},
	}

	reqBody := claudeRequest{
		Model:     c.model,
		MaxTokens: c.maxTokens,
		System:    systemPrompt,
		Messages:  msgs,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", claudeAPIVersion)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("claude API request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("claude API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var claudeResp claudeResponse
	if err := json.Unmarshal(respBody, &claudeResp); err != nil {
		return "", fmt.Errorf("parse claude response: %w", err)
	}

	if claudeResp.Error != nil {
		return "", fmt.Errorf("claude error: %s: %s", claudeResp.Error.Type, claudeResp.Error.Message)
	}

	if len(claudeResp.Content) == 0 {
		return "", fmt.Errorf("claude returned empty content")
	}

	for _, block := range claudeResp.Content {
		if block.Type == "text" {
			return block.Text, nil
		}
	}

	return "", fmt.Errorf("claude returned no text content")
}
