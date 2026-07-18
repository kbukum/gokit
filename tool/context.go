package tool

import (
	"context"
	"time"
)

// Context carries execution metadata through the tool call chain. It embeds context.Context for cancellation, deadlines, and values, and adds tool-specific fields (request ID, tool use ID, metadata).
//
// Use NewContext to create one from a standard context.Context.
type Context struct {
	context.Context

	// RequestID identifies the overall request (e.g., agent turn).
	RequestID string

	// ToolUseID identifies this specific tool invocation.
	ToolUseID string

	// MaxResultSize limits result content size in bytes. Zero means unlimited.
	MaxResultSize int

	metadata map[string]any
}

// NewContext creates a Context from a standard context.Context.
func NewContext(ctx context.Context) *Context {
	return &Context{Context: ctx}
}

// Background creates a Context from context.Background().
func Background() *Context {
	return NewContext(context.Background())
}

// Set stores a metadata value.
func (c *Context) Set(key string, value any) {
	if c.metadata == nil {
		c.metadata = make(map[string]any)
	}
	c.metadata[key] = value
}

// Get retrieves a metadata value.
func (c *Context) Get(key string) (any, bool) {
	if c.metadata == nil {
		return nil, false
	}
	v, ok := c.metadata[key]
	return v, ok
}

// Metadata returns a copy of all metadata.
func (c *Context) Metadata() map[string]any {
	if c.metadata == nil {
		return nil
	}
	cp := make(map[string]any, len(c.metadata))
	for k, v := range c.metadata {
		cp[k] = v
	}
	return cp
}

// WithTimeout returns a derived Context with a timeout and its cancel function.
func (c *Context) WithTimeout(d time.Duration) (*Context, context.CancelFunc) {
	newCtx, cancel := context.WithTimeout(c.Context, d)
	nc := c.clone()
	nc.Context = newCtx
	return nc, cancel
}

// WithCancel returns a derived Context with a cancel function.
func (c *Context) WithCancel() (*Context, context.CancelFunc) {
	newCtx, cancel := context.WithCancel(c.Context)
	nc := c.clone()
	nc.Context = newCtx
	return nc, cancel
}

func (c *Context) clone() *Context {
	nc := &Context{
		Context:       c.Context,
		RequestID:     c.RequestID,
		ToolUseID:     c.ToolUseID,
		MaxResultSize: c.MaxResultSize,
	}
	if c.metadata != nil {
		nc.metadata = make(map[string]any, len(c.metadata))
		for k, v := range c.metadata {
			nc.metadata[k] = v
		}
	}
	return nc
}
