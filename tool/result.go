package tool

import (
	"encoding/json"
	"fmt"

	"github.com/kbukum/gokit/ai"
)

// Result is the structured output of a tool execution. It separates structured output (for programmatic use) from human-readable content (for LLM consumption).
type Result struct {
	// Output is the structured JSON output for programmatic use.
	Output json.RawMessage `json:"output,omitempty"`
	// Content is a human-readable string for LLM consumption. If empty, Output is used as the content string.
	Content string `json:"content,omitempty"`
	// IsError indicates the tool encountered an error.
	IsError bool `json:"is_error,omitempty"`
	// Metadata carries additional data (timing, token count, etc.).
	Metadata map[string]any `json:"metadata,omitempty"`
}

// Text returns the human-readable content. Falls back to Output if Content is empty.
func (r *Result) Text() string {
	if r.Content != "" {
		return r.Content
	}
	if r.Output != nil {
		return string(r.Output)
	}
	return ""
}

// ToolResultBlock returns the shared GenAI tool-result content block for this result.
func (r *Result) ToolResultBlock(id string) ai.ToolResultBlock {
	if r == nil {
		return ai.ToolResultBlock{ID: id}
	}
	return ai.ToolResultBlock{ID: id, Content: r.Text(), IsError: r.IsError}
}

// ResultBlock builds a GenAI tool-result block from an optional result and error. If err is non-nil it takes priority and produces an error block. If r is nil and err is nil, an empty success block is returned.
func ResultBlock(id string, r *Result, err error) ai.ToolResultBlock {
	if err != nil {
		return ai.ToolResultBlock{ID: id, Content: err.Error(), IsError: true}
	}
	return r.ToolResultBlock(id) // nil-safe
}

// SetMeta sets a metadata key-value pair.
func (r *Result) SetMeta(key string, value any) {
	if r.Metadata == nil {
		r.Metadata = make(map[string]any)
	}
	r.Metadata[key] = value
}

// TextResult creates a Result with text content only.
func TextResult(content string) *Result {
	return &Result{Content: content}
}

// ErrorResult creates an error Result.
func ErrorResult(content string) *Result {
	return &Result{Content: content, IsError: true}
}

// JSONResult creates a Result from a JSON-serializable value. Both Output (raw JSON) and Content (JSON string) are set.
func JSONResult(v any) (*Result, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("tool result: marshal: %w", err)
	}
	return &Result{
		Output:  data,
		Content: string(data),
	}, nil
}

// MustJSONResult is like JSONResult but panics on error.
func MustJSONResult(v any) *Result {
	r, err := JSONResult(v)
	if err != nil {
		panic(err)
	}
	return r
}
