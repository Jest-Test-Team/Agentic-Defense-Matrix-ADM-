package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	defaultBaseURL    = "http://localhost:11434"
	defaultTimeout    = 30 * time.Second
	defaultMaxRetries = 3
)

// Client is a thin HTTP wrapper for the LLM backend. It speaks Ollama's native
// /api/chat protocol by default, or an OpenAI-compatible API (e.g. Groq) when
// openAI is set — in which case requests carry a Bearer token and hit
// /chat/completions.
type Client struct {
	baseURL    string
	httpClient *http.Client
	maxRetries int
	apiKey     string
	openAI     bool
	// model, when set, overrides the model name in every request — used so a
	// fallback provider can substitute its own model id (e.g. an X.AI grok
	// model in place of a Groq one).
	model string
	// fallback, when non-nil, is tried if this client's request fails after all
	// retries — e.g. Groq is rate-limited or down and we fail over to X.AI.
	fallback *Client
}

// Option configures the client.
type Option func(*Client)

// WithBaseURL sets the LLM server URL.
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

// WithAPIKey sets a bearer token sent on every request (for hosted APIs).
func WithAPIKey(key string) Option {
	return func(c *Client) { c.apiKey = key }
}

// WithOpenAICompat switches the client to the OpenAI-compatible API shape.
func WithOpenAICompat(v bool) Option {
	return func(c *Client) { c.openAI = v }
}

// WithModel pins the model id used for every request, overriding req.Model.
func WithModel(m string) Option {
	return func(c *Client) { c.model = m }
}

// WithFallback sets a secondary client tried when the primary fails.
func WithFallback(fb *Client) Option {
	return func(c *Client) { c.fallback = fb }
}

// NewClient creates a new LLM client.
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

// NewClientFromEnv builds a client from environment configuration, the single
// construction point used by the gateway and agents:
//
//	ADM_LLM_MODE     "ollama" (default) or "openai"
//	ADM_LLM_BASE_URL base URL; falls back to ADM_OLLAMA_URL, then the Ollama default
//	ADM_LLM_API_KEY  bearer token for openai mode (e.g. a Groq key)
//	ADM_MODEL        model id for the primary provider
//
// When ADM_LLM_FALLBACK_API_KEY (or _BASE_URL) is set, a second OpenAI-compatible
// provider is attached as a failover, tried automatically when the primary fails
// (e.g. Groq is rate-limited/down → fail over to X.AI):
//
//	ADM_LLM_FALLBACK_BASE_URL  e.g. https://api.x.ai/v1
//	ADM_LLM_FALLBACK_API_KEY   e.g. an xai-... key
//	ADM_LLM_FALLBACK_MODEL     e.g. grok-2-latest
func NewClientFromEnv() *Client {
	mode := strings.ToLower(os.Getenv("ADM_LLM_MODE"))
	baseURL := os.Getenv("ADM_LLM_BASE_URL")
	if baseURL == "" {
		baseURL = os.Getenv("ADM_OLLAMA_URL")
	}
	opts := []Option{}
	if baseURL != "" {
		opts = append(opts, WithBaseURL(baseURL))
	}
	if mode == "openai" {
		opts = append(opts, WithOpenAICompat(true), WithAPIKey(os.Getenv("ADM_LLM_API_KEY")))
		if m := os.Getenv("ADM_MODEL"); m != "" {
			opts = append(opts, WithModel(m))
		}
		if fb := fallbackFromEnv(); fb != nil {
			opts = append(opts, WithFallback(fb))
		}
	}
	return NewClient(opts...)
}

// fallbackFromEnv builds the OpenAI-compatible failover client, or nil if no
// fallback key/URL is configured.
func fallbackFromEnv() *Client {
	key := os.Getenv("ADM_LLM_FALLBACK_API_KEY")
	url := os.Getenv("ADM_LLM_FALLBACK_BASE_URL")
	if key == "" && url == "" {
		return nil
	}
	opts := []Option{WithOpenAICompat(true), WithAPIKey(key)}
	if url != "" {
		opts = append(opts, WithBaseURL(url))
	}
	if m := os.Getenv("ADM_LLM_FALLBACK_MODEL"); m != "" {
		opts = append(opts, WithModel(m))
	}
	return NewClient(opts...)
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
	Model           string      `json:"model"`
	Message         ChatMessage `json:"message"`
	Done            bool        `json:"done"`
	TotalDuration   int64       `json:"total_duration"`
	LoadDuration    int64       `json:"load_duration"`
	PromptEvalCount int         `json:"prompt_eval_count"`
	EvalCount       int         `json:"eval_count"`
	EvalDuration    int64       `json:"eval_duration"`
}

