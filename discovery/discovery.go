package discovery

import (
	"context"
	"encoding/json"
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
//
// Its JSON encoding is gokit's stable discovery contract: snake_case field
// names, a tri-state Health that is always one of "unknown"/"healthy"/
// "unhealthy", and an omitted Weight that decodes to 1. It is the shape gokit
// emits and accepts across its discovery backends; cross-kit alignment with
// rskit is tracked separately and is not yet byte-for-byte identical.
type ServiceInstance struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Address  string            `json:"address"`
	Port     int               `json:"port"`
	Protocol string            `json:"protocol"`
	Tags     []string          `json:"tags"`
	Metadata map[string]string `json:"metadata"`
	Health   HealthStatus      `json:"health"`
	Weight   int               `json:"weight"`
	LastSeen time.Time         `json:"last_seen"`
}

// UnmarshalJSON decodes a ServiceInstance, defaulting an omitted Weight to 1 so
// weighted balancing treats an unspecified instance as a single unit of load
// (an explicit "weight": 0 is preserved as-is), and an omitted Health to
// HealthUnknown so the field is always one of the tri-state values. A present
// but out-of-range Health is rejected so a malformed payload cannot enter the
// model.
func (s *ServiceInstance) UnmarshalJSON(data []byte) error {
	type alias ServiceInstance
	tmp := alias{Weight: 1, Health: HealthUnknown}
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}
	if !tmp.Health.valid() {
		return fmt.Errorf("discovery: invalid health status %q", string(tmp.Health))
	}
	*s = ServiceInstance(tmp)
	return nil
}

// Endpoint is an alias for ServiceInstance, providing a shorter name
// for callers that prefer endpoint-oriented terminology.
type Endpoint = ServiceInstance

// HostPort returns the host:port string (e.g., "192.168.1.5:8080").
// Use this to set a client's target address from a discovered endpoint.
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

// valid reports whether h is one of the defined tri-state values.
func (h HealthStatus) valid() bool {
	switch h {
	case HealthUnknown, HealthHealthy, HealthUnhealthy:
		return true
	default:
		return false
	}
}

// MarshalJSON encodes HealthStatus, mapping the zero value to "unknown" so the
// wire form is always populated. Any other out-of-range value is rejected so an
// invalid in-memory status cannot be emitted as a valid-looking payload.
func (h HealthStatus) MarshalJSON() ([]byte, error) {
	if h == "" {
		h = HealthUnknown
	}
	if !h.valid() {
		return nil, fmt.Errorf("discovery: invalid health status %q", string(h))
	}
	return json.Marshal(string(h))
}

// Discovery defines the contract for discovering service instances.
type Discovery interface {
	// Discover returns all healthy instances of the named service.
	Discover(ctx context.Context, serviceName string) ([]ServiceInstance, error)

	// Watch returns a channel that emits the current set of instances whenever the service membership changes.
	// Close the context to stop.
	Watch(ctx context.Context, serviceName string) (<-chan []ServiceInstance, error)

	// Close releases any resources held by the discovery client.
	Close() error
}
