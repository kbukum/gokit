package server

import (
	"context"
	"fmt"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/discovery"
	"github.com/kbukum/gokit/logging"
)

// DiscoveryServerComponent wraps a Server with service discovery integration.
// It automatically registers the server on Start and deregisters on Stop.
type DiscoveryServerComponent struct {
	inner     *Component
	registry  discovery.Registry
	serviceID string
	svcInfo   *discovery.ServiceInfo
	log       *logging.Logger
}

// NewDiscoveryServerComponent creates a discovery-enabled server component.
//
// Parameters:
//   - inner: the HTTP/gRPC server component to wrap
//   - registry: the discovery registry for registration
//   - svc: the service instance descriptor to register (ID, Name, Address,
//     Port, Tags, Metadata). Address must be provided.
//   - log: logger instance
//
// Returns an error if Address is empty (auto-resolution is not yet implemented).
func NewDiscoveryServerComponent(
	inner *Component,
	registry discovery.Registry,
	svc discovery.ServiceInfo,
	log *logging.Logger,
) (*DiscoveryServerComponent, error) {
	if registry == nil {
		return nil, fmt.Errorf("registry cannot be nil")
	}
	if inner == nil {
		return nil, fmt.Errorf("inner component cannot be nil")
	}

	if svc.Address == "" {
		return nil, fmt.Errorf("address must be provided (auto-resolution not yet implemented)")
	}

	return &DiscoveryServerComponent{
		inner:     inner,
		registry:  registry,
		serviceID: svc.ID,
		svcInfo:   &svc,
		log:       log.WithComponent("discovery-server"),
	}, nil
}

// ensure DiscoveryServerComponent satisfies component.Component.
var _ component.Component = (*DiscoveryServerComponent)(nil)

// Name returns the component name.
func (dsc *DiscoveryServerComponent) Name() string {
	return "discovery-server"
}

// Start starts the inner server, then registers with discovery.
func (dsc *DiscoveryServerComponent) Start(ctx context.Context) error {
	// Start the inner server first
	if err := dsc.inner.Start(ctx); err != nil {
		return fmt.Errorf("failed to start inner server: %w", err)
	}

	dsc.log.DebugCtx(ctx, "Starting service registration", map[string]any{
		"service_id":   dsc.serviceID,
		"service_name": dsc.svcInfo.Name,
		"address":      dsc.svcInfo.Address,
		"port":         dsc.svcInfo.Port,
	})

	// Register with discovery
	if err := dsc.registry.Register(ctx, dsc.svcInfo); err != nil {
		dsc.log.ErrorCtx(ctx, "Failed to register with discovery", map[string]any{
			"error": err.Error(),
		})
		// Stop the server if registration fails
		if stopErr := dsc.inner.Stop(ctx); stopErr != nil {
			dsc.log.WarnCtx(ctx, "Failed to stop server after registration failure", map[string]any{
				"error": stopErr.Error(),
			})
		}
		return fmt.Errorf("failed to register service: %w", err)
	}

	dsc.log.DebugCtx(ctx, "Service registered with discovery", map[string]any{
		"service_id": dsc.serviceID,
	})
	return nil
}

// Stop deregisters from discovery, then stops the inner server.
func (dsc *DiscoveryServerComponent) Stop(ctx context.Context) error {
	dsc.log.DebugCtx(ctx, "Stopping discovery-server component")

	// Deregister from discovery
	if err := dsc.registry.Deregister(ctx, dsc.serviceID); err != nil {
		dsc.log.WarnCtx(ctx, "Failed to deregister from discovery", map[string]any{
			"error": err.Error(),
		})
		// Continue to stop the server even if deregistration fails
	}

	// Stop the inner server
	if err := dsc.inner.Stop(ctx); err != nil {
		return fmt.Errorf("failed to stop inner server: %w", err)
	}

	dsc.log.DebugCtx(ctx, "Discovery-server component stopped")
	return nil
}

// Health delegates to the inner component and adds discovery health info.
func (dsc *DiscoveryServerComponent) Health(ctx context.Context) component.Health {
	innerHealth := dsc.inner.Health(ctx)

	// Add context about discovery registration
	if innerHealth.Status == component.StatusHealthy {
		return component.Health{
			Name:    dsc.Name(),
			Status:  component.StatusHealthy,
			Message: fmt.Sprintf("server healthy; registered as %s", dsc.svcInfo.Name),
		}
	}

	return component.Health{
		Name:    dsc.Name(),
		Status:  innerHealth.Status,
		Message: fmt.Sprintf("discovery-server: %s", innerHealth.Message),
	}
}

// Describe returns infrastructure summary info for the bootstrap display.
func (dsc *DiscoveryServerComponent) Describe() component.Description {
	innerDesc := dsc.inner.Describe()
	// Enhance with discovery info
	details := fmt.Sprintf("%s (discovery: %s)", innerDesc.Details, dsc.svcInfo.Name)
	return component.Description{
		Name:    "Discovery Server",
		Type:    "discovery-server",
		Details: details,
		Port:    dsc.svcInfo.Port,
	}
}

// Server returns the underlying server component for direct access if needed.
func (dsc *DiscoveryServerComponent) Server() *Component {
	return dsc.inner
}

// ServiceInfo returns the service info used for registration.
func (dsc *DiscoveryServerComponent) ServiceInfo() *discovery.ServiceInfo {
	return dsc.svcInfo
}
