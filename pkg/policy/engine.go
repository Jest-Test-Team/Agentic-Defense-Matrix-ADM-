package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Engine evaluates OPA Rego policies.
type Engine struct {
	policies map[string]*Policy
}

// Policy represents a loaded Rego policy.
type Policy struct {
	ID          string
	Name        string
	RegoSource  string
	Description string
	LoadedAt    time.Time
}

// NewEngine creates a new policy engine.
func NewEngine() *Engine {
	return &Engine{
		policies: make(map[string]*Policy),
	}
}

// LoadPolicy adds or updates a policy.
func (e *Engine) LoadPolicy(id, name, regoSource, description string) error {
	if regoSource == "" {
		return fmt.Errorf("rego source cannot be empty")
	}

	e.policies[id] = &Policy{
		ID:          id,
		Name:        name,
		RegoSource:  regoSource,
		Description: description,
		LoadedAt:    time.Now(),
	}
	return nil
}

// GetPolicy returns a policy by ID.
func (e *Engine) GetPolicy(id string) (*Policy, error) {
	p, ok := e.policies[id]
	if !ok {
		return nil, fmt.Errorf("policy %q not found", id)
	}
	return p, nil
}

// DeletePolicy removes a policy.
func (e *Engine) DeletePolicy(id string) error {
	if _, ok := e.policies[id]; !ok {
		return fmt.Errorf("policy %q not found", id)
	}
	delete(e.policies, id)
	return nil
}

// ListPolicies returns all loaded policies.
func (e *Engine) ListPolicies() []*Policy {
	out := make([]*Policy, 0, len(e.policies))
	for _, p := range e.policies {
		out = append(out, p)
	}
	return out
}

// EvaluationRequest is the input for policy evaluation.
type EvaluationRequest struct {
	PolicyID       string            `json:"policy_id"`
	AgentRole      string            `json:"agent_role"`
	ToolName       string            `json:"tool_name"`
	SessionID      string            `json:"session_id"`
	RequestCount   int               `json:"request_count"`
	Context        map[string]string `json:"context"`
}

// EvaluationResult is the outcome of policy evaluation.
type EvaluationResult struct {
	Allowed         bool     `json:"allowed"`
	Reasons         []string `json:"reasons"`
	EvaluationTimeNS int64   `json:"evaluation_time_ns"`
}

// Evaluate evaluates a request against a policy.
func (e *Engine) Evaluate(ctx context.Context, req *EvaluationRequest) (*EvaluationResult, error) {
	start := time.Now()

	policy, ok := e.policies[req.PolicyID]
	if !ok {
		return nil, fmt.Errorf("policy %q not found", req.PolicyID)
	}

	_ = policy // Would evaluate Rego in production via OPA client

	// Basic role-based evaluation
	result := &EvaluationResult{
		Allowed: e.evaluateBasicRules(req),
	}

	if !result.Allowed {
		result.Reasons = []string{
			fmt.Sprintf("agent role %q not authorized for tool %q", req.AgentRole, req.ToolName),
		}
	}

	result.EvaluationTimeNS = time.Since(start).Nanoseconds()
	return result, nil
}

// evaluateBasicRules performs basic role-based access control.
func (e *Engine) evaluateBasicRules(req *EvaluationRequest) bool {
	roleTools := map[string][]string{
		"planner":    {"read_file", "list_directory", "query_knowledge_base"},
		"executor":   {"run_command", "http_request", "write_file"},
		"summarizer": {"read_conversation"},
	}

	tools, ok := roleTools[req.AgentRole]
	if !ok {
		return false
	}

	for _, t := range tools {
		if t == req.ToolName {
			return true
		}
	}
	return false
}

// MarshalJSON serializes the engine state.
func (e *Engine) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]interface{}{
		"policies": e.policies,
		"count":    len(e.policies),
	})
}
