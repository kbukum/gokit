package resilience

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// ServiceHealth represents the health level of a service.
type ServiceHealth int

const (
	// Healthy means the service is operating normally.
	Healthy ServiceHealth = iota
	// Degraded means the service is partially available.
	Degraded
	// Unhealthy means the service is unavailable.
	Unhealthy
)

// String returns the health level name.
func (h ServiceHealth) String() string {
	switch h {
	case Healthy:
		return "healthy"
	case Degraded:
		return "degraded"
	case Unhealthy:
		return "unhealthy"
	default:
		return "unknown"
	}
}

// ServiceStatus holds the current status of a tracked service.
type ServiceStatus struct {
	Name       string        `json:"name"`
	Health     ServiceHealth `json:"health"`
	LastCheck  time.Time     `json:"last_check"`
	LastChange time.Time     `json:"last_change"`
	Error      string        `json:"error,omitempty"`
}

// DegradationManager tracks service health and feature flags for graceful degradation. It is safe for concurrent use.
type DegradationManager struct {
	mu       sync.RWMutex
	services map[string]ServiceStatus
	features map[string]bool
}

// NewDegradationManager creates a new DegradationManager.
func NewDegradationManager() *DegradationManager {
	return &DegradationManager{
		services: make(map[string]ServiceStatus),
		features: make(map[string]bool),
	}
}

// UpdateService sets the health status for a named service. An optional error can be provided to record the failure reason.
func (dm *DegradationManager) UpdateService(name string, health ServiceHealth, err ...error) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	now := time.Now()
	existing, ok := dm.services[name]

	status := ServiceStatus{
		Name:       name,
		Health:     health,
		LastCheck:  now,
		LastChange: now,
	}

	if ok && existing.Health == health {
		status.LastChange = existing.LastChange
	}

	if len(err) > 0 && err[0] != nil {
		status.Error = err[0].Error()
	}

	dm.services[name] = status
}

// ServiceStatus returns the status of a named service. Returns a zero-value ServiceStatus if the service is not tracked.
func (dm *DegradationManager) ServiceStatus(name string) ServiceStatus {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.services[name]
}

// AllStatuses returns a snapshot of all tracked service statuses.
func (dm *DegradationManager) AllStatuses() map[string]ServiceStatus {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	result := make(map[string]ServiceStatus, len(dm.services))
	for k, v := range dm.services {
		result[k] = v
	}
	return result
}

// SetFeature enables or disables a feature flag.
func (dm *DegradationManager) SetFeature(name string, enabled bool) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	dm.features[name] = enabled
}

// FeatureEnabled returns whether a feature flag is enabled. Returns false for unknown features.
func (dm *DegradationManager) FeatureEnabled(name string) bool {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.features[name]
}

// IsHealthy returns true only if all tracked services are Healthy.
func (dm *DegradationManager) IsHealthy() bool {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	for _, s := range dm.services {
		if s.Health != Healthy {
			return false
		}
	}
	return true
}

// OnCircuitBreakerStateChange returns a callback compatible with CircuitBreakerConfig.OnStateChange. It automatically updates the service health when the circuit breaker changes state:
//   - StateClosed  → Healthy
//   - StateHalfOpen → Degraded
//   - StateOpen    → Unhealthy
func (dm *DegradationManager) OnCircuitBreakerStateChange(serviceName string) func(string, State, State) {
	return func(_ string, _, to State) {
		switch to {
		case StateClosed:
			dm.UpdateService(serviceName, Healthy)
		case StateHalfOpen:
			dm.UpdateService(serviceName, Degraded)
		case StateOpen:
			dm.UpdateService(serviceName, Unhealthy)
		}
	}
}

// healthResponse is the JSON structure returned by HealthEndpoint.
type healthResponse struct {
	Status   string                   `json:"status"`
	Services map[string]ServiceStatus `json:"services"`
}

// HealthEndpoint returns an http.HandlerFunc that reports aggregate health. Returns 200 when all services are healthy, 503 otherwise.
func (dm *DegradationManager) HealthEndpoint() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		statuses := dm.AllStatuses()
		healthy := dm.IsHealthy()

		status := "healthy"
		httpCode := http.StatusOK
		if !healthy {
			status = "degraded"
			httpCode = http.StatusServiceUnavailable
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(httpCode)

		_ = json.NewEncoder(w).Encode(healthResponse{
			Status:   status,
			Services: statuses,
		})
	}
}