// Chat sends a chat request and returns the full response. If this client has a
// fallback and the primary attempt fails after all retries, the request is
// transparently retried against the fallback provider.
func (c *Client) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	req.Stream = false
	if c.model != "" {
		req.Model = c.model
	}

	resp, err := c.chatPrimary(ctx, req)
	if err != nil && c.fallback != nil {
		return c.fallback.Chat(ctx, req)
	}
	return resp, err
}

// chatPrimary runs the request against this client only (no fallback).
func (c *Client) chatPrimary(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	if c.openAI {
		return c.chatOpenAI(ctx, req)
	}

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

// openAIChatRequest is the OpenAI /chat/completions request body. Inference
// params live at the top level (unlike Ollama's nested "options").
type openAIChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Stream      bool          `json:"stream"`
	Tools       []Tool        `json:"tools,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
	TopP        float64       `json:"top_p,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

// openAIChatResponse is the OpenAI /chat/completions response body.
type openAIChatResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Role      string           `json:"role"`
			Content   string           `json:"content"`
			ToolCalls []OpenAIToolCall `json:"tool_calls,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

// chatOpenAI performs a non-streaming chat against an OpenAI-compatible API and
// maps the response back into the shared *ChatResponse so callers are unchanged.
func (c *Client) chatOpenAI(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	oreq := openAIChatRequest{
		Model:    req.Model,
		Messages: req.Messages,
		Stream:   false,
		Tools:    req.Tools,
	}
	if req.Options != nil {
		oreq.Temperature = req.Options.Temperature
		oreq.TopP = req.Options.TopP
		oreq.MaxTokens = req.Options.NumPredict
	}

	body, err := json.Marshal(oreq)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	var lastErr error
	for i := 0; i <= c.maxRetries; i++ {
		resp, err := c.doRequest(ctx, "/chat/completions", body)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(i+1) * 500 * time.Millisecond)
			continue
		}

		var oresp openAIChatResponse
		if err := json.NewDecoder(resp.Body).Decode(&oresp); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("decode response: %w", err)
		}
		resp.Body.Close()

		out := &ChatResponse{
			Model:           oresp.Model,
			Done:            true,
			PromptEvalCount: oresp.Usage.PromptTokens,
			EvalCount:       oresp.Usage.CompletionTokens,
		}
		if len(oresp.Choices) > 0 {
			m := oresp.Choices[0].Message
			out.Message = ChatMessage{Role: m.Role, Content: m.Content}
			for _, tc := range m.ToolCalls {
				out.Message.ToolCalls = append(out.Message.ToolCalls, ToolCall{
					Function: FunctionCall{Name: tc.Function.Name, Arguments: tc.Function.Arguments},
				})
			}
		}
		return out, nil
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

// HealthCheck verifies the LLM backend is reachable. Ollama exposes /api/tags;
// OpenAI-compatible backends (Groq) expose /models.
func (c *Client) HealthCheck(ctx context.Context) error {
	err := c.healthCheckPrimary(ctx)
	if err != nil && c.fallback != nil {
		return c.fallback.HealthCheck(ctx)
	}
	return err
}

func (c *Client) healthCheckPrimary(ctx context.Context) error {
	path := "/api/tags"
	if c.openAI {
		path = "/models"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("llm returned status %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) doRequest(ctx context.Context, path string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request %s: %w", path, err)
	}

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<12))
		resp.Body.Close()
		return nil, fmt.Errorf("llm %s returned status %d: %s", path, resp.StatusCode, strings.TrimSpace(string(msg)))
	}

	return resp, nil
}
