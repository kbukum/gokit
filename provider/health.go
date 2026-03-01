package provider

import "context"

// Status represents the health status of a provider.
type Status int

const (
	// StatusHealthy indicates the provider is fully operational.
	StatusHealthy Status = iota
	// StatusDegraded indicates the provider is operational but with reduced capability.
	StatusDegraded
	// StatusUnavailable indicates the provider cannot handle requests.
	StatusUnavailable
)

// String returns the status name.
func (s Status) String() string {
	switch s {
	case StatusHealthy:
		return "healthy"
	case StatusDegraded:
		return "degraded"
	case StatusUnavailable:
		return "unavailable"
	default:
		return "unknown"
	}
}

// HealthStatus contains detailed health information for a provider.
type HealthStatus struct {
	// Status is the overall health status.
	Status Status
	// Message is a human-readable description of the health state.
	Message string
	// Details contains additional health metadata (latency, pool size, etc).
	Details map[string]any
}

// HealthChecker is optionally implemented by providers that can report
// detailed health beyond the simple IsAvailable() bool check.
type HealthChecker interface {
	Health(ctx context.Context) HealthStatus
}
