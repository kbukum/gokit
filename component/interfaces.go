package component

import "context"

// State represents the lifecycle state of a component.
type State int

const (
	// StateCreated is the initial state after registration.
	StateCreated State = iota
	// StateStarting indicates the component is currently starting.
	StateStarting
	// StateRunning indicates the component started successfully and is operational.
	StateRunning
	// StateStopping indicates the component is currently shutting down.
	StateStopping
	// StateStopped indicates the component has been shut down.
	StateStopped
	// StateFailed indicates the component failed to start or encountered a fatal error.
	StateFailed
)

// String returns the human-readable name of a lifecycle state.
func (s State) String() string {
	switch s {
	case StateCreated:
		return "created"
	case StateStarting:
		return "starting"
	case StateRunning:
		return "running"
	case StateStopping:
		return "stopping"
	case StateStopped:
		return "stopped"
	case StateFailed:
		return "failed"
	default:
		return "unknown"
	}
}

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
//
// The canonical lifecycle state machine is:
//
//	Created → Starting → Running → Stopping → Stopped
//	                ↘ Failed
//
// Stop() is responsible for draining any inflight work before releasing
// resources. The framework enforces a per-component timeout via the context.
type Component interface {
	// Name returns the unique name of the component for registration.
	Name() string

	// Start initializes and starts the component.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the component and releases resources.
	// Implementations must drain inflight work within the context deadline.
	Stop(ctx context.Context) error

	// Health returns the current health status of the component.
	Health(ctx context.Context) Health
}

// StopResult holds the outcome of stopping a single component.
type StopResult struct {
	// Name of the component.
	Name string
	// Err is nil on success, non-nil on failure.
	Err error
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
