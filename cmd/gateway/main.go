package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/adm/pkg/auth"
	"github.com/adm/pkg/autoupdate"
	"github.com/adm/pkg/ollama"
	"github.com/adm/pkg/policy"
	"github.com/adm/pkg/semantic"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/zap"
)

type Gateway struct {
	echo           *echo.Echo
	ollamaClient   *ollama.Client
	registry       *ollama.Registry
	adapter        *ollama.SchemaAdapter
	analyzer       *semantic.Analyzer
	policyEngine   *policy.Engine
	tokenManager   *auth.Manager
	siemClient     *SIEMClient
	autoUpdate     *autoupdate.Client
	logger         *zap.Logger
	sessions       map[string]*Session
}

type Session struct {
	ID           string
	AgentRole    string
	Token        *auth.Token
	Conversation *ollama.Conversation
	CreatedAt    time.Time
}

type ChatRequest struct {
	Model      string                   `json:"model"`
	Messages   []ollama.ChatMessage     `json:"messages"`
	Tools      []ollama.OpenAITool      `json:"tools,omitempty"`
	Stream     bool                     `json:"stream"`
	Temperature float64                 `json:"temperature,omitempty"`
	MaxTokens  int                      `json:"max_tokens,omitempty"`
}

type ChatResponse struct {
	ID      string                   `json:"id"`
	Model   string                   `json:"model"`
	Choices []Choice                 `json:"choices"`
	Usage   Usage                    `json:"usage"`
}

