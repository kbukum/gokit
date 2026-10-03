package testutil

import (
	"context"

	"github.com/kbukum/gokit/component"
)

// Component lets tests observe startup, health, and cleanup without external resources.
type Component struct {
	ComponentName string
	StartFunc     func(context.Context) error
	StopFunc      func(context.Context) error
	HealthFunc    func(context.Context) component.Health
	QuiesceFunc   func() error
}

func (c *Component) Name() string { return c.ComponentName }
func (c *Component) Quiesce() error {
	if c.QuiesceFunc != nil {
		return c.QuiesceFunc()
	}
	return nil
}

func (c *Component) Start(ctx context.Context) error {
	if c.StartFunc != nil {
		return c.StartFunc(ctx)
	}
	return nil
}

func (c *Component) Stop(ctx context.Context) error {
	if c.StopFunc != nil {
		return c.StopFunc(ctx)
	}
	return nil
}

func (c *Component) Health(ctx context.Context) component.Health {
	if c.HealthFunc != nil {
		return c.HealthFunc(ctx)
	}
	return component.Health{Name: c.Name(), Status: component.StatusHealthy}
}
