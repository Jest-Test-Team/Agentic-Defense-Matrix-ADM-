package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/adm/pkg/ringbuffer"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/zap"
)

type SIEMEngine struct {
	echo        *echo.Echo
	ringBuffer  *ringbuffer.RingBuffer
	rules       *RuleEngine
	redis       *RedisClient
	logger      *zap.Logger
	alertChan   chan *Alert
}

type Alert struct {
	ID        string   `json:"id"`
	RuleID    string   `json:"rule_id"`
	RuleName  string   `json:"rule_name"`
	Severity  string   `json:"severity"`
	CreatedAt int64    `json:"created_at"`
	EventIDs  []string `json:"event_ids"`
	Status    string   `json:"status"`
}

func NewSIEMEngine() (*SIEMEngine, error) {
	logger, _ := zap.NewProduction()

	ringBuf := ringbuffer.New(65536)
	ruleEngine := NewRuleEngine()

	redis, err := NewRedisClient()
	if err != nil {
		logger.Warn("Redis not available, running in degraded mode", zap.Error(err))
	}

	e := echo.New()
	e.HideBanner = true

	engine := &SIEMEngine{
		echo:       e,
		ringBuffer: ringBuf,
		rules:      ruleEngine,
		redis:      redis,
		logger:     logger,
		alertChan:  make(chan *Alert, 1000),
	}

	engine.setupRoutes()
	engine.setupMiddleware()

	return engine, nil
}

func (e *SIEMEngine) setupMiddleware() {
	e.echo.Use(middleware.Logger())
	e.echo.Use(middleware.Recover())
	e.echo.Use(middleware.CORS())
	e.echo.Use(middleware.TimeoutWithConfig(middleware.TimeoutConfig{
		Timeout: 30 * time.Second,
	}))
}

func (e *SIEMEngine) setupRoutes() {
	e.echo.GET("/health", e.healthHandler)
	e.echo.GET("/ready", e.readyHandler)
	e.echo.POST("/api/v1/events", e.ingestEvent)
	e.echo.POST("/api/v1/events/batch", e.ingestBatch)
	e.echo.GET("/api/v1/events", e.queryEvents)
	e.echo.GET("/api/v1/alerts", e.getAlerts)
	e.echo.GET("/api/v1/rules", e.getRules)
	e.echo.POST("/api/v1/rules", e.addRule)
	e.echo.GET("/api/v1/metrics", e.getMetrics)
}

func (e *SIEMEngine) Start(addr string) error {
	e.logger.Info("Starting SIEM engine", zap.String("addr", addr))

	// Start alert processor
	go e.processAlerts()

	// Start ring buffer consumer
	go e.consumeRingBuffer()

	// Graceful shutdown
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		e.logger.Info("Shutting down SIEM engine...")
		e.echo.Close()
	}()

	return e.echo.Start(addr)
}

func (e *SIEMEngine) processAlerts() {
	for alert := range e.alertChan {
		e.logger.Info("Alert triggered",
			zap.String("rule_id", alert.RuleID),
			zap.String("severity", alert.Severity),
		)

		// In production: send webhook to Gateway
		// POST http://gateway:8080/api/v1/admin/alert
	}
}

func (e *SIEMEngine) consumeRingBuffer() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		events := e.ringBuffer.Drain(100)
		for _, event := range events {
			e.rules.Evaluate(event)
		}
	}
}

func (e *SIEMEngine) healthHandler(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]interface{}{
		"healthy": true,
		"version": "0.1.0",
	})
}

func (e *SIEMEngine) readyHandler(c echo.Context) error {
	if e.redis != nil && !e.redis.IsHealthy() {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"status": "redis unavailable",
		})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ready"})
}

func (e *SIEMEngine) ingestEvent(c echo.Context) error {
	var event ringbuffer.Event
	if err := c.Bind(&event); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	if !e.ringBuffer.Push(&event) {
		return c.JSON(http.StatusTooManyRequests, map[string]string{"error": "ring buffer full"})
	}

	// Store in Redis if available
	if e.redis != nil {
		ctx := context.Background()
		data, _ := json.Marshal(event)
		e.redis.Client.RPush(ctx, "siem:events", data).Err()
	}

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"accepted": true,
		"event_id": event.ID,
	})
}

func (e *SIEMEngine) ingestBatch(c echo.Context) error {
	var events []ringbuffer.Event
	if err := c.Bind(&events); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	accepted := 0
	for _, event := range events {
		if e.ringBuffer.Push(&event) {
			accepted++
		}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"accepted": accepted,
		"total":    len(events),
	})
}

func (e *SIEMEngine) queryEvents(c echo.Context) error {
	limit := 100
	if l := c.QueryParam("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}

	stats := e.ringBuffer.GetStats()
	return c.JSON(http.StatusOK, map[string]interface{}{
		"buffer_stats": stats,
		"limit":        limit,
	})
}

func (e *SIEMEngine) getAlerts(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]interface{}{
		"alerts": []Alert{},
	})
}

func (e *SIEMEngine) getRules(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]interface{}{
		"rules": e.rules.List(),
	})
}

func (e *SIEMEngine) addRule(c echo.Context) error {
	var rule CorrelationRule
	if err := c.Bind(&rule); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	e.rules.Add(rule)
	return c.JSON(http.StatusCreated, map[string]interface{}{
		"success": true,
		"rule_id": rule.ID,
	})
}

func (e *SIEMEngine) getMetrics(c echo.Context) error {
	stats := e.ringBuffer.GetStats()
	return c.JSON(http.StatusOK, map[string]interface{}{
		"ring_buffer_capacity": stats.Capacity,
		"ring_buffer_used":     stats.Length,
		"ring_buffer_dropped":  stats.Dropped,
	})
}

func main() {
	engine, err := NewSIEMEngine()
	if err != nil {
		panic(err)
	}

	port := os.Getenv("ADM_PORT")
	if port == "" {
		port = "9091"
	}

	if err := engine.Start(":" + port); err != nil {
		engine.logger.Fatal(err)
	}
}