type Choice struct {
	Index        int                   `json:"index"`
	Message      ollama.ChatMessage    `json:"message"`
	FinishReason string                `json:"finish_reason"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type SIEMClient struct {
	baseURL string
}

func NewSIEMClient() *SIEMClient {
	url := os.Getenv("ADM_SIEM_URL")
	if url == "" {
		url = "http://localhost:9091"
	}
	return &SIEMClient{baseURL: url}
}

func (s *SIEMClient) IngestEvent(event interface{}) error {
	data, _ := json.Marshal(event)
	resp, err := http.Post(s.baseURL+"/api/v1/events", "application/json", 
		io.NopCloser(nil))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_ = data
	return nil
}

func NewGateway() (*Gateway, error) {
	logger, _ := zap.NewProduction()

	ollamaClient := ollama.NewClient(
		ollama.WithBaseURL(os.Getenv("ADM_OLLAMA_URL")),
	)
	registry := ollama.NewRegistry()
	adapter := ollama.NewSchemaAdapter()
	analyzer := semantic.NewAnalyzer(0.7)
	policyEngine := policy.NewEngine()
	tokenManager := auth.NewManager(5 * time.Minute)

	repoOwner := os.Getenv("ADM_GITHUB_OWNER")
	if repoOwner == "" {
		repoOwner = "Jest-Test-Team"
	}
	repoName := os.Getenv("ADM_GITHUB_REPO")
	if repoName == "" {
		repoName = "Agentic-Defense-Matrix-ADM-"
	}
	autoUpdate := autoupdate.NewClient(repoOwner, repoName, logger)

	gw := &Gateway{
		echo:         echo.New(),
		ollamaClient: ollamaClient,
		registry:     registry,
		adapter:      adapter,
		analyzer:     analyzer,
		policyEngine: policyEngine,
		tokenManager: tokenManager,
		siemClient:   NewSIEMClient(),
		autoUpdate:   autoUpdate,
		logger:       logger,
		sessions:     make(map[string]*Session),
	}

	gw.setupRoutes()
	gw.setupMiddleware()

	return gw, nil
}

func (gw *Gateway) setupMiddleware() {
	gw.echo.Use(middleware.Logger())
	gw.echo.Use(middleware.Recover())
	gw.echo.Use(middleware.CORS())
	gw.echo.Use(middleware.RequestID())
	gw.echo.Use(middleware.TimeoutWithConfig(middleware.TimeoutConfig{
		Timeout: 60 * time.Second,
	}))
}

func (gw *Gateway) setupRoutes() {
	gw.echo.GET("/v1/health", gw.healthHandler)
	gw.echo.GET("/v1/ready", gw.readyHandler)
	gw.echo.GET("/v1/version", gw.versionHandler)
	gw.echo.POST("/v1/chat/completions", gw.chatCompletion)
	gw.echo.POST("/v1/tools/execute", gw.executeTool)
	gw.echo.POST("/v1/admin/revoke/:session_id", gw.revokeSession)
	gw.echo.GET("/v1/admin/sessions", gw.listSessions)
	gw.echo.GET("/v1/admin/metrics", gw.getMetrics)
	gw.echo.POST("/v1/admin/update/check", gw.checkUpdateHandler)
}

func (gw *Gateway) Start(addr string) error {
	gw.logger.Info("Starting Gateway", zap.String("addr", addr))

	// Start auto-update background check
	gw.autoUpdate.StartBackgroundCheck()

	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		gw.logger.Info("Shutting down Gateway...")
		gw.echo.Close()
	}()

	return gw.echo.Start(addr)
}

func (gw *Gateway) healthHandler(c echo.Context) error {
	ollamaOK := gw.ollamaClient.HealthCheck(c.Request().Context()) == nil
	return c.JSON(http.StatusOK, map[string]interface{}{
		"healthy": ollamaOK,
		"version": gw.autoUpdate.CurrentVersion(),
		"model":   gw.registry.Default().Name,
	})
}

func (gw *Gateway) versionHandler(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]interface{}{
		"version": gw.autoUpdate.CurrentVersion(),
		"update_available": false,
	})
}

func (gw *Gateway) checkUpdateHandler(c echo.Context) error {
	latest, err := gw.autoUpdate.GetLatestVersion()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
	}

	updateAvailable := latest.Version != gw.autoUpdate.CurrentVersion()
	return c.JSON(http.StatusOK, map[string]interface{}{
		"current_version": gw.autoUpdate.CurrentVersion(),
		"latest_version":  latest.Version,
		"update_available": updateAvailable,
		"changelog":       latest.Changelog,
	})
}

func (gw *Gateway) readyHandler(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ready"})
}

func (gw *Gateway) chatCompletion(c echo.Context) error {
	var req ChatRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	sessionID := c.Request().Header.Get("X-Session-ID")
	if sessionID == "" {
		sessionID = fmt.Sprintf("session-%d", time.Now().UnixNano())
	}

	// Get last user message
	if len(req.Messages) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "messages required"})
	}
	lastMsg := req.Messages[len(req.Messages)-1]

	// Semantic analysis
	result := gw.analyzer.Analyze(lastMsg.Content)
	if result.IsMalicious {
		gw.logger.Warn("Malicious prompt detected",
			zap.String("session", sessionID),
			zap.Float64("score", result.Score),
		)
		return c.JSON(http.StatusForbidden, map[string]interface{}{
			"error": "request blocked",
			"reason": result.Reasons,
		})
	}

	// Rate limit check
	if result.IsSuspicious {
		gw.logger.Info("Suspicious prompt detected",
			zap.String("session", sessionID),
			zap.Float64("score", result.Score),
		)
	}

	// Get or create session
	session, exists := gw.sessions[sessionID]
	if !exists {
		session = &Session{
			ID:           sessionID,
			AgentRole:    "planner",
			Conversation: ollama.NewConversation(),
			CreatedAt:    time.Now(),
		}
		gw.sessions[sessionID] = session
	}

	// Add user message to conversation
	session.Conversation.AddUserMessage(lastMsg.Content)

	// Determine model
	model := req.Model
	if model == "" {
		model = gw.registry.Default().Name
	}

	// Build Ollama request
	ollamaReq := ollama.ChatRequest{
		Model:    model,
		Messages: session.Conversation.Messages(),
		Stream:   false,
	}

	if len(req.Tools) > 0 {
		ollamaReq.Tools = gw.adapter.ToOllamaTools(req.Tools)
	}

	// Call Ollama
	resp, err := gw.ollamaClient.Chat(c.Request().Context(), ollamaReq)
	if err != nil {
		gw.logger.Error("Ollama error", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	// Add assistant response to conversation
	session.Conversation.AddAssistantMessage(resp.Message.Content)

	// Build response
	chatResp := ChatResponse{
		ID:    fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano()),
		Model: resp.Model,
		Choices: []Choice{
			{
				Index:        0,
				Message:      resp.Message,
				FinishReason: "stop",
			},
		},
		Usage: Usage{
			PromptTokens:     resp.PromptEvalCount,
			CompletionTokens: resp.EvalCount,
			TotalTokens:      resp.PromptEvalCount + resp.EvalCount,
		},
	}

	return c.JSON(http.StatusOK, chatResp)
}

func (gw *Gateway) executeTool(c echo.Context) error {
	var req struct {
		SessionID string `json:"session_id"`
		ToolName  string `json:"tool_name"`
		Arguments string `json:"arguments"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	// Policy check
	evalReq := &policy.EvaluationRequest{
		PolicyID:  "agent_permissions",
		AgentRole: "executor",
		ToolName:  req.ToolName,
		SessionID: req.SessionID,
	}

	result, err := gw.policyEngine.Evaluate(c.Request().Context(), evalReq)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	if !result.Allowed {
		return c.JSON(http.StatusForbidden, map[string]interface{}{
			"error":  "tool not allowed",
			"reasons": result.Reasons,
		})
	}

	// In production: forward to Executor agent via gRPC
	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"tool":    req.ToolName,
	})
}

func (gw *Gateway) revokeSession(c echo.Context) error {
	sessionID := c.Param("session_id")
	if err := gw.tokenManager.Revoke(sessionID); err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
	}

	delete(gw.sessions, sessionID)

	gw.logger.Info("Session revoked", zap.String("session_id", sessionID))
	return c.JSON(http.StatusOK, map[string]string{"status": "revoked"})
}

func (gw *Gateway) listSessions(c echo.Context) error {
	sessions := make([]map[string]interface{}, 0)
	for _, s := range gw.sessions {
		sessions = append(sessions, map[string]interface{}{
			"id":         s.ID,
			"role":       s.AgentRole,
			"created_at": s.CreatedAt,
			"messages":   s.Conversation.Length(),
		})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{
		"sessions": sessions,
		"count":    len(sessions),
	})
}

func (gw *Gateway) getMetrics(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]interface{}{
		"active_sessions":  len(gw.sessions),
		"semantic_stats":   gw.analyzer.GetStats(),
		"token_count":      gw.tokenManager.ActiveCount(),
	})
}

func main() {
	gateway, err := NewGateway()
	if err != nil {
		panic(err)
	}

	port := os.Getenv("ADM_PORT")
	if port == "" {
		port = "8080"
	}

	if err := gateway.Start(":" + port); err != nil {
		gateway.logger.Fatal("gateway start failed", zap.Error(err))
	}
}
