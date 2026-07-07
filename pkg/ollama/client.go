package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	defaultBaseURL    = "http://localhost:11434"
	defaultTimeout    = 30 * time.Second
	defaultMaxRetries = 3
)

// Client is a thin HTTP wrapper for the Ollama API.
type Client struct {
	baseURL    string
	httpClient *http.Client
	maxRetries int
}

// Option configures the client.
type Option func(*Client)

// WithBaseURL sets the Ollama server URL.
func WithBaseURL(url string) Option {
	return func(c *Client) { c.baseURL = url }
}

// WithTimeout sets the HTTP client timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) { c.httpClient.Timeout = d }
}

// WithMaxRetries sets retry count for transient errors.
func WithMaxRetries(n int) Option {
	return func(c *Client) { c.maxRetries = n }
}

// NewClient creates a new Ollama client.
func NewClient(opts ...Option) *Client {
	c := &Client{
		baseURL:    defaultBaseURL,
		httpClient: &http.Client{Timeout: defaultTimeout},
		maxRetries: defaultMaxRetries,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// ChatRequest represents the Ollama /api/chat request body.
type ChatRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
	Options  *ModelOptions `json:"options,omitempty"`
	Tools    []Tool        `json:"tools,omitempty"`
}

// ChatMessage represents a message in the chat.
type ChatMessage struct {
	Role      string     `json:"role"`
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// ToolCall represents an LLM-initiated tool call.
type ToolCall struct {
	Function FunctionCall `json:"function"`
}

// FunctionCall is the function name + args.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Tool is an OpenAI-compatible tool definition.
type Tool struct {
	Type     string             `json:"type"`
	Function FunctionDefinition `json:"function"`
}

// FunctionDefinition describes a tool the LLM can call.
type FunctionDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// ModelOptions are inference parameters.
type ModelOptions struct {
	Temperature float64 `json:"temperature,omitempty"`
	TopP        float64 `json:"top_p,omitempty"`
	NumPredict  int     `json:"num_predict,omitempty"`
	NumCtx      int     `json:"num_ctx,omitempty"`
}

// ChatResponse is the Ollama /api/chat response.
type ChatResponse struct {
	Model              string        `json:"model"`
	Message            ChatMessage   `json:"message"`
	Done               bool          `json:"done"`
	TotalDuration      int64         `json:"total_duration"`
	LoadDuration       int64         `json:"load_duration"`
	PromptEvalCount    int           `json:"prompt_eval_count"`
	EvalCount          int           `json:"eval_count"`
	EvalDuration       int64         `json:"eval_duration"`
}

// Chat sends a chat request and returns the full response.
func (c *Client) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	req.Stream = false

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	var lastErr error
	for i := 0; i <= c.maxRetries; i++ {
		resp, err := c.doRequest(ctx, "/api/chat", body)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(i+1) * 500 * time.Millisecond)
			continue
		}

		var chatResp ChatResponse
		if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("decode response: %w", err)
		}
		resp.Body.Close()
		return &chatResp, nil
	}

	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// ChatStream sends a chat request and returns a streaming reader.
// The caller must close the returned ReadCloser when done.
func (c *Client) ChatStream(ctx context.Context, req ChatRequest) (io.ReadCloser, error) {
	req.Stream = true

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	resp, err := c.doRequest(ctx, "/api/chat", body)
	if err != nil {
		return nil, err
	}

	return resp.Body, nil
}

// Generate sends a simple generation request (no tool calling).
func (c *Client) Generate(ctx context.Context, model, prompt string) (string, error) {
	payload := map[string]interface{}{
		"model":  model,
		"prompt": prompt,
		"stream": false,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	resp, err := c.doRequest(ctx, "/api/generate", body)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	return result.Response, nil
}

// ListModels returns all available models.
func (c *Client) ListModels(ctx context.Context) ([]ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/tags", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Models []ModelInfo `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode models: %w", err)
	}

	return result.Models, nil
}

// ModelInfo contains metadata about an Ollama model.
type ModelInfo struct {
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	Digest     string `json:"digest"`
	ModifiedAt string `json:"modified_at"`
}

// IsModelAvailable checks if a specific model is available.
func (c *Client) IsModelAvailable(ctx context.Context, modelName string) (bool, error) {
	models, err := c.ListModels(ctx)
	if err != nil {
		return false, err
	}

	for _, m := range models {
		if m.Name == modelName || m.Name == modelName+":latest" {
			return true, nil
		}
	}
	return false, nil
}

// HealthCheck verifies the Ollama server is reachable.
func (c *Client) HealthCheck(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/tags", nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) doRequest(ctx context.Context, path string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request %s: %w", path, err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("ollama %s returned status %d", path, resp.StatusCode)
	}

	return resp, nil
}
