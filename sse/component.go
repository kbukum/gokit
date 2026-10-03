package sse

import (
	"context"
	"fmt"

	"github.com/kbukum/gokit/component"
	apperrors "github.com/kbukum/gokit/errors"
)

// Component binds an injected Bus to the composition-root lifecycle without a dispatcher goroutine.
type Component struct{ bus *Bus }

// NewComponent requires the same Bus instance used by the endpoint and publishers.
func NewComponent(bus *Bus) (*Component, error) {
	if bus == nil {
		return nil, apperrors.InvalidInput("bus", "SSE bus is required")
	}
	return &Component{bus: bus}, nil
}

func (c *Component) Name() string { return "sse" }

func (c *Component) Start(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.bus.mu.Lock()
	defer c.bus.mu.Unlock()
	if c.bus.closed {
		return apperrors.ServiceUnavailable("SSE bus")
	}
	return nil
}

func (c *Component) Stop(context.Context) error {
	c.bus.Close()
	return nil
}

// Quiesce stops subscription admission and cancels streams before HTTP draining.
func (c *Component) Quiesce() error {
	c.bus.Close()
	return nil
}

func (c *Component) Health(context.Context) component.Health {
	c.bus.mu.Lock()
	defer c.bus.mu.Unlock()
	status := component.StatusHealthy
	if c.bus.closed {
		status = component.StatusUnhealthy
	}
	return component.Health{Name: c.Name(), Status: status, Message: fmt.Sprintf("%d streams", len(c.bus.subs))}
}

func (c *Component) Describe() component.Description {
	return component.Description{Name: "SSE Bus", Type: "sse", Details: "Single-instance scoped replay"}
}

var (
	_ component.Component   = (*Component)(nil)
	_ component.Describable = (*Component)(nil)
)
