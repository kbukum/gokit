package storage

import (
	"context"
	"fmt"

	"github.com/skillsenselab/gokit/component"
	"github.com/skillsenselab/gokit/logger"
)

// Component wraps Storage and implements component.Component for lifecycle management.
type Component struct {
	storage Storage
	cfg     Config
	log     *logger.Logger
}

// NewComponent creates a storage component for use with the component registry.
func NewComponent(cfg Config, log *logger.Logger) *Component {
	return &Component{
		cfg: cfg,
		log: log.WithComponent("storage"),
	}
}

// Storage returns the underlying Storage, or nil if not started.
func (c *Component) Storage() Storage {
	return c.storage
}

// ensure Component satisfies component.Component.
var _ component.Component = (*Component)(nil)

// Name returns the component name.
func (c *Component) Name() string { return "storage" }

// Start initializes the storage backend.
func (c *Component) Start(_ context.Context) error {
	if !c.cfg.Enabled {
		c.log.Info("storage component is disabled")
		return nil
	}

	s, err := New(c.cfg, c.log)
	if err != nil {
		return fmt.Errorf("storage start: %w", err)
	}
	c.storage = s
	return nil
}

// Stop gracefully shuts down the storage component.
func (c *Component) Stop(_ context.Context) error {
	c.storage = nil
	return nil
}

// Health returns the current health status of the storage component.
func (c *Component) Health(ctx context.Context) component.ComponentHealth {
	if !c.cfg.Enabled {
		return component.ComponentHealth{
			Name:    c.Name(),
			Status:  component.StatusHealthy,
			Message: "disabled",
		}
	}

	if c.storage == nil {
		return component.ComponentHealth{
			Name:    c.Name(),
			Status:  component.StatusUnhealthy,
			Message: "storage not initialized",
		}
	}

	// Simple health probe: check that we can resolve a URL.
	if _, err := c.storage.URL(ctx, ".health"); err != nil {
		return component.ComponentHealth{
			Name:    c.Name(),
			Status:  component.StatusUnhealthy,
			Message: fmt.Sprintf("health probe failed: %v", err),
		}
	}

	return component.ComponentHealth{
		Name:   c.Name(),
		Status: component.StatusHealthy,
	}
}

// Describe returns infrastructure summary info for the bootstrap display.
func (c *Component) Describe() component.Description {
	details := fmt.Sprintf("provider=%s", c.cfg.Provider)
	if c.cfg.Bucket != "" {
		details += fmt.Sprintf(" bucket=%s", c.cfg.Bucket)
	} else if c.cfg.BasePath != "" {
		details += fmt.Sprintf(" path=%s", c.cfg.BasePath)
	}
	return component.Description{
		Name:    "Storage",
		Type:    "storage",
		Details: details,
	}
}