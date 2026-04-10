package tool

import (
	"time"

	"github.com/kbukum/gokit/schema"
)

// Definition describes a tool in MCP-aligned format.
// It uses standard JSON Schema for input/output descriptions.
type Definition struct {
	// Name is the unique tool identifier.
	Name string `json:"name"`
	// Description explains what the tool does.
	Description string `json:"description"`
	// InputSchema is a standard JSON Schema describing the input.
	InputSchema schema.JSON `json:"inputSchema"`
	// OutputSchema is a standard JSON Schema describing the output (optional).
	OutputSchema schema.JSON `json:"outputSchema,omitempty"`
	// Annotations holds MCP-aligned metadata hints.
	Annotations *Annotations `json:"annotations,omitempty"`
	// ReadOnly indicates the tool does not modify external state.
	// Read-only tools can be executed concurrently in batch operations.
	ReadOnly bool `json:"readOnly,omitempty"`
	// Destructive indicates the tool may perform irreversible operations.
	Destructive bool `json:"destructive,omitempty"`
	// Timeout is the default timeout for this tool. Zero means no default.
	Timeout time.Duration `json:"-"`
	// MaxResultSize limits result content size in bytes. Zero means unlimited.
	MaxResultSize int `json:"maxResultSize,omitempty"`
}

// Annotations provides metadata hints about a tool's behavior.
// Field names align with the MCP tool specification (2025-11-25).
type Annotations struct {
	// Title is a human-readable display name.
	Title string `json:"title,omitempty"`
	// ReadOnlyHint indicates the tool does not modify external state.
	ReadOnlyHint *bool `json:"readOnlyHint,omitempty"`
	// DestructiveHint indicates the tool may perform destructive operations.
	DestructiveHint *bool `json:"destructiveHint,omitempty"`
	// IdempotentHint indicates the tool can be called repeatedly with the same result.
	IdempotentHint *bool `json:"idempotentHint,omitempty"`
	// OpenWorldHint indicates the tool interacts with external entities.
	OpenWorldHint *bool `json:"openWorldHint,omitempty"`
	// Category groups tools for filtering.
	Category string `json:"category,omitempty"`
	// Tags are searchable labels.
	Tags []string `json:"tags,omitempty"`
	// ExecutionHint tells the frontend how to handle the tool result.
	// "ui"      — tool only validates/extracts params; frontend drives the action.
	// "backend" — tool executes a real operation; result is authoritative.
	// "hybrid"  — tool executes backend AND frontend should refresh/navigate.
	// Empty string defaults to "backend" for backward compatibility.
	ExecutionHint string `json:"executionHint,omitempty"`
}

// boolPtr is a helper for creating *bool values in annotations.
func boolPtr(v bool) *bool { return &v }
