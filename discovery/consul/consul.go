package consul

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/hashicorp/consul/api"

	"github.com/kbukum/gokit/discovery"
	"github.com/kbukum/gokit/logger"
)

// Provider implements both discovery.Registry and discovery.Discovery using HashiCorp Consul.
type Provider struct {
	mu     sync.RWMutex
	client *api.Client
	cfg    discovery.Config
	consul *Config
	log    *logger.Logger
	stats  discovery.RegistryStats
}

func init() {
	discovery.RegisterProviderFactory("consul", func(cfg discovery.Config, providerCfg any, log *logger.Logger) (discovery.Registry, discovery.Discovery, error) {
		consulCfg, ok := providerCfg.(*Config)
		if !ok {
			return nil, nil, fmt.Errorf("consul provider requires *consul.Config, got %T", providerCfg)
		}
		p, err := NewProvider(cfg, consulCfg, log)
		if err != nil {
			return nil, nil, err
		}
		return p, p, nil
	})
}

// NewProvider creates a Provider from the given Config.
func NewProvider(cfg discovery.Config, consulCfg *Config, log *logger.Logger) (*Provider, error) {
	consulCfg.ApplyDefaults()
	if err := consulCfg.Validate(); err != nil {
		return nil, fmt.Errorf("consul config: %w", err)
	}

	apiCfg := api.DefaultConfig()
	apiCfg.Address = consulCfg.Address
	apiCfg.Scheme = consulCfg.Scheme
	apiCfg.Token = consulCfg.Token
	if consulCfg.Datacenter != "" {
		apiCfg.Datacenter = consulCfg.Datacenter
	}

	// Apply TLS settings
	if consulCfg.TLS != nil && consulCfg.TLS.IsEnabled() {
		apiCfg.TLSConfig = api.TLSConfig{
			Address:            consulCfg.TLS.ServerName,
			CAFile:             consulCfg.TLS.CAFile,
			CAPath:             consulCfg.TLSCAPath,
			CertFile:           consulCfg.TLS.CertFile,
			KeyFile:            consulCfg.TLS.KeyFile,
			InsecureSkipVerify: consulCfg.TLS.SkipVerify,
		}
	}

	// Apply transport/pool settings
	if consulCfg.Pool != nil {
		transport := apiCfg.Transport
		if transport == nil {
			transport = api.DefaultConfig().Transport
		}
		if transport != nil {
			transport.MaxIdleConns = consulCfg.Pool.MaxIdleConns
			transport.MaxIdleConnsPerHost = consulCfg.Pool.MaxIdleConnsPerHost
			transport.MaxConnsPerHost = consulCfg.Pool.MaxConnsPerHost
			transport.IdleConnTimeout = consulCfg.Pool.IdleConnTimeout
			apiCfg.Transport = transport
		}
	}

	client, err := api.NewClient(apiCfg)
	if err != nil {
		return nil, fmt.Errorf("consul client: %w", err)
	}

	return &Provider{
		client: client,
		cfg:    cfg,
		consul: consulCfg,
		log:    log,
	}, nil
}

// --- Registry implementation ---

// Register registers a service instance with Consul.
func (c *Provider) Register(ctx context.Context, service *discovery.ServiceInfo) error {
	reg := &api.AgentServiceRegistration{
		ID:      service.ID,
		Name:    service.Name,
		Address: service.Address,
		Port:    service.Port,
		Tags:    service.Tags,
		Meta:    service.Metadata,
	}

	reg.Check = c.buildHealthCheck(service)

	if err := c.client.Agent().ServiceRegister(reg); err != nil {
		c.log.Error("failed to register service", map[string]interface{}{
			"service_id": service.ID, "error": err.Error(),
		})
		return fmt.Errorf("consul register %q: %w", service.Name, err)
	}

	c.mu.Lock()
	c.stats.RegisteredServices++
	c.stats.LastHeartbeat = time.Now()
	c.mu.Unlock()

	c.log.Info("service registered", map[string]interface{}{
		"service_id": service.ID, "address": service.Address, "port": service.Port,
	})
	return nil
}

// Deregister removes a service instance from Consul.
func (c *Provider) Deregister(ctx context.Context, serviceID string) error {
	if err := c.client.Agent().ServiceDeregister(serviceID); err != nil {
		return fmt.Errorf("consul deregister %q: %w", serviceID, err)
	}

	c.mu.Lock()
	if c.stats.RegisteredServices > 0 {
		c.stats.RegisteredServices--
	}
	c.mu.Unlock()

	c.log.Info("service deregistered", map[string]interface{}{"service_id": serviceID})
	return nil
}

