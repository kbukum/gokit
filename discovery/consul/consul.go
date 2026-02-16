package consul

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/hashicorp/consul/api"

	"github.com/skillsenselab/gokit/discovery"
	"github.com/skillsenselab/gokit/logger"
)

// Provider implements both discovery.Registry and discovery.Discovery using HashiCorp Consul.
type Provider struct {
	mu     sync.RWMutex
	client *api.Client
	cfg    discovery.Config
	log    *logger.Logger
	stats  discovery.RegistryStats
}

func init() {
	discovery.RegisterProviderFactory("consul", func(cfg discovery.Config, log *logger.Logger) (discovery.Registry, discovery.Discovery, error) {
		p, err := NewProvider(cfg, log)
		if err != nil {
			return nil, nil, err
		}
		return p, p, nil
	})
}

// NewProvider creates a Provider from the given Config.
func NewProvider(cfg discovery.Config, log *logger.Logger) (*Provider, error) {
	apiCfg := api.DefaultConfig()
	apiCfg.Address = cfg.ConsulAddr
	apiCfg.Scheme = cfg.ConsulScheme
	apiCfg.Token = cfg.ConsulToken
	if cfg.ConsulDatacenter != "" {
		apiCfg.Datacenter = cfg.ConsulDatacenter
	}

	client, err := api.NewClient(apiCfg)
	if err != nil {
		return nil, fmt.Errorf("consul client: %w", err)
	}

	return &Provider{
		client: client,
		cfg:    cfg,
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

	if c.cfg.HealthCheckPath != "" {
		scheme := c.cfg.ConsulScheme
		if scheme == "" {
			scheme = "http"
		}
		reg.Check = &api.AgentServiceCheck{
			HTTP:                           fmt.Sprintf("%s://%s:%d%s", scheme, service.Address, service.Port, c.cfg.HealthCheckPath),
			Interval:                       c.cfg.HealthCheckInterval.String(),
			Timeout:                        c.cfg.HealthCheckTimeout.String(),
			DeregisterCriticalServiceAfter: c.cfg.DeregisterAfter.String(),
		}
	}

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
		fmt.Sscanf(w, "%d", &weight)
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

// Compile-time checks.
var (
	_ discovery.Registry  = (*Provider)(nil)
	_ discovery.Discovery = (*Provider)(nil)
)
