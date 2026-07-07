package main

import (
	"sync"
	"time"

	"github.com/adm/pkg/ringbuffer"
)

type CorrelationRule struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	EventTypes  []string `json:"event_types"`
	Severity    string   `json:"severity"`
	Window      int      `json:"window_seconds"`
	Threshold   int      `json:"threshold"`
	Enabled     bool     `json:"enabled"`
}

type RuleEngine struct {
	mu    sync.RWMutex
	rules map[string]*CorrelationRule
	hits  map[string][]time.Time
}

func NewRuleEngine() *RuleEngine {
	engine := &RuleEngine{
		rules: make(map[string]*CorrelationRule),
		hits:  make(map[string][]time.Time),
	}

	// Default rules
	engine.Add(CorrelationRule{
		ID:          "rule_001",
		Name:        "High-frequency prompt injection",
		Description: "N prompt injection attempts from same session within 60s",
		EventTypes:  []string{"prompt_injection"},
		Severity:    "high",
		Window:      60,
		Threshold:   5,
		Enabled:     true,
	})

	engine.Add(CorrelationRule{
		ID:          "rule_002",
		Name:        "Tool chain anomaly",
		Description: "read_secret -> external_send within 10s",
		EventTypes:  []string{"tool_call"},
		Severity:    "critical",
		Window:      10,
		Threshold:   2,
		Enabled:     true,
	})

	engine.Add(CorrelationRule{
		ID:          "rule_003",
		Name:        "Syscall anomaly",
		Description: "Unauthorized process spawn from agent container",
		EventTypes:  []string{"syscall_anomaly"},
		Severity:    "high",
		Window:      5,
		Threshold:   1,
		Enabled:     true,
	})

	engine.Add(CorrelationRule{
		ID:          "rule_004",
		Name:        "RAG poisoning",
		Description: "Knowledge base query -> malicious URL",
		EventTypes:  []string{"rag_query", "egress_violation"},
		Severity:    "critical",
		Window:      30,
		Threshold:   2,
		Enabled:     true,
	})

	engine.Add(CorrelationRule{
		ID:          "rule_005",
		Name:        "Reverse shell indicators",
		Description: "Outbound connection + shell process",
		EventTypes:  []string{"process_exec", "egress_attempt"},
		Severity:    "critical",
		Window:      5,
		Threshold:   2,
		Enabled:     true,
	})

	engine.Add(CorrelationRule{
		ID:          "rule_006",
		Name:        "Excessive API rate",
		Description: "Too many requests from single session",
		EventTypes:  []string{"api_request"},
		Severity:    "medium",
		Window:      60,
		Threshold:   100,
		Enabled:     true,
	})

	return engine
}

func (e *RuleEngine) Add(rule CorrelationRule) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rules[rule.ID] = &rule
}

func (e *RuleEngine) Get(id string) *CorrelationRule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.rules[id]
}

func (e *RuleEngine) List() []*CorrelationRule {
	e.mu.RLock()
	defer e.mu.RUnlock()

	out := make([]*CorrelationRule, 0, len(e.rules))
	for _, r := range e.rules {
		out = append(out, r)
	}
	return out
}

func (e *RuleEngine) Evaluate(event *ringbuffer.Event) {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := time.Now()

	for _, rule := range e.rules {
		if !rule.Enabled {
			continue
		}

		if !e.matchesEventTypes(rule, event) {
			continue
		}

		key := rule.ID + ":" + event.SessionID
		e.hits[key] = append(e.hits[key], now)

		// Clean old hits
		window := time.Duration(rule.Window) * time.Second
		cutoff := now.Add(-window)
		filtered := make([]time.Time, 0)
		for _, t := range e.hits[key] {
			if t.After(cutoff) {
				filtered = append(filtered, t)
			}
		}
		e.hits[key] = filtered

		// Check threshold
		if len(filtered) >= rule.Threshold {
			// Alert triggered!
			delete(e.hits, key)
			// In production: send to alert channel
		}
	}
}

func (e *RuleEngine) matchesEventTypes(rule *CorrelationRule, event *ringbuffer.Event) bool {
	if len(rule.EventTypes) == 0 {
		return true
	}

	for _, t := range rule.EventTypes {
		if t == event.EventType {
			return true
		}
	}
	return false
}
