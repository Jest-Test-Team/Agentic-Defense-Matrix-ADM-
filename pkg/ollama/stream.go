package ollama

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

// StreamReader reads streaming chat responses from Ollama.
type StreamReader struct {
	scanner *bufio.Scanner
	closer  io.Closer
	done    bool
}

// NewStreamReader wraps an io.ReadCloser into a StreamReader.
func NewStreamer(rc io.ReadCloser) *StreamReader {
	return &StreamReader{
		scanner: bufio.NewScanner(rc),
		closer:  rc,
	}
}

// Next reads the next chunk from the stream. Returns false when done or on error.
func (s *StreamReader) Next() bool {
	if s.done {
		return false
	}
	return s.scanner.Scan()
}

// Chunk returns the current streaming chunk.
func (s *StreamReader) Chunk() (*ChatResponse, error) {
	line := s.scanner.Text()
	if line == "" {
		return nil, io.EOF
	}

	var resp ChatResponse
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		return nil, fmt.Errorf("decode stream chunk: %w", err)
	}

	if resp.Done {
		s.done = true
	}

	return &resp, nil
}

// Err returns any error encountered during scanning.
func (s *StreamReader) Err() error {
	return s.scanner.Err()
}

// Close closes the underlying reader.
func (s *StreamReader) Close() error {
	return s.closer.Close()
}

// Collect reads all streaming chunks and returns the complete concatenated response.
func Collect(stream *StreamReader) (*ChatResponse, error) {
	defer stream.Close()

	var (
		model       string
		fullContent string
		totalTokens int
	)

	for stream.Next() {
		chunk, err := stream.Chunk()
		if err != nil {
			return nil, err
		}

		if chunk.Model != "" {
			model = chunk.Model
		}

		fullContent += chunk.Message.Content
		totalTokens += chunk.EvalCount
	}

	if err := stream.Err(); err != nil {
		return nil, fmt.Errorf("stream read error: %w", err)
	}

	return &ChatResponse{
		Model: model,
		Message: ChatMessage{
			Role:    "assistant",
			Content: fullContent,
		},
		Done:      true,
		EvalCount: totalTokens,
	}, nil
}

// CollectWithToolCalls reads all chunks and extracts tool calls.
func CollectWithToolCalls(stream *StreamReader) (*ChatResponse, []OpenAIToolCall, error) {
	defer stream.Close()

	var (
		model        string
		fullContent  string
		allToolCalls []ToolCall
		totalTokens  int
	)

	for stream.Next() {
		chunk, err := stream.Chunk()
		if err != nil {
			return nil, nil, err
		}

		if chunk.Model != "" {
			model = chunk.Model
		}

		fullContent += chunk.Message.Content
		allToolCalls = append(allToolCalls, chunk.Message.ToolCalls...)
		totalTokens += chunk.EvalCount
	}

	if err := stream.Err(); err != nil {
		return nil, nil, fmt.Errorf("stream read error: %w", err)
	}

	resp := &ChatResponse{
		Model: model,
		Message: ChatMessage{
			Role:      "assistant",
			Content:   fullContent,
			ToolCalls: allToolCalls,
		},
		Done:      true,
		EvalCount: totalTokens,
	}

	adapter := NewSchemaAdapter()
	openaiCalls, err := adapter.ParseToolCalls(resp.Message)
	if err != nil {
		return resp, nil, err
	}

	return resp, openaiCalls, nil
}
