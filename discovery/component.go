package discovery

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/logger"
)

// ProviderFactory creates a Registry and Discovery pair from a Config.
// The factory reads generic connection fields (Addr, Scheme, Token) directly
// from Config, and exotic provider-specific settings from Config.ProviderOptions.
type ProviderFactory func(cfg Config, log *logger.Logger) (Registry, Discovery, error)

// ProviderRegistry stores discovery provider factories by name.
type ProviderRegistry struct {
	mu        sync.RWMutex
	factories map[string]ProviderFactory
}

// NewProviderRegistry creates an isolated discovery provider registry.
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{factories: make(map[string]ProviderFactory)}
}

// Register stores a discovery provider factory.
// It panics for invalid input or duplicate names.
func (r *ProviderRegistry) Register(name string, f ProviderFactory) {
	if name == "" {
		panic("discovery: provider name cannot be empty")
	}
	if f == nil {
		panic(fmt.Sprintf("discovery: factory %q cannot be nil", name))
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.factories[name]; exists {
		panic(fmt.Sprintf("discovery: provider factory %q already registered", name))
	}
	r.factories[name] = f
}

// Get returns a discovery provider factory by name.
func (r *ProviderRegistry) Get(name string) (ProviderFactory, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	f, ok := r.factories[name]
	return f, ok
}

// Component wraps a Registry and Discovery pair and implements
// component.Component for lifecycle management.
type Component struct {
	registry      Registry
	discovery     Discovery
	client        *Client
	cfg           Config
	log           *logger.Logger
	providers     *ProviderRegistry
	localIPFinder func(probeTarget string) (string, error)
	ipProbeTarget string
}

// ComponentOption configures discovery component behavior.
type ComponentOption func(*Component)

// WithIPProbeTarget configures fallback UDP probe target for local IP detection.
// When not set (or empty), UDP probing is disabled and only interface enumeration
// is used to determine the local IP address.
func WithIPProbeTarget(addr string) ComponentOption {
	return func(c *Component) {
		if addr != "" {
			c.ipProbeTarget = addr
		}
	}
}

// NewComponent creates a discovery Component for use with the component registry.
// registry must not be nil; panics if it is.
func NewComponent(registry *ProviderRegistry, cfg Config, log *logger.Logger, opts ...ComponentOption) *Component {
	if registry == nil {
		panic("discovery: provider registry must not be nil")
	}
	c := &Component{
		cfg:           cfg,
		log:           log.WithComponent("discovery"),
		providers:     registry,
		localIPFinder: defaultLocalIPFinder,
		ipProbeTarget: "",
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
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

// Start initializes the appropriate provider and registers the local service.
func (c *Component) Start(ctx context.Context) error {
	c.cfg.ApplyDefaults()

	if !c.cfg.Enabled {
		c.log.Info("discovery disabled, using static provider")
		f, ok := c.providers.Get("static")
		if !ok {
			return fmt.Errorf("discovery: static provider not registered")
		}
		reg, disc, err := f(c.cfg, c.log)
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

	f, ok := c.providers.Get(c.cfg.Provider)
	if !ok {
		return fmt.Errorf("unsupported discovery provider %q (not registered)", c.cfg.Provider)
	}

	reg, disc, err := f(c.cfg, c.log)
	if err != nil {
		return fmt.Errorf("discovery start: %w", err)
	}
	c.registry = reg
	c.discovery = disc

	// Auto-register self when registration is enabled.
	if c.cfg.Registration.Enabled {
		addr := c.cfg.Registration.ServiceAddress
		if addr == "" {
			ip, err := c.localIPFinder(c.ipProbeTarget)
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

		if err := c.registerWithRetry(ctx, svc); err != nil {
			if c.cfg.Registration.Required {
				return fmt.Errorf("discovery: register self: %w", err)
			}
			c.log.Warn("failed to register with discovery — continuing in degraded mode", map[string]interface{}{
				"error":      err.Error(),
				"service_id": svc.ID,
			})
		}
	}

	c.client = c.buildClient()

	c.log.Debug("discovery component started", map[string]interface{}{"provider": c.cfg.Provider})
	return nil
}

// Stop deregisters the local service and releases resources.
func (c *Component) Stop(ctx context.Context) error {
	c.log.Debug("discovery component stopping")

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

type networkResolver interface {
	Interfaces() ([]net.Interface, error)
	InterfaceAddrs(net.Interface) ([]net.Addr, error)
	Dial(network, address string) (net.Conn, error)
}

type stdNetworkResolver struct{}

func (stdNetworkResolver) Interfaces() ([]net.Interface, error) { return net.Interfaces() }
func (stdNetworkResolver) InterfaceAddrs(iface net.Interface) ([]net.Addr, error) {
	return iface.Addrs()
}
func (stdNetworkResolver) Dial(network, address string) (net.Conn, error) {
	dialer := &net.Dialer{}
	return dialer.Dial(network, address)
}

func defaultLocalIPFinder(probeTarget string) (string, error) {
	return resolveLocalIPv4(stdNetworkResolver{}, probeTarget)
}

func resolveLocalIPv4(resolver networkResolver, probeTarget string) (string, error) {
	ip, err := localIPv4FromInterfaces(resolver)
	if err == nil {
		return ip, nil
	}
	if probeTarget == "" {
		return "", fmt.Errorf("discovery: failed to detect local IP via interfaces and probe target not configured: %w", err)
	}
	return localIPFromUDPProbe(resolver, probeTarget)
}

func localIPv4FromInterfaces(resolver networkResolver) (string, error) {
	ifaces, err := resolver.Interfaces()
	if err != nil {
		return "", err
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := resolver.InterfaceAddrs(iface)
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil {
				continue
			}
			v4 := ip.To4()
			if v4 == nil || v4.IsLoopback() || v4.IsLinkLocalUnicast() {
				continue
			}
			return v4.String(), nil
		}
	}
	return "", fmt.Errorf("no suitable non-loopback IPv4 interface address found")
}

func localIPFromUDPProbe(resolver networkResolver, probeTarget string) (string, error) {
	conn, err := resolver.Dial("udp", probeTarget)
	if err != nil {
		return "", err
	}
	defer conn.Close() //nolint:errcheck
	udpAddr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		return "", fmt.Errorf("discovery: unexpected local address type %T", conn.LocalAddr())
	}
	return udpAddr.IP.String(), nil
}

// registerWithRetry attempts registration with exponential backoff.
// Returns nil on success, or the last error after all retries are exhausted.
func (c *Component) registerWithRetry(ctx context.Context, svc *ServiceInfo) error {
	maxRetries := c.cfg.Registration.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}
	interval := ParseDuration(c.cfg.Registration.RetryInterval)
	if interval <= 0 {
		interval = 2 * time.Second
	}

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if err := c.registry.Register(ctx, svc); err != nil {
			lastErr = err
			c.log.Warn("failed to register service", map[string]interface{}{
				"error":      err.Error(),
				"service_id": svc.ID,
				"attempt":    attempt,
				"max":        maxRetries,
			})
			if attempt < maxRetries {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(interval):
				}
				interval *= 2
			}
			continue
		}
		return nil
	}
	return lastErr
}

// buildClient derives a high-level Client from the component's Config and Discovery backend.
func (c *Component) buildClient() *Client {
	if c.discovery == nil {
		return nil
	}
	return NewClient(c.discovery, c.cfg.BuildClientConfig(), c.log)
}