// UpdateHealth updates the health status for a TTL-based check.
// For HTTP/gRPC/TCP checks, this is a no-op since Consul polls them directly.
func (c *Provider) UpdateHealth(_ context.Context, serviceID string, healthy bool, note string) error {
	checkID := "service:" + serviceID
	if healthy {
		return c.client.Agent().PassTTL(checkID, note)
	}
	return c.client.Agent().FailTTL(checkID, note)
}

// --- Discovery implementation ---

// Discover queries Consul for healthy instances of the named service.
func (c *Provider) Discover(ctx context.Context, serviceName string) ([]discovery.ServiceInstance, error) {
	entries, _, err := c.client.Health().Service(serviceName, "", true, nil)
	if err != nil {
		return nil, fmt.Errorf("consul discover %q: %w", serviceName, err)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("%w: %s", discovery.ErrNoHealthyEndpoints, serviceName)
	}

	now := time.Now()
	instances := make([]discovery.ServiceInstance, 0, len(entries))
	for _, e := range entries {
		instances = append(instances, serviceEntryToInstance(e, now))
	}
	return instances, nil
}

// Watch returns a channel that emits updated instances whenever membership changes.
func (c *Provider) Watch(ctx context.Context, serviceName string) (<-chan []discovery.ServiceInstance, error) {
	ch := make(chan []discovery.ServiceInstance, 1)

	go func() {
		defer close(ch)
		var lastIndex uint64
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			opts := &api.QueryOptions{
				WaitIndex: lastIndex,
				WaitTime:  30 * time.Second,
			}
			opts = opts.WithContext(ctx)

			entries, meta, err := c.client.Health().Service(serviceName, "", true, opts)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				c.log.Warn("consul watch error", map[string]interface{}{
					"service": serviceName, "error": err.Error(),
				})
				time.Sleep(time.Second)
				continue
			}

			if meta.LastIndex == lastIndex {
				continue
			}
			lastIndex = meta.LastIndex

			now := time.Now()
			instances := make([]discovery.ServiceInstance, 0, len(entries))
			for _, e := range entries {
				instances = append(instances, serviceEntryToInstance(e, now))
			}

			select {
			case ch <- instances:
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch, nil
}

// Close is a no-op; the HTTP client does not require explicit closing.
func (c *Provider) Close() error {
	return nil
}

// Stats returns current registry statistics.
func (c *Provider) Stats() discovery.RegistryStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.stats
}

func serviceEntryToInstance(e *api.ServiceEntry, now time.Time) discovery.ServiceInstance {
	health := discovery.HealthHealthy
	for _, chk := range e.Checks {
		if chk.Status != api.HealthPassing {
			health = discovery.HealthUnhealthy
			break
		}
	}

	// Determine protocol from metadata or tags
	protocol := ""
	if meta, ok := e.Service.Meta["protocol"]; ok {
		protocol = meta
	} else {
		for _, tag := range e.Service.Tags {
			if tag == "http" || tag == "grpc" || tag == "websocket" {
				protocol = tag
				break
			}
		}
	}

	weight := 0
	if w, ok := e.Service.Meta["weight"]; ok {
		_, _ = fmt.Sscanf(w, "%d", &weight)
	}

	return discovery.ServiceInstance{
		ID:       e.Service.ID,
		Name:     e.Service.Service,
		Address:  e.Service.Address,
		Port:     e.Service.Port,
		Protocol: protocol,
		Tags:     e.Service.Tags,
		Metadata: e.Service.Meta,
		Health:   health,
		Weight:   weight,
		LastSeen: now,
	}
}

// buildHealthCheck creates the appropriate Consul health check based on config.
func (c *Provider) buildHealthCheck(service *discovery.ServiceInfo) *api.AgentServiceCheck {
	check := &api.AgentServiceCheck{
		Interval:                       c.cfg.Health.Interval,
		Timeout:                        c.cfg.Health.Timeout,
		DeregisterCriticalServiceAfter: c.cfg.Health.DeregisterAfter,
	}

	addr := fmt.Sprintf("%s:%d", service.Address, service.Port)

	switch c.cfg.Health.Type {
	case discovery.HealthCheckGRPC:
		check.GRPC = addr
		check.GRPCUseTLS = c.consul.Scheme == "https"
	case discovery.HealthCheckTCP:
		check.TCP = addr
	case discovery.HealthCheckTTL:
		check.TTL = c.cfg.Health.Interval
		// Remove interval/timeout for TTL checks (Consul doesn't poll)
		check.Interval = ""
		check.Timeout = ""
	default: // "http"
		scheme := c.consul.Scheme
		if scheme == "" {
			scheme = "http"
		}
		check.HTTP = fmt.Sprintf("%s://%s:%d%s", scheme, service.Address, service.Port, c.cfg.Health.Path)
	}

	return check
}

// Compile-time checks.
var (
	_ discovery.Registry  = (*Provider)(nil)
	_ discovery.Discovery = (*Provider)(nil)
)
