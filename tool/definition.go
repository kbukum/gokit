package tool

import (
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
	// Annotations holds non-executable metadata hints.
	Annotations Annotations `json:"annotations,omitzero"`
	// Envelope is the executable permission envelope for this tool. It
	// is the single source of truth for what the tool may do at runtime.
	// The zero value adds no extra capabilities and follows Envelope's
	// field-level defaults (for example, nil network/filesystem/subprocess
	// policies deny those capabilities, while empty scopes require none).
	// Safety, timeout, and result-size policy live here — not as separate
	// Definition fields.
	Envelope Envelope `json:"envelope,omitempty"`
}

// Annotations provides non-executable metadata about a tool.
type Annotations struct {
	// Title is a human-readable display name.
	Title string `json:"title,omitempty"`
	// Category groups tools for filtering.
	Category string `json:"category,omitempty"`
	// Tags are searchable labels.
	Tags []string `json:"tags,omitempty"`
	// IdempotentHint indicates the tool can be called repeatedly with the same result.
	IdempotentHint *bool `json:"idempotentHint,omitempty"`
	// ExecutionHint tells the frontend how to handle the tool result.
	ExecutionHint ExecutionHint `json:"executionHint,omitzero"`
}

// ExecutionHint tells the frontend how to handle the tool result.
type ExecutionHint string

const (
	// ExecutionBackend means the tool executes a real operation.
	ExecutionBackend ExecutionHint = "backend"
	// ExecutionUI means the tool only validates/extracts params; frontend drives the action.
	ExecutionUI ExecutionHint = "ui"
	// ExecutionHybrid means the tool executes backend and frontend should refresh/navigate.
	ExecutionHybrid ExecutionHint = "hybrid"
)

// Resolved returns the effective hint, defaulting to Backend when zero.
func (h ExecutionHint) Resolved() ExecutionHint {
	if h == "" {
		return ExecutionBackend
	}
	return h
}
