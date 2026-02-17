package component

import "context"

// HealthStatus represents the health state of a component.
type HealthStatus string

const (
	StatusHealthy   HealthStatus = "healthy"
	StatusUnhealthy HealthStatus = "unhealthy"
	StatusDegraded  HealthStatus = "degraded"
)

// Health holds health information for a component.
type Health struct {
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
	Health(ctx context.Context) Health
}

// Description holds summary information for the bootstrap display.
// Components that implement Describable return this to self-report
// what they are and how they're configured.
type Description struct {
	// Name is the human-readable display name (e.g., "HTTP Server", "PostgreSQL").
	// If empty, the component's Name() is used.
	Name string
	// Type categorizes the component: "database", "server", "kafka", "redis", etc.
	Type string
	// Details is a human-readable one-liner shown in the startup summary.
	// Examples: "localhost:5432 pool=25/5", "localhost:6379 db=0 pool=10"
	Details string
	// Port is the primary port, 0 if not applicable.
	Port int
}

// Describable is optionally implemented by Components to provide
// startup summary information for the bootstrap display.
//
// When a component implements this interface, the bootstrap system
// automatically includes it in the infrastructure section of the
// startup summary — no manual TrackInfrastructure calls needed.
type Describable interface {
	Describe() Description
}

// Route holds a single HTTP route for the startup summary.
type Route struct {
	Method  string
	Path    string
	Handler string
}

// RouteProvider is optionally implemented by server components to
// auto-report registered HTTP routes for the startup summary.
type RouteProvider interface {
	Routes() []Route
}
