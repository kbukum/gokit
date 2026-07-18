package storage

import (
	"context"
	"fmt"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/logging"
)

// Component wraps Storage and implements component.Component for lifecycle management.
type Component struct {
	storage  Storage
	registry *FactoryRegistry
	cfg      Config
	log      *logging.Logger
}

// NewComponent creates a storage component for use with the component registry. registry is mandatory; construct one and register the desired provider(s) before passing it.
func NewComponent(registry *FactoryRegistry, cfg Config, log *logging.Logger) *Component {
	if log == nil {
		log = logging.NewDefault("storage") //nolint:contextcheck // default logger construction has no request-scoped operation
	}
	return &Component{
		registry: registry,
		cfg:      cfg,
		log:      log.WithComponent("storage"),
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
		c.log.Info("storage component is disabled") //nolint:contextcheck // lifecycle Start has no request context
		return nil
	}

	s, err := New(c.registry, c.cfg, c.log) //nolint:contextcheck // storage init has no request context
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
