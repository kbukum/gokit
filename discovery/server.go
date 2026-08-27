package discovery

import (
	"context"
	"fmt"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/logging"
)

// DiscoveryServer wraps a server component with service-discovery
// auto-registration. On Start the inner component starts first, then the
// instance is registered; on Stop the instance is deregistered first, then the
// inner component stops. This keeps a service registered only while it is
// actually serving.
type DiscoveryServer struct {
	name     string
	inner    component.Component
	registry Registry
	service  *ServiceInfo
	log      *logging.Logger
}

// NewDiscoveryServer wraps inner with auto-registration against registry using
// the given service instance. name identifies the wrapper component; inner,
// registry, and service must be non-nil. A nil logger falls back to the default.
func NewDiscoveryServer(name string, inner component.Component, registry Registry, service *ServiceInfo, log *logging.Logger) (*DiscoveryServer, error) {
	if inner == nil {
		return nil, fmt.Errorf("discovery server: inner component must not be nil")
	}
	if registry == nil {
		return nil, fmt.Errorf("discovery server: registry must not be nil")
	}
	if service == nil {
		return nil, fmt.Errorf("discovery server: service instance must not be nil")
	}
	if log == nil {
		log = logging.NewDefault(name)
	}
	return &DiscoveryServer{
		name:     name,
		inner:    inner,
		registry: registry,
		service:  service,
		log:      log.WithComponent(name),
	}, nil
}

var _ component.Component = (*DiscoveryServer)(nil)

// Name returns the wrapper component name.
func (s *DiscoveryServer) Name() string { return s.name }

// Inner returns the wrapped component.
func (s *DiscoveryServer) Inner() component.Component { return s.inner }

// Instance returns the service instance registered on start.
func (s *DiscoveryServer) Instance() *ServiceInfo { return s.service }

// Start starts the inner component and then registers with discovery. When
// registration fails, the inner component is stopped (best effort) and the error
// is returned, so the process never advertises a service it cannot back.
func (s *DiscoveryServer) Start(ctx context.Context) error {
	if err := s.inner.Start(ctx); err != nil {
		return fmt.Errorf("discovery server %q: start inner component: %w", s.name, err)
	}

	if err := s.registry.Register(ctx, s.service); err != nil {
		s.log.WarnCtx(ctx, "registration failed, stopping inner component", map[string]any{
			"error":      err.Error(),
			"service_id": s.service.ID,
		})
		if stopErr := s.inner.Stop(ctx); stopErr != nil {
			s.log.WarnCtx(ctx, "failed to stop inner component after registration failure", map[string]any{
				"error": stopErr.Error(),
			})
		}
		return fmt.Errorf("discovery server %q: register with discovery: %w", s.name, err)
	}

	s.log.DebugCtx(ctx, "service registered", map[string]any{"service_id": s.service.ID})
	return nil
}

// Stop deregisters from discovery (best effort) and then stops the inner
// component. A deregistration failure is logged but does not prevent shutdown.
func (s *DiscoveryServer) Stop(ctx context.Context) error {
	if err := s.registry.Deregister(ctx, s.service.ID); err != nil {
		s.log.WarnCtx(ctx, "failed to deregister on stop", map[string]any{
			"error":      err.Error(),
			"service_id": s.service.ID,
		})
	}

	if err := s.inner.Stop(ctx); err != nil {
		return fmt.Errorf("discovery server %q: stop inner component: %w", s.name, err)
	}
	return nil
}

// Health reports the inner component's health, tagged with registration status.
func (s *DiscoveryServer) Health(ctx context.Context) component.Health {
	inner := s.inner.Health(ctx)
	if inner.Status == component.StatusHealthy {
		return component.Health{Name: s.name, Status: component.StatusHealthy, Message: "registered"}
	}
	return component.Health{Name: s.name, Status: inner.Status, Message: "inner: " + inner.Message}
}
