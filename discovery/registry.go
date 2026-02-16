package discovery

import (
	"context"
	"time"
)

// ServiceInfo contains information about a service instance to register.
type ServiceInfo struct {
	ID       string
	Name     string
	Address  string
	Port     int
	Tags     []string
	Metadata map[string]string
}

// Registry defines the contract for service registration and deregistration.
type Registry interface {
	// Register registers a service instance with the discovery backend.
	Register(ctx context.Context, service *ServiceInfo) error

	// Deregister removes a service instance from the discovery backend.
	Deregister(ctx context.Context, serviceID string) error

	// UpdateHealth pushes a health status update for a registered service.
	// For TTL-based checks, this extends the TTL; for others it may be a no-op.
	UpdateHealth(ctx context.Context, serviceID string, healthy bool, note string) error

	// Stats returns current registry statistics.
	Stats() RegistryStats

	// Close releases any resources held by the registry.
	Close() error
}

// RegistryStats holds registry metrics.
type RegistryStats struct {
	RegisteredServices int
	LastHeartbeat      time.Time
}
