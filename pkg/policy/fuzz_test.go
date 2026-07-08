package policy

import (
	"context"
	"testing"
	"unicode/utf8"
)

func FuzzPolicyEvaluate(f *testing.F) {
	f.Add("agent_permissions", "planner", "read_file", "session-1", 0)
	f.Add("agent_permissions", "executor", "run_command", "session-1", 1)
	f.Add("agent_permissions", "summarizer", "read_conversation", "session-1", 0)
	f.Add("agent_permissions", "planner", "run_command", "session-1", 0)
	f.Add("agent_permissions", "executor", "read_file", "session-1", 1)
	f.Add("agent_permissions", "unknown", "any_tool", "session-1", 0)

	f.Fuzz(func(t *testing.T, policyID, role, tool, session string, count int) {
		if !utf8.ValidString(policyID) || !utf8.ValidString(role) ||
			!utf8.ValidString(tool) || !utf8.ValidString(session) {
			return
		}

		if count < 0 || count > 10000 {
			return
		}

		engine := NewEngine()
		_ = engine.LoadPolicy("agent_permissions", "Agent Permissions", "default", "Default policy")

		req := &EvaluationRequest{
			PolicyID:     policyID,
			AgentRole:    role,
			ToolName:     tool,
			SessionID:    session,
			RequestCount: count,
		}

		result, err := engine.Evaluate(context.Background(), req)
		if err != nil {
			if policyID != "nonexistent" {
				t.Errorf("unexpected error: %v", err)
			}
			return
		}

		if result == nil {
			t.Fatal("Evaluate returned nil result")
		}

		validRoles := map[string]bool{
			"planner":    true,
			"executor":   true,
			"summarizer": true,
		}

		if validRoles[role] && !result.Allowed && count <= 60 {
			// Valid role with valid tool should be allowed
			validTools := map[string][]string{
				"planner":    {"read_file", "list_directory", "query_knowledge_base"},
				"executor":   {"run_command", "http_request", "write_file"},
				"summarizer": {"read_conversation"},
			}
			for _, t := range validTools[role] {
				if t == tool {
					t.Logf("Expected allowed for role=%s tool=%s but got denied", role, tool)
				}
			}
		}
	})
}

func FuzzPolicyLoad(f *testing.F) {
	f.Add("policy-1", "Test Policy", "package test\nallow = true")
	f.Add("policy-2", "Another Policy", "package test\nallow = false")
	f.Add("", "", "")
	f.Add("policy-3", "Empty Rego", "")

	f.Fuzz(func(t *testing.T, id, name, rego string) {
		if !utf8.ValidString(id) || !utf8.ValidString(name) || !utf8.ValidString(rego) {
			return
		}

		engine := NewEngine()

		// Loading should not panic
		_ = engine.LoadPolicy(id, name, rego, "test")

		// Retrieving should work
		policy, err := engine.GetPolicy(id)
		if id == "" {
			if err == nil {
				t.Errorf("expected error for empty id")
			}
			return
		}

		if err != nil {
			t.Errorf("unexpected error: %v", err)
			return
		}

		if policy != nil && policy.Name != name {
			t.Errorf("name mismatch: got %q, want %q", policy.Name, name)
		}
	})
}

func FuzzPolicyDelete(f *testing.F) {
	f.Add("policy-1")
	f.Add("policy-2")
	f.Add("")
	f.Add("nonexistent")

	f.Fuzz(func(t *testing.T, id string) {
		if !utf8.ValidString(id) {
			return
		}

		engine := NewEngine()
		_ = engine.LoadPolicy(id, "Test", "package test", "test")

		err := engine.DeletePolicy(id)
		if id == "" && err == nil {
			t.Errorf("expected error for empty id")
		}

		// Verify deleted
		policy, _ := engine.GetPolicy(id)
		if policy != nil && id != "" {
			t.Errorf("policy still exists after delete")
		}
	})
}
