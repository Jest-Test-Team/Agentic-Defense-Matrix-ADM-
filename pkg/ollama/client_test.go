package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestChatOpenAI verifies the OpenAI-compatible path: requests hit
// /chat/completions with a Bearer token, and the OpenAI response shape is
// mapped back into the shared ChatResponse.
func TestChatOpenAI(t *testing.T) {
	var gotPath, gotAuth string
	var gotReq openAIChatRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotReq)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"model": "groq-model",
			"choices": []map[string]any{{
				"message":       map[string]any{"role": "assistant", "content": "pong"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 11, "completion_tokens": 3},
		})
	}))
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL), WithOpenAICompat(true), WithAPIKey("secret-key"))
	resp, err := c.Chat(context.Background(), ChatRequest{
		Model:    "groq-model",
		Messages: []ChatMessage{{Role: "user", Content: "ping"}},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	if gotPath != "/chat/completions" {
		t.Errorf("path = %q, want /chat/completions", gotPath)
	}
	if gotAuth != "Bearer secret-key" {
		t.Errorf("auth = %q, want Bearer secret-key", gotAuth)
	}
	if gotReq.Stream {
		t.Error("request should be non-streaming")
	}
	if resp.Message.Content != "pong" {
		t.Errorf("content = %q, want pong", resp.Message.Content)
	}
	if resp.PromptEvalCount != 11 || resp.EvalCount != 3 {
		t.Errorf("token counts = %d/%d, want 11/3", resp.PromptEvalCount, resp.EvalCount)
	}
	if !resp.Done {
		t.Error("Done should be true")
	}
}

// TestHealthCheckOpenAI verifies openai mode hits /models with auth.
func TestHealthCheckOpenAI(t *testing.T) {
	var gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL), WithOpenAICompat(true), WithAPIKey("k"))
	if err := c.HealthCheck(context.Background()); err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
	if gotPath != "/models" {
		t.Errorf("path = %q, want /models", gotPath)
	}
	if gotAuth != "Bearer k" {
		t.Errorf("auth = %q, want Bearer k", gotAuth)
	}
}

// TestChatOllamaNative verifies the default path still uses /api/chat with no auth.
func TestChatOllamaNative(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		json.NewEncoder(w).Encode(ChatResponse{Message: ChatMessage{Role: "assistant", Content: "hi"}, Done: true})
	}))
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL))
	resp, err := c.Chat(context.Background(), ChatRequest{Model: "llama3.1:8b", Messages: []ChatMessage{{Role: "user", Content: "x"}}})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if gotPath != "/api/chat" {
		t.Errorf("path = %q, want /api/chat", gotPath)
	}
	if resp.Message.Content != "hi" {
		t.Errorf("content = %q", resp.Message.Content)
	}
}

// TestChatFallback verifies that when the primary provider fails, the request
// fails over to the fallback provider, and the fallback substitutes its own
// model id via WithModel.
func TestChatFallback(t *testing.T) {
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer primary.Close()

	var fbModel string
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req openAIChatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		fbModel = req.Model
		json.NewEncoder(w).Encode(map[string]any{
			"model":   "grok-2-latest",
			"choices": []map[string]any{{"message": map[string]any{"role": "assistant", "content": "from-grok"}}},
		})
	}))
	defer fallback.Close()

	fb := NewClient(WithBaseURL(fallback.URL), WithOpenAICompat(true), WithAPIKey("xai-k"), WithModel("grok-2-latest"))
	c := NewClient(WithBaseURL(primary.URL), WithOpenAICompat(true), WithAPIKey("gsk_k"),
		WithModel("llama-3.1-8b-instant"), WithMaxRetries(0), WithFallback(fb))

	resp, err := c.Chat(context.Background(), ChatRequest{Messages: []ChatMessage{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Message.Content != "from-grok" {
		t.Errorf("content = %q, want from-grok (fallback response)", resp.Message.Content)
	}
	if fbModel != "grok-2-latest" {
		t.Errorf("fallback model = %q, want grok-2-latest", fbModel)
	}
}

// TestRegistryHonorsADMModel verifies ADM_MODEL becomes the default model.
func TestRegistryHonorsADMModel(t *testing.T) {
	t.Setenv("ADM_MODEL", "llama-3.1-8b-instant")
	r := NewRegistry()
	if d := r.Default(); d == nil || d.Name != "llama-3.1-8b-instant" {
		t.Fatalf("default = %v, want llama-3.1-8b-instant", d)
	}
}
