package llm

import (
	"bufio"
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
	baseURL   string
	client    *http.Client
	maxTokens int
	// OnChunk is called with accumulated text as each SSE chunk arrives.
	// Use this to show streaming progress in the TUI.
	OnChunk func(accumulated string)
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

// WithOnChunk sets a callback for streaming progress.
func WithOnChunk(fn func(accumulated string)) OpenAIOption {
	return func(o *OpenAIProvider) { o.OnChunk = fn }
}

// NewOpenAIProvider creates a Provider that calls an OpenAI-compatible API.
func NewOpenAIProvider(apiKey, model, baseURL string, opts ...OpenAIOption) *OpenAIProvider {
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
	Model     string      `json:"model"`
	MaxTokens int         `json:"max_tokens"`
	Messages  []openaiMsg `json:"messages"`
	Stream    bool        `json:"stream"`
}

type openaiMsg struct {
	Role             string `json:"role"`
	Content          string `json:"content"`
	ReasoningContent string `json:"reasoning_content,omitempty"`
}

// SSE streaming response types
type streamChunk struct {
	Choices []streamChoice `json:"choices"`
	Error   *openaiError   `json:"error,omitempty"`
}

type streamChoice struct {
	Delta        openaiMsg `json:"delta"`
	FinishReason *string   `json:"finish_reason"`
}

// Non-streaming response types (fallback)
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

// Generate implements Provider. Uses SSE streaming for progressive output.
func (o *OpenAIProvider) Generate(ctx context.Context, current *manifest.Manifest, prompt string, history []Message) (*manifest.Manifest, error) {
	systemPrompt := BuildSystemPrompt(current)

	msgs := []openaiMsg{{Role: "system", Content: systemPrompt}}
	for _, h := range history {
		msgs = append(msgs, openaiMsg{Role: h.Role, Content: h.Content})
	}
	msgs = append(msgs, openaiMsg{Role: "user", Content: prompt})

	reqBody := openaiRequest{
		Model:     o.model,
		MaxTokens: o.maxTokens,
		Messages:  msgs,
		Stream:    true,
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
	req.Header.Set("Accept", "text/event-stream")

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai API request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai API error (status %d): %s", resp.StatusCode, string(body))
	}

	// Check if response is SSE (streaming) or JSON (non-streaming fallback)
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "text/event-stream") {
		text, err := o.readSSE(resp.Body)
		if err != nil {
			return nil, err
		}
		return ParseManifestResponse(text)
	}

	// Fallback: non-streaming JSON response
	return o.readNonStreaming(resp.Body)
}

// readSSE reads Server-Sent Events and accumulates the content.
func (o *OpenAIProvider) readSSE(body io.Reader) (string, error) {
	scanner := bufio.NewScanner(body)
	var accumulated strings.Builder
	chunkCount := 0

	for scanner.Scan() {
		line := scanner.Text()

		// SSE lines start with "data: "
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		// End of stream
		if data == "[DONE]" {
			break
		}

		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			log.Printf("[openai] failed to parse SSE chunk: %v (data: %s)", err, data)
			continue
		}

		if chunk.Error != nil {
			return "", fmt.Errorf("openai stream error: %s", chunk.Error.Message)
		}

		if len(chunk.Choices) > 0 {
			delta := chunk.Choices[0].Delta.Content
			if delta != "" {
				accumulated.WriteString(delta)
				chunkCount++

				// Notify listener of progress
				if o.OnChunk != nil {
					o.OnChunk(accumulated.String())
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read SSE stream: %w", err)
	}

	text := accumulated.String()
	log.Printf("[openai] streaming complete: %d chunks, %d chars", chunkCount, len(text))

	if text == "" {
		return "", fmt.Errorf("openai stream returned empty content")
	}

	return text, nil
}

// readNonStreaming handles a regular JSON response (fallback when server doesn't stream).
func (o *OpenAIProvider) readNonStreaming(body io.Reader) (*manifest.Manifest, error) {
	respBody, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	log.Printf("[openai] non-streaming response: %d bytes", len(respBody))

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
		text = oaiResp.Choices[0].Message.ReasoningContent
	}
	if text == "" {
		var rawMap map[string]any
		json.Unmarshal(respBody, &rawMap)
		text = findContentInRaw(rawMap)
	}
	if text == "" {
		preview := string(respBody)
		if len(preview) > 300 {
			preview = preview[:300] + "..."
		}
		return nil, fmt.Errorf("openai returned empty content. Raw: %s", preview)
	}

	return ParseManifestResponse(text)
}

// findContentInRaw walks a raw JSON map looking for any string content field.
func findContentInRaw(raw map[string]any) string {
	if choices, ok := raw["choices"].([]any); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]any); ok {
			if msg, ok := choice["message"].(map[string]any); ok {
				for _, key := range []string{"content", "reasoning_content", "text"} {
					if v, ok := msg[key].(string); ok && v != "" {
						return v
					}
				}
			}
			if v, ok := choice["text"].(string); ok && v != "" {
				return v
			}
		}
	}
	if v, ok := raw["output"].(string); ok && v != "" {
		return v
	}
	return ""
}
