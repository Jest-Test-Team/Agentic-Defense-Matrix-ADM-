package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/adm/pkg/ollama"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

type SummarizerAgent struct {
	echo         *echo.Echo
	ollamaClient *ollama.Client
	registry     *ollama.Registry
	logger       *zap.Logger
}

type SummaryRequest struct {
	SessionID     string                `json:"session_id"`
	Conversation  []ollama.ChatMessage  `json:"conversation"`
	ToolResults   []ToolResult          `json:"tool_results"`
	MaxLength     int                   `json:"max_length"`
}

type ToolResult struct {
	ExecutionID string `json:"execution_id"`
	ToolName    string `json:"tool_name"`
	Result      string `json:"result"`
	Success     bool   `json:"success"`
}

type SummaryResponse struct {
	Summary      string   `json:"summary"`
	KeyDecisions []string `json:"key_decisions"`
	ActionItems  []string `json:"action_items"`
}

func NewSummarizerAgent() (*SummarizerAgent, error) {
	logger, _ := zap.NewProduction()

	ollamaClient := ollama.NewClient(
		ollama.WithBaseURL(os.Getenv("ADM_OLLAMA_URL")),
	)

	agent := &SummarizerAgent{
		echo:         echo.New(),
		ollamaClient: ollamaClient,
		registry:     ollama.NewRegistry(),
		logger:       logger,
	}

	agent.setupRoutes()
	return agent, nil
}

func (a *SummarizerAgent) setupRoutes() {
	a.echo.GET("/health", a.healthHandler)
	a.echo.POST("/summarize", a.summarizeHandler)
}

func (a *SummarizerAgent) healthHandler(c echo.Context) error {
	return c.JSON(200, map[string]interface{}{
		"healthy": true,
		"role":    "summarizer",
	})
}

func (a *SummarizerAgent) summarizeHandler(c echo.Context) error {
	var req SummaryRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(400, map[string]string{"error": err.Error()})
	}

	// Build summarization prompt
	systemPrompt := `You are a conversation summarizer. Given a conversation history and tool results,
generate a concise summary with key decisions and action items.

Output format:
{
  "summary": "Brief summary of the conversation",
  "key_decisions": ["decision 1", "decision 2"],
  "action_items": ["action 1", "action 2"]
}`

	// Format conversation for LLM
	convStr := a.formatConversation(req.Conversation, req.ToolResults)

	messages := []ollama.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: fmt.Sprintf("Summarize this conversation:\n\n%s", convStr)},
	}

	// Call LLM
	model := a.registry.Default().Name
	resp, err := a.ollamaClient.Chat(c.Request().Context(), ollama.ChatRequest{
		Model:    model,
		Messages: messages,
		Stream:   false,
	})
	if err != nil {
		return c.JSON(500, map[string]string{"error": err.Error()})
	}

	// Parse response
	var summary SummaryResponse
	if err := json.Unmarshal([]byte(resp.Message.Content), &summary); err != nil {
		// Fallback: use raw response as summary
		summary = SummaryResponse{
			Summary:      resp.Message.Content,
			KeyDecisions: []string{},
			ActionItems:  []string{},
		}
	}

	// Truncate if needed
	if req.MaxLength > 0 && len(summary.Summary) > req.MaxLength {
		summary.Summary = summary.Summary[:req.MaxLength] + "..."
	}

	return c.JSON(200, summary)
}

func (a *SummarizerAgent) formatConversation(conv []ollama.ChatMessage, results []ToolResult) string {
	result := ""
	for _, msg := range conv {
		result += fmt.Sprintf("[%s] %s\n", msg.Role, msg.Content)
	}

	if len(results) > 0 {
		result += "\nTool Results:\n"
		for _, r := range results {
			status := "success"
			if !r.Success {
				status = "failed"
			}
			result += fmt.Sprintf("- %s (%s): %s\n", r.ToolName, status, r.Result)
		}
	}

	return result
}

func (a *SummarizerAgent) Start(addr string) error {
	a.logger.Info("Starting Summarizer agent", zap.String("addr", addr))

	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		a.echo.Close()
	}()

	return a.echo.Start(addr)
}

func main() {
	agent, err := NewSummarizerAgent()
	if err != nil {
		panic(err)
	}

	port := os.Getenv("ADM_GRPC_PORT")
	if port == "" {
		port = "9083"
	}

	if err := agent.Start(":" + port); err != nil {
		agent.logger.Fatal("summarizer start failed", zap.Error(err))
	}
}
