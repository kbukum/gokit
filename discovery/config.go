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

	// ConsulAddr is the Consul agent address (host:port).
	ConsulAddr string `yaml:"consul_addr" mapstructure:"consul_addr"`

	// ConsulToken is the Consul ACL token for authentication.
	ConsulToken string `yaml:"consul_token" mapstructure:"consul_token"`

	// ConsulScheme is the URI scheme for Consul ("http" or "https").
	ConsulScheme string `yaml:"consul_scheme" mapstructure:"consul_scheme"`

	// ConsulDatacenter is the Consul datacenter name.
	ConsulDatacenter string `yaml:"consul_datacenter" mapstructure:"consul_datacenter"`

	// --- Registration (self) ---

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

	// --- Health checks ---

	// HealthCheckType is the type of health check: "http", "grpc", "tcp", or "ttl".
	// Defaults to "http".
	HealthCheckType string `yaml:"health_check_type" mapstructure:"health_check_type"`

	// HealthCheckPath is the HTTP path for health checks (e.g. "/healthz").
	HealthCheckPath string `yaml:"health_check_path" mapstructure:"health_check_path"`

	// HealthCheckInterval controls how often health is polled (e.g. "10s").
	HealthCheckInterval string `yaml:"health_check_interval" mapstructure:"health_check_interval"`

	// HealthCheckTimeout is the timeout for a single health check (e.g. "5s").
	HealthCheckTimeout string `yaml:"health_check_timeout" mapstructure:"health_check_timeout"`

	// DeregisterAfter removes the service after being critical for this duration (e.g. "1m").
	DeregisterAfter string `yaml:"deregister_after" mapstructure:"deregister_after"`

	// --- Discovery (others) ---

	// CacheTTL controls how long discovered endpoints are cached (e.g. "30s").
	CacheTTL string `yaml:"cache_ttl" mapstructure:"cache_ttl"`

	// Services lists remote services this application depends on.
	Services []DiscoveredService `yaml:"services" mapstructure:"services"`

	// StaticEndpoints provides endpoints for the static provider or as fallback.
	StaticEndpoints []StaticEndpoint `yaml:"static_endpoints" mapstructure:"static_endpoints"`
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
	if c.ConsulAddr == "" {
		c.ConsulAddr = "localhost:8500"
	}
	if c.ConsulScheme == "" {
		c.ConsulScheme = "http"
	}
	if c.ServiceID == "" {
		c.ServiceID = c.ServiceName
	}
	if c.HealthCheckType == "" {
		c.HealthCheckType = HealthCheckHTTP
	}
	if c.HealthCheckPath == "" {
		c.HealthCheckPath = "/healthz"
	}
	if c.HealthCheckInterval == "" {
		c.HealthCheckInterval = "10s"
	}
	if c.HealthCheckTimeout == "" {
		c.HealthCheckTimeout = "5s"
	}
	if c.DeregisterAfter == "" {
		c.DeregisterAfter = "1m"
	}
}

// Validate checks that required fields are present and consistent.
func (c *Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	switch c.Provider {
	case "consul", "static", "k8s":
	default:
		return fmt.Errorf("unsupported discovery provider %q", c.Provider)
	}
	if c.Provider == "consul" && c.ConsulAddr == "" {
		return fmt.Errorf("consul_addr is required when provider is consul")
	}
	if c.ServiceName == "" {
		return fmt.Errorf("service_name is required")
	}
	if c.ServicePort <= 0 {
		return fmt.Errorf("service_port must be > 0")
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
