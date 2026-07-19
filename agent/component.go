package agent

import (
	"context"

	"github.com/kbukum/gokit/component"
)

// Name returns the agent name (configured Model or "agent").
func (a *Agent) Name() string {
	if a.config.Model != "" {
		return "agent-" + a.config.Model
	}
	return "agent"
}

// IsAvailable reports whether the underlying provider is reachable.
func (a *Agent) IsAvailable(ctx context.Context) bool {
	if a.config.Provider == nil {
		return false
	}
	return a.config.Provider.IsAvailable(ctx)
}

// Start marks the agent ready. The underlying provider is started independently by bootstrap; the agent itself only flips its lifecycle flag.
func (a *Agent) Start(_ context.Context) error {
	a.lifecycle.MarkReady()
	return nil
}

// Stop marks the agent as stopped. Inflight Run calls observe ctx cancellation.
func (a *Agent) Stop(_ context.Context) error {
	a.lifecycle.MarkStopped()
	return nil
}

// Health reports the agent's readiness and the underlying provider's reachability.
func (a *Agent) Health(ctx context.Context) component.Health {
	if !a.lifecycle.Ready() {
		return component.Health{Name: a.Name(), Status: component.StatusDegraded, Message: "not started"}
	}
	if a.config.Provider != nil && !a.config.Provider.IsAvailable(ctx) {
		return component.Health{Name: a.Name(), Status: component.StatusUnhealthy, Message: "provider unreachable"}
	}
	msg := "ready"
	if last := a.lifecycle.LastCall(); !last.IsZero() {
		msg = "last_turn=" + last.UTC().Format("2006-01-02T15:04:05Z")
	}
	return component.Health{Name: a.Name(), Status: component.StatusHealthy, Message: msg}
}
