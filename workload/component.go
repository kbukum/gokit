package workload

import (
	"context"
	"fmt"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/logger"
)

// Component wraps Manager and implements component.Component for lifecycle management.
type Component struct {
	manager     Manager
	cfg         Config
	providerCfg any
	log         *logger.Logger
}

// NewComponent creates a workload component for use with the component registry.
func NewComponent(cfg Config, providerCfg any, log *logger.Logger) *Component {
	return &Component{
		cfg:         cfg,
		providerCfg: providerCfg,
		log:         log.WithComponent("workload"),
	}
}

// Manager returns the underlying Manager, or nil if not started.
func (c *Component) Manager() Manager {
	return c.manager
}

var _ component.Component = (*Component)(nil)

func (c *Component) Name() string { return "workload" }

func (c *Component) Start(_ context.Context) error {
	if !c.cfg.Enabled {
		c.log.Info("workload component is disabled")
		return nil
	}
	m, err := New(c.cfg, c.providerCfg, c.log)
	if err != nil {
		return fmt.Errorf("workload start: %w", err)
	}
	c.manager = m
	return nil
}

func (c *Component) Stop(_ context.Context) error {
	c.manager = nil
	return nil
}

func (c *Component) Health(ctx context.Context) component.ComponentHealth {
	if !c.cfg.Enabled {
		return component.ComponentHealth{
			Name:    c.Name(),
			Status:  component.StatusHealthy,
			Message: "disabled",
		}
	}
	if c.manager == nil {
		return component.ComponentHealth{
			Name:    c.Name(),
			Status:  component.StatusUnhealthy,
			Message: "workload manager not initialized",
		}
	}
	if err := c.manager.HealthCheck(ctx); err != nil {
		return component.ComponentHealth{
			Name:    c.Name(),
			Status:  component.StatusUnhealthy,
			Message: fmt.Sprintf("health check failed: %v", err),
		}
	}
	return component.ComponentHealth{
		Name:   c.Name(),
		Status: component.StatusHealthy,
	}
}

func (c *Component) Describe() component.Description {
	return component.Description{
		Name:    "Workload",
		Type:    "workload",
		Details: fmt.Sprintf("provider=%s", c.cfg.Provider),
	}
}
