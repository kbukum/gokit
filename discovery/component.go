package discovery

import (
	"context"
	"fmt"
	"net"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/logger"
)

// ProviderFactory creates a Registry and Discovery pair from a Config.
// providerCfg holds provider-specific configuration (e.g., *consul.Config).
// Providers should type-assert providerCfg to their own config type.
type ProviderFactory func(cfg Config, providerCfg any, log *logger.Logger) (Registry, Discovery, error)

var providerFactories = make(map[string]ProviderFactory)

// RegisterProviderFactory registers a discovery backend factory for the given
// provider name. Implementation packages call this (typically in an init
// function) to make themselves available to the Component.
func RegisterProviderFactory(name string, f ProviderFactory) {
	providerFactories[name] = f
}

// Component wraps a Registry and Discovery pair and implements
// component.Component for lifecycle management.
type Component struct {
	registry    Registry
	discovery   Discovery
	client      *Client
	cfg         Config
	providerCfg any
	log         *logger.Logger
}

// NewComponent creates a discovery Component for use with the component registry.
// providerCfg holds provider-specific configuration (e.g., *consul.Config for Consul).
func NewComponent(cfg Config, providerCfg any, log *logger.Logger) *Component {
	return &Component{
		cfg:         cfg,
		providerCfg: providerCfg,
		log:         log.WithComponent("discovery"),
	}
}

// ensure Component satisfies component.Component.
var _ component.Component = (*Component)(nil)

// Name returns the component name.
func (c *Component) Name() string { return "discovery" }

// Registry returns the underlying Registry, or nil if not started.
func (c *Component) Registry() Registry { return c.registry }

// Discovery returns the underlying Discovery, or nil if not started.
func (c *Component) Discovery() Discovery { return c.discovery }

// Client returns the high-level discovery Client (with caching and LB),
// auto-created from Config after Start. Returns nil if not started.
func (c *Component) Client() *Client { return c.client }

// Start initialises the appropriate provider and registers the local service.
func (c *Component) Start(ctx context.Context) error {
	c.cfg.ApplyDefaults()

	if !c.cfg.Enabled {
		c.log.Info("discovery disabled, using static provider")
		f, ok := providerFactories["static"]
		if !ok {
			return fmt.Errorf("discovery: static provider not registered")
		}
		reg, disc, err := f(c.cfg, c.providerCfg, c.log)
		if err != nil {
			return fmt.Errorf("discovery start: %w", err)
		}
		c.registry = reg
		c.discovery = disc
		c.client = c.buildClient()
		return nil
	}

	if err := c.cfg.Validate(); err != nil {
		return fmt.Errorf("discovery config: %w", err)
	}

	f, ok := providerFactories[c.cfg.Provider]
	if !ok {
		return fmt.Errorf("unsupported discovery provider %q (not registered)", c.cfg.Provider)
	}

	reg, disc, err := f(c.cfg, c.providerCfg, c.log)
	if err != nil {
		return fmt.Errorf("discovery start: %w", err)
	}
	c.registry = reg
	c.discovery = disc

	// Auto-register self for providers that support it.
	if c.cfg.Provider == "consul" {
		addr := c.cfg.Registration.ServiceAddress
		if addr == "" {
			ip, err := getLocalIP()
			if err != nil {
				return fmt.Errorf("discovery: resolve local IP: %w", err)
			}
			addr = ip
		}

		svc := &ServiceInfo{
			ID:       c.cfg.Registration.ServiceID,
			Name:     c.cfg.Registration.ServiceName,
			Address:  addr,
			Port:     c.cfg.Registration.ServicePort,
			Tags:     c.cfg.Registration.Tags,
			Metadata: c.cfg.Registration.Metadata,
		}
		if err := c.registry.Register(ctx, svc); err != nil {
			return fmt.Errorf("discovery: register self: %w", err)
		}
	}

	c.client = c.buildClient()

	c.log.Info("discovery component started", map[string]interface{}{
		"provider": c.cfg.Provider,
	})
	return nil
}

// Stop deregisters the local service and releases resources.
func (c *Component) Stop(ctx context.Context) error {
	c.log.Info("discovery component stopping")

	if c.registry != nil && c.cfg.Enabled && c.cfg.Registration.ServiceID != "" {
		if err := c.registry.Deregister(ctx, c.cfg.Registration.ServiceID); err != nil {
			c.log.Warn("failed to deregister on stop", map[string]interface{}{
				"error": err.Error(),
			})
		}
	}

	if c.discovery != nil {
		return c.discovery.Close()
	}
	return nil
}

// Health returns the current health status of the discovery component.
func (c *Component) Health(ctx context.Context) component.Health {
	if c.discovery == nil {
		return component.Health{
			Name:    c.Name(),
			Status:  component.StatusUnhealthy,
			Message: "discovery not initialized",
		}
	}

	if !c.cfg.Enabled {
		return component.Health{
			Name:    c.Name(),
			Status:  component.StatusHealthy,
			Message: "disabled (static)",
		}
	}

	// Verify registration succeeded via local stats (avoids Consul health-check
	// propagation race where the service is registered but Consul hasn't yet
	// polled the /health endpoint, making self-discover fail).
	if c.registry != nil {
		stats := c.registry.Stats()
		if stats.RegisteredServices > 0 {
			return component.Health{
				Name:   c.Name(),
				Status: component.StatusHealthy,
			}
		}
	}

	return component.Health{
		Name:    c.Name(),
		Status:  component.StatusDegraded,
		Message: "no services registered",
	}
}

// Describe returns infrastructure summary info for the bootstrap display.
func (c *Component) Describe() component.Description {
	details := fmt.Sprintf("provider=%s service=%s", c.cfg.Provider, c.cfg.Registration.ServiceName)
	return component.Description{
		Name:    "Discovery",
		Type:    "discovery",
		Details: details,
		Port:    c.cfg.Registration.ServicePort,
	}
}

func getLocalIP() (string, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "", err
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String(), nil
}

// buildClient derives a high-level Client from the component's Config and Discovery backend.
func (c *Component) buildClient() *Client {
	if c.discovery == nil {
		return nil
	}
	return NewClient(c.discovery, c.cfg.BuildClientConfig(), c.log)
}
