package discovery

import (
	"fmt"
	"time"
)

// Config holds service discovery and registration configuration.
type Config struct {
	// Enabled controls whether the discovery component is active.
	Enabled bool `yaml:"enabled" mapstructure:"enabled"`

	// Provider selects the discovery backend: "consul", "static", or "k8s".
	Provider string `yaml:"provider" mapstructure:"provider"`

	// Registration holds self-registration settings.
	Registration RegistrationConfig `yaml:"registration" mapstructure:"registration"`

	// Health holds health check settings for registered services.
	Health HealthCheckConfig `yaml:"health" mapstructure:"health"`

	// CacheTTL controls how long discovered endpoints are cached (e.g. "30s").
	CacheTTL string `yaml:"cache_ttl" mapstructure:"cache_ttl"`

	// Services lists remote services this application depends on.
	Services []DiscoveredService `yaml:"services" mapstructure:"services"`

	// StaticEndpoints provides endpoints for the static provider or as fallback.
	StaticEndpoints []StaticEndpoint `yaml:"static_endpoints" mapstructure:"static_endpoints"`
}

// RegistrationConfig holds settings for registering this service with a discovery backend.
type RegistrationConfig struct {
	// Enabled toggles self-registration.
	Enabled bool `yaml:"enabled" mapstructure:"enabled"`

	// ServiceName is the name used when registering this service.
	ServiceName string `yaml:"service_name" mapstructure:"service_name"`

	// ServiceID is the unique instance ID; defaults to ServiceName if empty.
	ServiceID string `yaml:"service_id" mapstructure:"service_id"`

	// ServiceAddress is the address advertised to other services.
	ServiceAddress string `yaml:"service_address" mapstructure:"service_address"`

	// ServicePort is the port advertised to other services.
	ServicePort int `yaml:"service_port" mapstructure:"service_port"`

	// Tags are metadata tags attached to the service registration.
	Tags []string `yaml:"tags" mapstructure:"tags"`

	// Metadata is arbitrary key-value metadata for the service.
	Metadata map[string]string `yaml:"metadata" mapstructure:"metadata"`
}

// HealthCheckConfig holds health check settings for service registration.
type HealthCheckConfig struct {
	// Enabled toggles health checks.
	Enabled bool `yaml:"enabled" mapstructure:"enabled"`

	// Type is the health check type: "http", "grpc", "tcp", or "ttl".
	// Defaults to "http".
	Type string `yaml:"type" mapstructure:"type"`

	// Path is the HTTP path for health checks (e.g. "/healthz").
	Path string `yaml:"path" mapstructure:"path"`

	// Interval controls how often health is polled (e.g. "10s").
	Interval string `yaml:"interval" mapstructure:"interval"`

	// Timeout is the timeout for a single health check (e.g. "5s").
	Timeout string `yaml:"timeout" mapstructure:"timeout"`

	// DeregisterAfter removes the service after being critical for this duration (e.g. "1m").
	DeregisterAfter string `yaml:"deregister_after" mapstructure:"deregister_after"`
}

// DiscoveredService describes a remote service dependency.
type DiscoveredService struct {
	Name        string      `yaml:"name" mapstructure:"name"`
	Protocol    string      `yaml:"protocol" mapstructure:"protocol"`
	Criticality Criticality `yaml:"criticality" mapstructure:"criticality"`
}

// StaticEndpoint describes a statically configured service endpoint.
type StaticEndpoint struct {
	Name     string            `yaml:"name" mapstructure:"name"`
	Address  string            `yaml:"address" mapstructure:"address"`
	Port     int               `yaml:"port" mapstructure:"port"`
	Protocol string            `yaml:"protocol" mapstructure:"protocol"`
	Tags     []string          `yaml:"tags" mapstructure:"tags"`
	Metadata map[string]string `yaml:"metadata" mapstructure:"metadata"`
	Weight   int               `yaml:"weight" mapstructure:"weight"`
	Healthy  bool              `yaml:"healthy" mapstructure:"healthy"`
}

// Health check type constants.
const (
	HealthCheckHTTP = "http"
	HealthCheckGRPC = "grpc"
	HealthCheckTCP  = "tcp"
	HealthCheckTTL  = "ttl"
)

// ApplyDefaults fills zero-valued fields with sensible defaults.
func (c *Config) ApplyDefaults() {
	if c.Provider == "" {
		c.Provider = "static"
	}
	c.Registration.ApplyDefaults()
	c.Health.ApplyDefaults()
}

// ApplyDefaults fills zero-valued RegistrationConfig fields.
func (r *RegistrationConfig) ApplyDefaults() {
	if r.ServiceID == "" {
		r.ServiceID = r.ServiceName
	}
}

// ApplyDefaults fills zero-valued HealthCheckConfig fields.
func (h *HealthCheckConfig) ApplyDefaults() {
	if h.Type == "" {
		h.Type = HealthCheckHTTP
	}
	if h.Path == "" {
		h.Path = "/healthz"
	}
	if h.Interval == "" {
		h.Interval = "10s"
	}
	if h.Timeout == "" {
		h.Timeout = "5s"
	}
	if h.DeregisterAfter == "" {
		h.DeregisterAfter = "1m"
	}
}

// Validate checks that required fields are present and consistent.
func (c *Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	if c.Registration.ServiceName == "" {
		return fmt.Errorf("registration.service_name is required")
	}
	if c.Registration.ServicePort <= 0 {
		return fmt.Errorf("registration.service_port must be > 0")
	}
	return nil
}

// BuildClientConfig derives a ClientConfig from this Config, avoiding duplication.
func (c *Config) BuildClientConfig() ClientConfig {
	cc := ClientConfig{
		CacheTTL:        ParseDuration(c.CacheTTL),
		Services:        make([]string, 0, len(c.Services)),
		Criticality:     make(map[string]Criticality, len(c.Services)),
		StaticEndpoints: c.StaticEndpoints,
	}
	for _, svc := range c.Services {
		cc.Services = append(cc.Services, svc.Name)
		cc.Criticality[svc.Name] = svc.Criticality
	}
	return cc
}

// ParseDuration parses a duration string, returning zero on empty or invalid input.
func ParseDuration(s string) time.Duration {
	d, _ := time.ParseDuration(s)
	return d
}
