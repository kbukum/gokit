package static

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kbukum/gokit/discovery"
	"github.com/kbukum/gokit/logger"
)

// Provider implements discovery.Registry and discovery.Discovery using an in-memory list of
// endpoints. Useful for local development and testing.
type Provider struct {
	mu        sync.RWMutex
	instances map[string][]discovery.ServiceInstance // keyed by service name
}

func init() {
	discovery.RegisterProviderFactory("static", func(cfg discovery.Config, _ any, _ *logger.Logger) (discovery.Registry, discovery.Discovery, error) {
		p := NewProvider(cfg.StaticEndpoints)
		return p, p, nil
	})

	// k8s uses the static provider as a fallback.
	discovery.RegisterProviderFactory("k8s", func(cfg discovery.Config, _ any, _ *logger.Logger) (discovery.Registry, discovery.Discovery, error) {
		p := NewProvider(cfg.StaticEndpoints)
		return p, p, nil
	})
}

// NewProvider creates a Provider pre-populated from static config.
func NewProvider(endpoints []discovery.StaticEndpoint) *Provider {
	sp := &Provider{
		instances: make(map[string][]discovery.ServiceInstance),
	}
	now := time.Now()
	for _, ep := range endpoints {
		inst := discovery.ServiceInstance{
			ID:       fmt.Sprintf("%s-%s-%d", ep.Name, ep.Address, ep.Port),
			Name:     ep.Name,
			Address:  ep.Address,
			Port:     ep.Port,
			Protocol: ep.Protocol,
			Tags:     ep.Tags,
			Metadata: ep.Metadata,
			Weight:   ep.Weight,
			Health:   discovery.HealthHealthy,
			LastSeen: now,
		}
		if inst.Weight <= 0 {
			inst.Weight = 1
		}
		if !ep.Healthy && ep.Address != "" {
			inst.Health = discovery.HealthUnhealthy
		}
		// Ensure protocol is in tags for backward compatibility
		if ep.Protocol != "" {
			if inst.Tags == nil {
				inst.Tags = []string{ep.Protocol}
			}
			if inst.Metadata == nil {
				inst.Metadata = map[string]string{"protocol": ep.Protocol}
			}
		}
		sp.instances[ep.Name] = append(sp.instances[ep.Name], inst)
	}
	return sp
}

// --- Registry implementation ---

// Register adds a service instance to the in-memory store.
func (s *Provider) Register(_ context.Context, svc *discovery.ServiceInfo) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	inst := discovery.ServiceInstance{
		ID:       svc.ID,
		Name:     svc.Name,
		Address:  svc.Address,
		Port:     svc.Port,
		Tags:     svc.Tags,
		Metadata: svc.Metadata,
		Health:   discovery.HealthHealthy,
		LastSeen: time.Now(),
	}
	s.instances[svc.Name] = append(s.instances[svc.Name], inst)
	return nil
}

// Deregister removes a service instance by ID.
func (s *Provider) Deregister(_ context.Context, serviceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for name, list := range s.instances {
		for i, inst := range list {
			if inst.ID == serviceID {
				s.instances[name] = append(list[:i], list[i+1:]...)
				return nil
			}
		}
	}
	return nil
}

// UpdateHealth is a no-op for the static provider.
func (s *Provider) UpdateHealth(_ context.Context, _ string, _ bool, _ string) error {
	return nil
}

// Stats returns empty stats for the static provider.
func (s *Provider) Stats() discovery.RegistryStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	count := 0
	for _, v := range s.instances {
		count += len(v)
	}
	return discovery.RegistryStats{RegisteredServices: count}
}

// --- Discovery implementation ---

// Discover returns the currently registered instances for the named service.
func (s *Provider) Discover(_ context.Context, serviceName string) ([]discovery.ServiceInstance, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	instances, ok := s.instances[serviceName]
	if !ok || len(instances) == 0 {
		return nil, fmt.Errorf("%w: %s", discovery.ErrServiceNotFound, serviceName)
	}

	out := make([]discovery.ServiceInstance, len(instances))
	now := time.Now()
	for i, inst := range instances {
		inst.LastSeen = now
		out[i] = inst
	}
	return out, nil
}

// Watch returns a channel that never emits for the static provider (instances are fixed).
// The channel is closed when the context is cancelled.
func (s *Provider) Watch(ctx context.Context, _ string) (<-chan []discovery.ServiceInstance, error) {
	ch := make(chan []discovery.ServiceInstance)
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch, nil
}

// Close is a no-op for the static provider.
func (s *Provider) Close() error {
	return nil
}

// Compile-time checks.
var (
	_ discovery.Registry  = (*Provider)(nil)
	_ discovery.Discovery = (*Provider)(nil)
)
