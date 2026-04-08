package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

const openaiDefaultMaxTok = 8192

// OpenAIProvider calls any OpenAI-compatible API (OpenAI, x.ai, z.ai, Groq, Together, etc.)
type OpenAIProvider struct {
	apiKey    string
	model     string
	baseURL   string // e.g. "https://open.bigmodel.cn/api/paas/v4/chat/completions"
	client    *http.Client
	maxTokens int
}

// OpenAIOption configures an OpenAIProvider.
type OpenAIOption func(*OpenAIProvider)

// WithOpenAIClient overrides the HTTP client.
func WithOpenAIClient(client *http.Client) OpenAIOption {
	return func(o *OpenAIProvider) { o.client = client }
}

// WithOpenAIMaxTokens overrides the max_tokens parameter.
func WithOpenAIMaxTokens(n int) OpenAIOption {
	return func(o *OpenAIProvider) { o.maxTokens = n }
}

// NewOpenAIProvider creates a Provider that calls an OpenAI-compatible API.
// baseURL should be the full chat completions endpoint, e.g.:
//   - "https://api.openai.com/v1/chat/completions"
//   - "https://open.bigmodel.cn/api/paas/v4/chat/completions"
//   - "https://api.x.ai/v1/chat/completions"
func NewOpenAIProvider(apiKey, model, baseURL string, opts ...OpenAIOption) *OpenAIProvider {
	// Auto-append /chat/completions if not present
	if !strings.HasSuffix(baseURL, "/chat/completions") {
		baseURL = strings.TrimRight(baseURL, "/") + "/chat/completions"
	}

	p := &OpenAIProvider{
		apiKey:    apiKey,
		model:     model,
		baseURL:   baseURL,
		client:    http.DefaultClient,
		maxTokens: openaiDefaultMaxTok,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

type openaiRequest struct {
	Model          string            `json:"model"`
	MaxTokens      int               `json:"max_tokens"`
	Messages       []openaiMsg       `json:"messages"`
	ResponseFormat *openaiRespFormat `json:"response_format,omitempty"`
}

type openaiRespFormat struct {
	Type string `json:"type"`
}

type openaiMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openaiResponse struct {
	Choices []openaiChoice `json:"choices"`
	Error   *openaiError   `json:"error,omitempty"`
}

type openaiChoice struct {
	Message openaiMsg `json:"message"`
}

type openaiError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

// Generate implements Provider.
func (o *OpenAIProvider) Generate(ctx context.Context, current *manifest.Manifest, prompt string, history []Message) (*manifest.Manifest, error) {
	systemPrompt := BuildSystemPrompt(current)

	// Build messages: system + history + user prompt
	msgs := []openaiMsg{{Role: "system", Content: systemPrompt}}
	for _, h := range history {
		msgs = append(msgs, openaiMsg{Role: h.Role, Content: h.Content})
	}
	msgs = append(msgs, openaiMsg{Role: "user", Content: prompt})

	reqBody := openaiRequest{
		Model:          o.model,
		MaxTokens:      o.maxTokens,
		Messages:       msgs,
		ResponseFormat: &openaiRespFormat{Type: "json_object"},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai API request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var oaiResp openaiResponse
	if err := json.Unmarshal(respBody, &oaiResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if oaiResp.Error != nil {
		return nil, fmt.Errorf("openai error: %s", oaiResp.Error.Message)
	}

	if len(oaiResp.Choices) == 0 {
		return nil, fmt.Errorf("openai returned no choices")
	}

	text := oaiResp.Choices[0].Message.Content
	if text == "" {
		return nil, fmt.Errorf("openai returned empty content")
	}

	return ParseManifestResponse(text)
}
