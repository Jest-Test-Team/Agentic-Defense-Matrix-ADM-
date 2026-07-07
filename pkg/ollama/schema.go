package ollama

import (
	"encoding/json"
	"fmt"
	"strings"
)

// SchemaAdapter converts between OpenAI tool schemas and Ollama tool formats.
type SchemaAdapter struct{}

// NewSchemaAdapter creates a new adapter.
func NewSchemaAdapter() *SchemaAdapter {
	return &SchemaAdapter{}
}

// OpenAITool represents a tool in OpenAI function calling format.
type OpenAITool struct {
	Type     string             `json:"type"`
	Function OpenAIFunction     `json:"function"`
}

// OpenAIFunction is the OpenAI function definition.
type OpenAIFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// OpenAIToolCall is a tool call from the LLM response.
type OpenAIToolCall struct {
	ID       string                `json:"id"`
	Type     string                `json:"type"`
	Function OpenAIToolCallFunction `json:"function"`
}

// OpenAIToolCallFunction contains the function name and arguments.
type OpenAIToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToOllamaTools converts OpenAI tool definitions to Ollama format.
func (a *SchemaAdapter) ToOllamaTools(tools []OpenAITool) []Tool {
	out := make([]Tool, 0, len(tools))
	for _, t := range tools {
		out = append(out, Tool{
			Type: "function",
			Function: FunctionDefinition{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				Parameters:  t.Function.Parameters,
			},
		})
	}
	return out
}

// ToOllamaTool converts a single OpenAI tool to Ollama format.
func (a *SchemaAdapter) ToOllamaTool(tool OpenAITool) Tool {
	return Tool{
		Type: "function",
		Function: FunctionDefinition{
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			Parameters:  tool.Function.Parameters,
		},
	}
}

// ParseToolCalls extracts tool calls from Ollama chat response messages.
// Ollama returns tool calls in the message.tool_calls field.
func (a *SchemaAdapter) ParseToolCalls(msg ChatMessage) ([]OpenAIToolCall, error) {
	if len(msg.ToolCalls) == 0 {
		return nil, nil
	}

	calls := make([]OpenAIToolCall, 0, len(msg.ToolCalls))
	for i, tc := range msg.ToolCalls {
		call := OpenAIToolCall{
			ID:   fmt.Sprintf("call_%d", i),
			Type: "function",
			Function: OpenAIToolCallFunction{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		}
		calls = append(calls, call)
	}

	return calls, nil
}

// ValidateToolSchema checks if a tool schema is valid for Ollama.
func (a *SchemaAdapter) ValidateToolSchema(tool OpenAITool) error {
	if tool.Type != "function" {
		return fmt.Errorf("tool type must be 'function', got %q", tool.Type)
	}

	if tool.Function.Name == "" {
		return fmt.Errorf("function name is required")
	}

	if !isValidFunctionName(tool.Function.Name) {
		return fmt.Errorf("function name %q contains invalid characters (alphanumeric and underscores only)", tool.Function.Name)
	}

	if tool.Function.Parameters != nil {
		var params map[string]interface{}
		if err := json.Unmarshal(tool.Function.Parameters, &params); err != nil {
			return fmt.Errorf("invalid parameters JSON: %w", err)
		}

		if params["type"] != "object" {
			return fmt.Errorf("parameters type must be 'object'")
		}
	}

	return nil
}

// MergeToolSchemas combines multiple tool schema arrays, deduplicating by name.
func (a *SchemaAdapter) MergeToolSchemas(schemas ...[]OpenAITool) []OpenAITool {
	seen := make(map[string]bool)
	merged := make([]OpenAITool, 0)

	for _, batch := range schemas {
		for _, tool := range batch {
			if !seen[tool.Function.Name] {
				seen[tool.Function.Name] = true
				merged = append(merged, tool)
			}
		}
	}

	return merged
}

// BuildToolResponse creates a ChatMessage with tool results for sending back to the LLM.
func (a *SchemaAdapter) BuildToolResponse(toolCallID, content string) ChatMessage {
	return ChatMessage{
		Role:    "tool",
		Content: content,
	}
}

// FormatToolError formats an error as a tool response message.
func (a *SchemaAdapter) FormatToolError(err error) string {
	return fmt.Sprintf("Error: %s", err.Error())
}

func isValidFunctionName(name string) bool {
	if name == "" {
		return false
	}

	for i, ch := range name {
		if i == 0 {
			if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_') {
				return false
			}
		} else {
			if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_') {
				return false
			}
		}
	}

	reserved := []string{"function", "tool", "system", "user", "assistant"}
	for _, r := range reserved {
		if strings.ToLower(name) == r {
			return false
		}
	}

	return true
}
