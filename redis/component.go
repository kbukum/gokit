package redis

import (
	"context"
	"fmt"

	"github.com/skillsenselab/gokit/component"
	"github.com/skillsenselab/gokit/logger"
)

// Component wraps Client and implements component.Component for lifecycle management.
type Component struct {
	client *Client
	cfg    Config
	log    *logger.Logger
}

// NewComponent creates a Redis component for use with the component registry.
func NewComponent(cfg Config, log *logger.Logger) *Component {
	return &Component{
		cfg: cfg,
		log: log.WithComponent("redis"),
	}
}

// Client returns the underlying *Client, or nil if not started.
func (c *Component) Client() *Client {
	return c.client
}

// ensure Component satisfies component.Component
var _ component.Component = (*Component)(nil)

// Name returns the component name.
func (c *Component) Name() string { return "redis" }

// Start initializes the Redis client and verifies connectivity.
func (c *Component) Start(ctx context.Context) error {
	client, err := New(c.cfg, c.log)
	if err != nil {
		return fmt.Errorf("redis start: %w", err)
	}

	if err := client.Ping(ctx); err != nil {
		_ = client.Close()
		return fmt.Errorf("redis start ping: %w", err)
	}

	c.client = client
	c.log.Info("Redis component started")
	return nil
}

// Stop gracefully closes the Redis connection.
func (c *Component) Stop(_ context.Context) error {
	if c.client == nil {
		return nil
	}
	c.log.Info("Redis component stopping")
	return c.client.Close()
}

// Health returns the current health status of the Redis connection.
func (c *Component) Health(ctx context.Context) component.ComponentHealth {
	if c.client == nil {
		return component.ComponentHealth{
			Name:    c.Name(),
			Status:  component.StatusUnhealthy,
			Message: "redis not initialized",
		}
	}

	if err := c.client.Ping(ctx); err != nil {
		return component.ComponentHealth{
			Name:    c.Name(),
			Status:  component.StatusUnhealthy,
			Message: fmt.Sprintf("ping failed: %v", err),
		}
	}

	return component.ComponentHealth{
		Name:   c.Name(),
		Status: component.StatusHealthy,
	}
}

// Describe returns infrastructure summary info for the bootstrap display.
func (c *Component) Describe() component.Description {
	return component.Description{
		Name:    "Redis",
		Type:    "redis",
		Details: fmt.Sprintf("%s db=%d pool=%d", c.cfg.Addr, c.cfg.DB, c.cfg.PoolSize),
	}
}