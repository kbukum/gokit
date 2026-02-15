package component

import "context"

// HealthStatus represents the health state of a component.
type HealthStatus string

const (
	StatusHealthy   HealthStatus = "healthy"
	StatusUnhealthy HealthStatus = "unhealthy"
	StatusDegraded  HealthStatus = "degraded"
)

// ComponentHealth holds health information for a component.
type ComponentHealth struct {
	Name    string       `json:"name"`
	Status  HealthStatus `json:"status"`
	Message string       `json:"message,omitempty"`
}

// Component represents a lifecycle-managed infrastructure component.
// Each infrastructure module (database, redis, kafka, etc.) implements this interface.
type Component interface {
	// Name returns the unique name of the component for registration.
	Name() string

	// Start initializes and starts the component.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the component and releases resources.
	Stop(ctx context.Context) error

	// Health returns the current health status of the component.
	Health(ctx context.Context) ComponentHealth
}
