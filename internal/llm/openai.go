package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
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
	Role             string `json:"role"`
	Content          string `json:"content"`
	ReasoningContent string `json:"reasoning_content,omitempty"` // some models (DeepSeek, GLM) use this
}

type openaiResponse struct {
	Choices []openaiChoice `json:"choices"`
	Error   *openaiError   `json:"error,omitempty"`
}

type openaiChoice struct {
	Message      openaiMsg `json:"message"`
	FinishReason string    `json:"finish_reason,omitempty"`
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
		Model:     o.model,
		MaxTokens: o.maxTokens,
		Messages:  msgs,
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

	log.Printf("[openai] response status: %d, body length: %d bytes", resp.StatusCode, len(respBody))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	// Log raw response for debugging (truncated)
	rawPreview := string(respBody)
	if len(rawPreview) > 500 {
		rawPreview = rawPreview[:500] + "..."
	}
	log.Printf("[openai] raw response: %s", rawPreview)

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

	// Try multiple content sources — different providers use different fields
	text := oaiResp.Choices[0].Message.Content
	if text == "" {
		text = oaiResp.Choices[0].Message.ReasoningContent
	}
	if text == "" {
		// Last resort: try to find content anywhere in the raw JSON
		var rawMap map[string]any
		json.Unmarshal(respBody, &rawMap)
		text = findContentInRaw(rawMap)
	}
	if text == "" {
		return nil, fmt.Errorf("openai returned empty content. Raw response: %s", rawPreview)
	}

	log.Printf("[openai] extracted text length: %d chars", len(text))

	return ParseManifestResponse(text)
}

// findContentInRaw walks a raw JSON map looking for any string content field.
// Handles providers that nest content in unexpected structures.
func findContentInRaw(raw map[string]any) string {
	// Try choices[0].message.content (standard)
	if choices, ok := raw["choices"].([]any); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]any); ok {
			if msg, ok := choice["message"].(map[string]any); ok {
				// Try all known content field names
				for _, key := range []string{"content", "reasoning_content", "text"} {
					if v, ok := msg[key].(string); ok && v != "" {
						return v
					}
				}
			}
			// Some providers put content directly on choice
			if v, ok := choice["text"].(string); ok && v != "" {
				return v
			}
		}
	}
	// Try output field (some newer APIs)
	if v, ok := raw["output"].(string); ok && v != "" {
		return v
	}
	return ""
}
