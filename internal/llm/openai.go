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

const openaiDefaultMaxTok = 65536

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

	log.Printf("[openai] response status 200, Content-Type: %s", resp.Header.Get("Content-Type"))

	// Read from the live stream line-by-line for real-time progress.
	// We sent stream:true, so the response should be SSE.
	// If it's not SSE, we detect it from the first line and fall back.
	text, err := o.readStream(resp.Body)
	if err != nil {
		return nil, err
	}

	return ParseManifestResponse(text)
}

// readStream reads the response body line-by-line for real-time progress.
// Handles both SSE format (data: {...}) and plain JSON fallback.
func (o *OpenAIProvider) readStream(body io.Reader) (string, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer

	var accumulated strings.Builder
	var fallbackBuf strings.Builder // captures all lines in case we need JSON fallback
	chunkCount := 0
	reasoningChunks := 0
	lineCount := 0
	isSSE := false

	for scanner.Scan() {
		line := scanner.Text()
		lineCount++
		fallbackBuf.WriteString(line + "\n")

		// Log first few lines
		if lineCount <= 3 {
			preview := line
			if len(preview) > 150 {
				preview = preview[:150] + "..."
			}
			log.Printf("[openai] line %d: %q", lineCount, preview)
		}

		// Detect format from first data line
		if lineCount == 1 && !strings.HasPrefix(line, "data:") && strings.HasPrefix(line, "{") {
			// First line is JSON, not SSE — read rest and parse as non-streaming
			log.Printf("[openai] detected non-streaming JSON response")
			rest, _ := io.ReadAll(body)
			fullBody := line + "\n" + string(rest)
			return o.parseJSONResponse([]byte(fullBody))
		}

		if line == "" {
			continue
		}

		// Extract SSE data
		var data string
		if strings.HasPrefix(line, "data: ") {
			data = strings.TrimPrefix(line, "data: ")
			isSSE = true
		} else if strings.HasPrefix(line, "data:") {
			data = strings.TrimPrefix(line, "data:")
			isSSE = true
		} else {
			continue
		}

		data = strings.TrimSpace(data)
		if data == "" || data == "[DONE]" {
			if data == "[DONE]" {
				log.Printf("[openai] stream [DONE]: %d reasoning, %d content chunks", reasoningChunks, chunkCount)
			}
			continue
		}

		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		if chunk.Error != nil {
			return "", fmt.Errorf("stream error: %s", chunk.Error.Message)
		}

		if len(chunk.Choices) > 0 {
			delta := chunk.Choices[0].Delta

			// reasoning_content = chain-of-thought (show progress, don't include in output)
			if delta.ReasoningContent != "" {
				reasoningChunks++
				if o.OnChunk != nil {
					o.OnChunk(fmt.Sprintf("Thinking... (%d tokens)", reasoningChunks))
				}
			}

			// content = actual output
			if delta.Content != "" {
				accumulated.WriteString(delta.Content)
				chunkCount++
				if o.OnChunk != nil {
					o.OnChunk(fmt.Sprintf("Generating... (%d chars)", accumulated.Len()))
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read stream: %w", err)
	}

	text := accumulated.String()
	log.Printf("[openai] stream done: %d lines, %d reasoning, %d content, %d chars",
		lineCount, reasoningChunks, chunkCount, len(text))

	// If we got content from SSE, return it
	if text != "" {
		return text, nil
	}

	// If SSE had reasoning but no content, the model may have hit token limit
	if isSSE && reasoningChunks > 0 && chunkCount == 0 {
		return "", fmt.Errorf("model spent all tokens on reasoning (%d chunks) without generating output — try a simpler prompt or increase max_tokens", reasoningChunks)
	}

	// Fallback: try parsing the entire captured body as JSON
	if !isSSE {
		return o.parseJSONResponse([]byte(fallbackBuf.String()))
	}

	return "", fmt.Errorf("stream returned empty content")
}

// parseJSONResponse handles a non-streaming JSON response.
func (o *OpenAIProvider) parseJSONResponse(respBody []byte) (string, error) {
	log.Printf("[openai] parsing as JSON: %d bytes", len(respBody))

	var oaiResp openaiResponse
	if err := json.Unmarshal(respBody, &oaiResp); err != nil {
		return "", fmt.Errorf("parse JSON response: %w", err)
	}

	if oaiResp.Error != nil {
		return "", fmt.Errorf("API error: %s", oaiResp.Error.Message)
	}

	if len(oaiResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	text := oaiResp.Choices[0].Message.Content
	if text == "" {
		text = oaiResp.Choices[0].Message.ReasoningContent
	}
	if text == "" {
		return "", fmt.Errorf("empty content in response")
	}

	return text, nil
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
