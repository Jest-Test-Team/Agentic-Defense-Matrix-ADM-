package ollama

import (
	"sync"
	"time"
)

// Conversation tracks a multi-turn chat session with tool call history.
type Conversation struct {
	mu       sync.RWMutex
	messages []ChatMessage
	history  []ToolCallRecord
}

// ToolCallRecord tracks a tool call and its result.
type ToolCallRecord struct {
	ID        string
	Name      string
	Arguments string
	Result    string
	Timestamp time.Time
}

// NewConversation creates an empty conversation.
func NewConversation() *Conversation {
	return &Conversation{
		messages: make([]ChatMessage, 0),
		history:  make([]ToolCallRecord, 0),
	}
}

// AddUserMessage appends a user message.
func (c *Conversation) AddUserMessage(content string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = append(c.messages, ChatMessage{
		Role:    "user",
		Content: content,
	})
}

// AddAssistantMessage appends an assistant message.
func (c *Conversation) AddAssistantMessage(content string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = append(c.messages, ChatMessage{
		Role:    "assistant",
		Content: content,
	})
}

// AddToolMessage appends a tool result message.
func (c *Conversation) AddToolMessage(content string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = append(c.messages, ChatMessage{
		Role:    "tool",
		Content: content,
	})
}

// RecordToolCall records a tool call and its result.
func (c *Conversation) RecordToolCall(id, name, args, result string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.history = append(c.history, ToolCallRecord{
		ID:        id,
		Name:      name,
		Arguments: args,
		Result:    result,
		Timestamp: time.Now(),
	})
}

// Messages returns a copy of all messages for sending to the LLM.
func (c *Conversation) Messages() []ChatMessage {
	c.mu.RLock()
	defer c.mu.RUnlock()

	out := make([]ChatMessage, len(c.messages))
	copy(out, c.messages)
	return out
}

// ToolHistory returns all recorded tool calls.
func (c *Conversation) ToolHistory() []ToolCallRecord {
	c.mu.RLock()
	defer c.mu.RUnlock()

	out := make([]ToolCallRecord, len(c.history))
	copy(out, c.history)
	return out
}

// Clear removes all messages and history.
func (c *Conversation) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = c.messages[:0]
	c.history = c.history[:0]
}

// LastMessage returns the most recent message, or nil.
func (c *Conversation) LastMessage() *ChatMessage {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.messages) == 0 {
		return nil
	}
	msg := c.messages[len(c.messages)-1]
	return &msg
}

// Length returns the number of messages.
func (c *Conversation) Length() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.messages)
}

// TruncateToLimit truncates conversation to fit within a token limit.
// This is a heuristic: ~4 chars per token, keeps system + recent messages.
func (c *Conversation) TruncateToLimit(maxMessages int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.messages) <= maxMessages {
		return
	}

	// Always keep the first message (context) and the most recent ones
	truncated := make([]ChatMessage, 0, maxMessages)
	truncated = append(truncated, c.messages[0]) // first message

	keep := maxMessages - 1
	start := len(c.messages) - keep
	truncated = append(truncated, c.messages[start:]...)

	c.messages = truncated
}
