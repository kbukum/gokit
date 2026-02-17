package storage

import (
	"context"
	"fmt"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/logger"
)

// Component wraps Storage and implements component.Component for lifecycle management.
type Component struct {
	storage     Storage
	cfg         Config
	providerCfg any
	log         *logger.Logger
}

// NewComponent creates a storage component for use with the component registry.
func NewComponent(cfg Config, providerCfg any, log *logger.Logger) *Component {
	return &Component{
		cfg:         cfg,
		providerCfg: providerCfg,
		log:         log.WithComponent("storage"),
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

	s, err := New(c.cfg, c.providerCfg, c.log)
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
func (c *Component) Health(ctx context.Context) component.Health {
	if !c.cfg.Enabled {
		return component.Health{
			Name:    c.Name(),
			Status:  component.StatusHealthy,
			Message: "disabled",
		}
	}

	if c.storage == nil {
		return component.Health{
			Name:    c.Name(),
			Status:  component.StatusUnhealthy,
			Message: "storage not initialized",
		}
	}

	// Simple health probe: check that we can resolve a URL.
	if _, err := c.storage.URL(ctx, ".health"); err != nil {
		return component.Health{
			Name:    c.Name(),
			Status:  component.StatusUnhealthy,
			Message: fmt.Sprintf("health probe failed: %v", err),
		}
	}

	return component.Health{
		Name:   c.Name(),
		Status: component.StatusHealthy,
	}
}

// Describe returns infrastructure summary info for the bootstrap display.
func (c *Component) Describe() component.Description {
	details := fmt.Sprintf("provider=%s", c.cfg.Provider)

	// Try to extract bucket from provider config via BucketDescriber interface.
	if bp, ok := c.providerCfg.(BucketDescriber); ok {
		if b := bp.GetBucket(); b != "" {
			details += fmt.Sprintf(" bucket=%s", b)
		}
	}

	return component.Description{
		Name:    "Storage",
		Type:    "storage",
		Details: details,
	}
}

// BucketDescriber is optionally implemented by provider configs that use a bucket.
type BucketDescriber interface {
	GetBucket() string
}
