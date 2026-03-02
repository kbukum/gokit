package discovery

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Common discovery errors.
var (
	ErrServiceNotFound    = errors.New("service not found")
	ErrNoHealthyEndpoints = errors.New("no healthy endpoints found")
	ErrDiscoveryDisabled  = errors.New("service discovery is disabled")
)

// ServiceInstance represents a discovered service endpoint.
type ServiceInstance struct {
	ID       string
	Name     string
	Address  string
	Port     int
	Protocol string
	Tags     []string
	Metadata map[string]string
	Health   HealthStatus
	Weight   int
	LastSeen time.Time
}

// Endpoint is an alias for ServiceInstance, providing a shorter name
// for callers that prefer endpoint-oriented terminology.
type Endpoint = ServiceInstance

// HostPort returns the host:port string (e.g., "192.168.1.5:8080").
// Use this to set client Config.Addr from a discovered endpoint.
func (s ServiceInstance) HostPort() string {
	return fmt.Sprintf("%s:%d", s.Address, s.Port)
}

// HealthStatus represents endpoint health.
type HealthStatus string

const (
	HealthUnknown   HealthStatus = "unknown"
	HealthHealthy   HealthStatus = "healthy"
	HealthUnhealthy HealthStatus = "unhealthy"
)

// Discovery defines the contract for discovering service instances.
type Discovery interface {
	// Discover returns all healthy instances of the named service.
	Discover(ctx context.Context, serviceName string) ([]ServiceInstance, error)

	// Watch returns a channel that emits the current set of instances
	// whenever the service membership changes. Close the context to stop.
	Watch(ctx context.Context, serviceName string) (<-chan []ServiceInstance, error)

	// Close releases any resources held by the discovery client.
	Close() error
}
