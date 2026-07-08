package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

type ExecutorAgent struct {
	echo         *echo.Echo
	dockerClient *client.Client
	logger       *zap.Logger
	sessions     map[string]*ContainerInfo
}

type ContainerInfo struct {
	ContainerID string
	SessionID   string
	CreatedAt   time.Time
}

type ExecuteRequest struct {
	SessionID      string            `json:"session_id"`
	PlanID         string            `json:"plan_id"`
	ToolName       string            `json:"tool_name"`
	Arguments      string            `json:"arguments"`
	Env            map[string]string `json:"env"`
	TimeoutSeconds int               `json:"timeout_seconds"`
}

type ExecuteResponse struct {
	ExecutionID     string   `json:"execution_id"`
	Success         bool     `json:"success"`
	Result          string   `json:"result"`
	Error           string   `json:"error,omitempty"`
	ExecutionTimeMs int64    `json:"execution_time_ms"`
}

func NewExecutorAgent() (*ExecutorAgent, error) {
	logger, _ := zap.NewProduction()

	dockerClient, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		logger.Warn("Docker not available, running in limited mode", zap.Error(err))
		dockerClient = nil
	}

	agent := &ExecutorAgent{
		echo:         echo.New(),
		dockerClient: dockerClient,
		logger:       logger,
		sessions:     make(map[string]*ContainerInfo),
	}

	agent.setupRoutes()
	return agent, nil
}

func (a *ExecutorAgent) setupRoutes() {
	a.echo.GET("/health", a.healthHandler)
	a.echo.POST("/execute", a.executeHandler)
	a.echo.POST("/kill", a.killHandler)
}

func (a *ExecutorAgent) healthHandler(c echo.Context) error {
	dockerOK := a.dockerClient != nil
	return c.JSON(200, map[string]interface{}{
		"healthy": true,
		"role":    "executor",
		"docker":  dockerOK,
	})
}

func (a *ExecutorAgent) executeHandler(c echo.Context) error {
	var req ExecuteRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(400, map[string]string{"error": err.Error()})
	}

	start := time.Now()

	// Execute in sandbox
	result, err := a.executeInSandbox(c.Request().Context(), req)
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		return c.JSON(200, ExecuteResponse{
			ExecutionID:     fmt.Sprintf("exec-%d", time.Now().UnixNano()),
			Success:         false,
			Error:           err.Error(),
			ExecutionTimeMs: elapsed,
		})
	}

	return c.JSON(200, ExecuteResponse{
		ExecutionID:     fmt.Sprintf("exec-%d", time.Now().UnixNano()),
		Success:         true,
		Result:          result,
		ExecutionTimeMs: elapsed,
	})
}

func (a *ExecutorAgent) executeInSandbox(ctx context.Context, req ExecuteRequest) (string, error) {
	if a.dockerClient == nil {
		// Fallback: execute directly (not recommended for production)
		return a.executeDirect(req)
	}

	// Create ephemeral container
	resp, err := a.dockerClient.ContainerCreate(ctx,
		&container.Config{
			Image: "alpine:latest",
			Cmd:   []string{"sh", "-c", req.ToolName + " " + req.Arguments},
			Env:   a.envToSlice(req.Env),
		},
		&container.HostConfig{
			Resources: container.Resources{
				Memory:   256 * 1024 * 1024, // 256MB
				NanoCPUs: 500000000,          // 0.5 cores
			},
			NetworkMode: "none",
		},
		nil, nil, "",
	)
	if err != nil {
		return "", fmt.Errorf("create container: %w", err)
	}

	// Track container
	a.sessions[req.SessionID] = &ContainerInfo{
		ContainerID: resp.ID,
		SessionID:   req.SessionID,
		CreatedAt:   time.Now(),
	}

	// Start container
	if err := a.dockerClient.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		return "", fmt.Errorf("start container: %w", err)
	}

	// Wait for completion with timeout
	timeout := time.Duration(req.TimeoutSeconds) * time.Second
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	statusCh, errCh := a.dockerClient.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			return "", fmt.Errorf("wait container: %w", err)
		}
	case <-time.After(timeout):
		a.killContainer(ctx, resp.ID)
		return "", fmt.Errorf("execution timed out after %v", timeout)
	case status := <-statusCh:
		if status.StatusCode != 0 {
			return "", fmt.Errorf("container exited with code %d", status.StatusCode)
		}
	}

	// Get logs
	out, err := a.dockerClient.ContainerLogs(ctx, resp.ID, types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	if err != nil {
		return "", fmt.Errorf("get logs: %w", err)
	}
	defer out.Close()

	buf := make([]byte, 4096)
	n, _ := out.Read(buf)

	// Cleanup
	a.dockerClient.ContainerRemove(ctx, resp.ID, types.ContainerRemoveOptions{})
	delete(a.sessions, req.SessionID)

	return string(buf[:n]), nil
}

func (a *ExecutorAgent) executeDirect(req ExecuteRequest) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", req.ToolName+" "+req.Arguments)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("execute: %w", err)
	}
	return string(output), nil
}

func (a *ExecutorAgent) killHandler(c echo.Context) error {
	var req struct {
		SessionID string `json:"session_id"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(400, map[string]string{"error": err.Error()})
	}

	info, exists := a.sessions[req.SessionID]
	if !exists {
		return c.JSON(404, map[string]string{"error": "session not found"})
	}

	if a.dockerClient != nil {
		a.killContainer(c.Request().Context(), info.ContainerID)
	}

	delete(a.sessions, req.SessionID)
	return c.JSON(200, map[string]string{"status": "killed"})
}

func (a *ExecutorAgent) killContainer(ctx context.Context, containerID string) {
	if a.dockerClient == nil {
		return
	}

	a.dockerClient.ContainerKill(ctx, containerID, "SIGKILL")
	a.dockerClient.ContainerRemove(ctx, containerID, types.ContainerRemoveOptions{
		Force: true,
	})
}

func (a *ExecutorAgent) envToSlice(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	return out
}

func (a *ExecutorAgent) Start(addr string) error {
	a.logger.Info("Starting Executor agent", zap.String("addr", addr))

	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		a.echo.Close()
	}()

	return a.echo.Start(addr)
}

func main() {
	agent, err := NewExecutorAgent()
	if err != nil {
		panic(err)
	}

	port := os.Getenv("ADM_GRPC_PORT")
	if port == "" {
		port = "9082"
	}

	if err := agent.Start(":" + port); err != nil {
		agent.logger.Fatal("executor start failed", zap.Error(err))
	}
}
