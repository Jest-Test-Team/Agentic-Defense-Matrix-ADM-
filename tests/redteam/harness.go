package redteam

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type TestHarness struct {
	GatewayURL string
	HTTPClient *http.Client
}

func NewTestHarness() *TestHarness {
	return &TestHarness{
		GatewayURL: "http://localhost:8080",
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}
}

type ChatRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Tools    []interface{} `json:"tools,omitempty"`
	Stream   bool          `json:"stream"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatResponse struct {
	ID      string   `json:"id"`
	Choices []Choice `json:"choices"`
}

type Choice struct {
	Message ChatMessage `json:"message"`
}

func (h *TestHarness) SendPrompt(t *testing.T, prompt string) (*ChatResponse, int, error) {
	req := ChatRequest{
		Model: "llama3.1:8b",
		Messages: []ChatMessage{
			{Role: "user", Content: prompt},
		},
		Stream: false,
	}

	body, _ := json.Marshal(req)
	resp, err := h.HTTPClient.Post(
		h.GatewayURL+"/v1/chat/completions",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		if strings.Contains(err.Error(), "connection refused") ||
			strings.Contains(err.Error(), "operation not permitted") {
			t.Skipf("gateway is not running at %s: %v", h.GatewayURL, err)
		}
		return nil, 0, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var chatResp ChatResponse
	json.Unmarshal(respBody, &chatResp)

	return &chatResp, resp.StatusCode, nil
}
