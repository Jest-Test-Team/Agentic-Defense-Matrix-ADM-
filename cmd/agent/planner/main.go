package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/adm/pkg/ollama"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

type PlannerAgent struct {
	echo         *echo.Echo
	ollamaClient *ollama.Client
	registry     *ollama.Registry
	adapter      *ollama.SchemaAdapter
	logger       *zap.Logger
}

type PlanRequest struct {
	SessionID      string                `json:"session_id"`
	UserPrompt     string                `json:"user_prompt"`
	Context        []ollama.ChatMessage  `json:"context"`
	AvailableTools []ollama.OpenAITool   `json:"available_tools"`
	Model          string                `json:"model"`
}

type PlanResponse struct {
	PlanID     string         `json:"plan_id"`
	Steps      []PlannedStep  `json:"steps"`
	Reasoning  string         `json:"reasoning"`
	Confidence float64        `json:"confidence"`
}

type PlannedStep struct {
	StepNumber int    `json:"step_number"`
	ToolName   string `json:"tool_name"`
	Arguments  string `json:"arguments"`
	DependsOn  string `json:"depends_on,omitempty"`
	Rationale  string `json:"rationale"`
}

func NewPlannerAgent() (*PlannerAgent, error) {
	logger, _ := zap.NewProduction()

	ollamaClient := ollama.NewClient(
		ollama.WithBaseURL(os.Getenv("ADM_OLLAMA_URL")),
	)

	agent := &PlannerAgent{
		echo:         echo.New(),
		ollamaClient: ollamaClient,
		registry:     ollama.NewRegistry(),
		adapter:      ollama.NewSchemaAdapter(),
		logger:       logger,
	}

	agent.setupRoutes()
	return agent, nil
}

func (a *PlannerAgent) setupRoutes() {
	a.echo.GET("/health", a.healthHandler)
	a.echo.POST("/plan", a.planHandler)
}

func (a *PlannerAgent) healthHandler(c echo.Context) error {
	return c.JSON(200, map[string]interface{}{
		"healthy": true,
		"role":    "planner",
	})
}

func (a *PlannerAgent) planHandler(c echo.Context) error {
	var req PlanRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(400, map[string]string{"error": err.Error()})
	}

	// Build planning prompt
	systemPrompt := `You are a task planning agent. Given a user request and available tools, 
decompose the task into ordered steps. Each step should use exactly one tool.

Output your plan as a JSON array of steps with the following format:
[{"step_number": 1, "tool_name": "tool_name", "arguments": "{...}", "rationale": "why"}]

Be concise and only use necessary tools.`

	messages := []ollama.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: fmt.Sprintf("User request: %s\n\nAvailable tools: %s", 
			req.UserPrompt, a.formatTools(req.AvailableTools))},
	}

	// Call LLM
	model := req.Model
	if model == "" {
		model = a.registry.Default().Name
	}

	resp, err := a.ollamaClient.Chat(c.Request().Context(), ollama.ChatRequest{
		Model:    model,
		Messages: messages,
		Stream:   false,
	})
	if err != nil {
		return c.JSON(500, map[string]string{"error": err.Error()})
	}

	// Parse steps from LLM response
	var steps []PlannedStep
	if err := json.Unmarshal([]byte(resp.Message.Content), &steps); err != nil {
		// Fallback: single step with the whole response
		steps = []PlannedStep{
			{
				StepNumber: 1,
				ToolName:   "run_command",
				Arguments:  resp.Message.Content,
				Rationale:  "LLM response parsed as command",
			},
		}
	}

	return c.JSON(200, PlanResponse{
		PlanID:     fmt.Sprintf("plan-%d", time.Now().UnixNano()),
		Steps:      steps,
		Reasoning:  "Task decomposed based on available tools",
		Confidence: 0.85,
	})
}

func (a *PlannerAgent) formatTools(tools []ollama.OpenAITool) string {
	data, _ := json.MarshalIndent(tools, "", "  ")
	return string(data)
}

func (a *PlannerAgent) Start(addr string) error {
	a.logger.Info("Starting Planner agent", zap.String("addr", addr))

	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		a.echo.Close()
	}()

	return a.echo.Start(addr)
}

func main() {
	agent, err := NewPlannerAgent()
	if err != nil {
		panic(err)
	}

	port := os.Getenv("ADM_GRPC_PORT")
	if port == "" {
		port = "9081"
	}

	if err := agent.Start(":" + port); err != nil {
		agent.logger.Fatal("planner start failed", zap.Error(err))
	}
}
