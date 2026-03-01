package httpclient

import (
	"context"

	"github.com/kbukum/gokit/component"
)

// Component wraps an Adapter with lifecycle management.
// Use this when the HTTP adapter is part of a managed application
// (e.g., with bootstrap.Start/Stop).
type Component struct {
	adapter *Adapter
	config  Config
	opts    []Option
}

// compile-time assertions
var _ component.Component = (*Component)(nil)
var _ component.Describable = (*Component)(nil)

// NewComponent creates a new HTTP adapter component.
// The adapter is created lazily in Start().
func NewComponent(cfg Config, opts ...Option) *Component {
	return &Component{config: cfg, opts: opts}
}

// Name returns the component name.
func (c *Component) Name() string {
	name := c.config.Name
	if name == "" {
		name = "http"
	}
	return name
}

// Start initializes the HTTP adapter.
func (c *Component) Start(_ context.Context) error {
	a, err := New(c.config, c.opts...)
	if err != nil {
		return err
	}
	c.adapter = a
	return nil
}

// Stop closes the HTTP adapter and releases resources.
func (c *Component) Stop(ctx context.Context) error {
	if c.adapter != nil {
		return c.adapter.Close(ctx)
	}
	return nil
}

// Health returns the adapter health status.
func (c *Component) Health(ctx context.Context) component.Health {
	status := component.StatusHealthy
	if c.adapter == nil || !c.adapter.IsAvailable(ctx) {
		status = component.StatusUnhealthy
	}
	return component.Health{
		Name:   c.Name(),
		Status: status,
	}
}

// Describe returns component description for the bootstrap summary.
func (c *Component) Describe() component.Description {
	return component.Description{
		Name:    c.Name(),
		Type:    "http-adapter",
		Details: c.config.BaseURL,
	}
}

// Adapter returns the underlying HTTP adapter. Must be called after Start().
func (c *Component) Adapter() *Adapter {
	return c.adapter
}
