package cache

import (
	"context"
	"fmt"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/logging"
)

// Component wraps Store and implements component.Component.
type Component struct {
	store       Store
	registry    *FactoryRegistry
	cfg         Config
	providerCfg any
	log         *logging.Logger
}

// NewComponent creates a cache component. The registry is explicit
// and must contain the selected provider.
func NewComponent(registry *FactoryRegistry, cfg Config, providerCfg any, log *logging.Logger) *Component {
	return &Component{
		registry:    registry,
		cfg:         cfg,
		providerCfg: providerCfg,
		log:         log.WithComponent("cache"),
	}
}

// Store returns the started cache store, if any.
func (c *Component) Store() Store {
	return c.store
}

func (c *Component) Name() string { return "cache" }

func (c *Component) Start(_ context.Context) error {
	if !c.cfg.Enabled {
		c.log.Info("cache component is disabled") //nolint:contextcheck // lifecycle Start has no request context
		return nil
	}
	store, err := New(c.registry, c.cfg, c.providerCfg, c.log) //nolint:contextcheck // cache construction has no request context
	if err != nil {
		return fmt.Errorf("cache start: %w", err)
	}
	c.store = store
	return nil
}

func (c *Component) Stop(_ context.Context) error {
	if closer, ok := c.store.(CloseStore); ok {
		if err := closer.Close(); err != nil {
			return err
		}
	}
	c.store = nil
	return nil
}

func (c *Component) Health(ctx context.Context) component.Health {
	if !c.cfg.Enabled {
		return component.Health{Name: c.Name(), Status: component.StatusHealthy, Message: "disabled"}
	}
	if c.store == nil {
		return component.Health{Name: c.Name(), Status: component.StatusUnhealthy, Message: "cache not initialized"}
	}
	if _, err := c.store.Exists(ctx, ".health"); err != nil {
		return component.Health{Name: c.Name(), Status: component.StatusUnhealthy, Message: fmt.Sprintf("health probe failed: %v", err)}
	}
	return component.Health{Name: c.Name(), Status: component.StatusHealthy}
}

func (c *Component) Describe() component.Description {
	return component.Description{Name: "Cache", Type: "cache", Details: fmt.Sprintf("provider=%s", c.cfg.Provider)}
}

var _ component.Component = (*Component)(nil)
