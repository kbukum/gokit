package observability

import "context"

// HealthStatus represents the health state of a component or service.
type HealthStatus string

const (
	// HealthStatusHealthy indicates a component or service is fully operational.
	HealthStatusHealthy HealthStatus = "healthy"
	// HealthStatusDegraded indicates a component or service is operational but impaired.
	HealthStatusDegraded HealthStatus = "degraded"
	// HealthStatusUnhealthy indicates a component or service is not operational.
	HealthStatusUnhealthy HealthStatus = "unhealthy"
)

// Health describes the health of an individual component.
type Health struct {
	Name    string            `json:"name"`
	Status  HealthStatus      `json:"status"`
	Message string            `json:"message,omitempty"`
	Details map[string]string `json:"details,omitempty"`
}

// ServiceHealth describes the overall health of a service and its components.
type ServiceHealth struct {
	Service    string       `json:"service"`
	Status     HealthStatus `json:"status"`
	Version    string       `json:"version,omitempty"`
	Components []Health     `json:"components,omitempty"`
}

// HealthChecker is implemented by components that can report their health.
type HealthChecker interface {
	CheckHealth(ctx context.Context) Health
}

// NewServiceHealth creates a ServiceHealth with a healthy status.
func NewServiceHealth(service, version string) *ServiceHealth {
	return &ServiceHealth{
		Service: service,
		Status:  HealthStatusHealthy,
		Version: version,
	}
}

// AddComponent adds a component health result and degrades overall status if needed.
func (sh *ServiceHealth) AddComponent(ch Health) {
	sh.Components = append(sh.Components, ch)

	switch ch.Status {
	case HealthStatusUnhealthy:
		sh.Status = HealthStatusUnhealthy
	case HealthStatusDegraded:
		if sh.Status != HealthStatusUnhealthy {
			sh.Status = HealthStatusDegraded
		}
	default:
	}
}
