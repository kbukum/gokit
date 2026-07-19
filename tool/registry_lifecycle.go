package tool

import (
	"context"
	"fmt"

	"github.com/kbukum/gokit/component"
)

// Name returns the registry component name.
func (r *Registry) Name() string { return "tool-registry" }

// Start marks the registry ready for tool lookup and invocation.
func (r *Registry) Start(_ context.Context) error {
	r.lifecycle.MarkReady()
	return nil
}

// Stop marks the registry stopped. Registered tools are caller-owned.
func (r *Registry) Stop(_ context.Context) error {
	r.lifecycle.MarkStopped()
	return nil
}

// Health reports whether the registry is ready to serve tool calls.
func (r *Registry) Health(_ context.Context) component.Health {
	if !r.lifecycle.Ready() {
		return component.Health{Name: r.Name(), Status: component.StatusDegraded, Message: "not started"}
	}
	r.mu.RLock()
	count := r.inner.Len()
	r.mu.RUnlock()
	return component.Health{Name: r.Name(), Status: component.StatusHealthy, Message: fmt.Sprintf("tools=%d", count)}
}
