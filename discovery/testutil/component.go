package testutil

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/discovery"
	"github.com/kbukum/gokit/testutil"
)

// Component is a test discovery component backed by in-memory maps.
// It implements component.Component, testutil.TestComponent,
// discovery.Registry, and discovery.Discovery.
type Component struct {
	services  map[string]*discovery.ServiceInfo
	instances map[string][]discovery.ServiceInstance
	started   bool
	mu        sync.RWMutex
}

var _ component.Component = (*Component)(nil)
var _ testutil.TestComponent = (*Component)(nil)
var _ discovery.Registry = (*Component)(nil)
var _ discovery.Discovery = (*Component)(nil)

// NewComponent creates a new in-memory discovery test component.
func NewComponent() *Component {
	return &Component{}
}

// AddInstance pre-populates a service instance for discovery.
// Call before or after Start.
func (c *Component) AddInstance(serviceName string, inst discovery.ServiceInstance) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.instances == nil {
		c.instances = make(map[string][]discovery.ServiceInstance)
	}
	c.instances[serviceName] = append(c.instances[serviceName], inst)
}

// Registry returns the component itself as a discovery.Registry.
func (c *Component) Registry() discovery.Registry { return c }

// Discovery returns the component itself as a discovery.Discovery.
func (c *Component) DiscoveryClient() discovery.Discovery { return c }

// --- component.Component ---

func (c *Component) Name() string { return "discovery-test" }

func (c *Component) Start(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.started {
		return fmt.Errorf("component already started")
	}
	if c.services == nil {
		c.services = make(map[string]*discovery.ServiceInfo)
	}
	if c.instances == nil {
		c.instances = make(map[string][]discovery.ServiceInstance)
	}
	c.started = true
	return nil
}

func (c *Component) Stop(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.started = false
	return nil
}

func (c *Component) Health(_ context.Context) component.Health {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.started {
		return component.Health{Name: c.Name(), Status: component.StatusUnhealthy, Message: "not started"}
	}
	return component.Health{Name: c.Name(), Status: component.StatusHealthy}
}

// --- testutil.TestComponent ---

func (c *Component) Reset(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.started {
		return fmt.Errorf("component not started")
	}
	c.services = make(map[string]*discovery.ServiceInfo)
	c.instances = make(map[string][]discovery.ServiceInstance)
	return nil
}

func (c *Component) Snapshot(_ context.Context) (interface{}, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.started {
		return nil, fmt.Errorf("component not started")
	}
	snap := &snapshot{
		services:  make(map[string]*discovery.ServiceInfo, len(c.services)),
		instances: make(map[string][]discovery.ServiceInstance, len(c.instances)),
	}
	for k, v := range c.services {
		cp := *v
		snap.services[k] = &cp
	}
	for k, v := range c.instances {
		cp := make([]discovery.ServiceInstance, len(v))
		copy(cp, v)
		snap.instances[k] = cp
	}
	return snap, nil
}

func (c *Component) Restore(_ context.Context, s interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.started {
		return fmt.Errorf("component not started")
	}
	snap, ok := s.(*snapshot)
	if !ok {
		return fmt.Errorf("invalid snapshot type: expected *snapshot, got %T", s)
	}
	c.services = snap.services
	c.instances = snap.instances
	return nil
}

type snapshot struct {
	services  map[string]*discovery.ServiceInfo
	instances map[string][]discovery.ServiceInstance
}

// --- discovery.Registry ---

func (c *Component) Register(_ context.Context, svc *discovery.ServiceInfo) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.services[svc.ID] = svc
	return nil
}

func (c *Component) Deregister(_ context.Context, serviceID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.services, serviceID)
	return nil
}

func (c *Component) UpdateHealth(_ context.Context, _ string, _ bool, _ string) error {
	return nil
}

func (c *Component) Stats() discovery.RegistryStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return discovery.RegistryStats{
		RegisteredServices: len(c.services),
		LastHeartbeat:      time.Now(),
	}
}

// --- discovery.Discovery ---

func (c *Component) Discover(_ context.Context, serviceName string) ([]discovery.ServiceInstance, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	instances, ok := c.instances[serviceName]
	if !ok || len(instances) == 0 {
		return nil, discovery.ErrServiceNotFound
	}
	// Return only healthy instances
	var healthy []discovery.ServiceInstance
	for i := range instances {
		if instances[i].Health == discovery.HealthHealthy || instances[i].Health == "" {
			healthy = append(healthy, instances[i])
		}
	}
	if len(healthy) == 0 {
		return nil, discovery.ErrNoHealthyEndpoints
	}
	return healthy, nil
}

func (c *Component) Watch(ctx context.Context, serviceName string) (<-chan []discovery.ServiceInstance, error) {
	ch := make(chan []discovery.ServiceInstance, 1)
	instances, _ := c.Discover(ctx, serviceName)
	if instances != nil {
		ch <- instances
	}
	// Static mock: close channel immediately (no live updates)
	close(ch)
	return ch, nil
}

func (c *Component) Close() error { return nil }
