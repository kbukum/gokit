package discovery

import (
	"fmt"
	"time"
)

// Config holds service discovery and registration configuration.
type Config struct {
	// Enabled controls whether the discovery component is active.
	Enabled bool `mapstructure:"enabled"`

	// Provider selects the discovery backend: "consul", "static", or "k8s".
	Provider string `mapstructure:"provider"`

	// ConsulAddr is the Consul agent address (host:port).
	ConsulAddr string `mapstructure:"consul_addr"`

	// ConsulToken is the Consul ACL token for authentication.
	ConsulToken string `mapstructure:"consul_token"`

	// ConsulScheme is the URI scheme for Consul ("http" or "https").
	ConsulScheme string `mapstructure:"consul_scheme"`

	// ConsulDatacenter is the Consul datacenter name.
	ConsulDatacenter string `mapstructure:"consul_datacenter"`

	// ServiceName is the name used when registering this service.
	ServiceName string `mapstructure:"service_name"`

	// ServiceID is the unique instance ID; defaults to ServiceName if empty.
	ServiceID string `mapstructure:"service_id"`

	// ServiceAddress is the address advertised to other services.
	ServiceAddress string `mapstructure:"service_address"`

	// ServicePort is the port advertised to other services.
	ServicePort int `mapstructure:"service_port"`

	// HealthCheckPath is the HTTP path for health checks (e.g. "/healthz").
	HealthCheckPath string `mapstructure:"health_check_path"`

	// HealthCheckInterval controls how often health is polled.
	HealthCheckInterval time.Duration `mapstructure:"health_check_interval"`

	// HealthCheckTimeout is the timeout for a single health check.
	HealthCheckTimeout time.Duration `mapstructure:"health_check_timeout"`

	// DeregisterAfter removes the service after being critical for this duration.
	DeregisterAfter time.Duration `mapstructure:"deregister_after"`

	// Tags are metadata tags attached to the service registration.
	Tags []string `mapstructure:"tags"`

	// Metadata is arbitrary key-value metadata for the service.
	Metadata map[string]string `mapstructure:"metadata"`

	// StaticEndpoints provides endpoints for the static provider.
	StaticEndpoints []StaticEndpoint `mapstructure:"static_endpoints"`
}

// StaticEndpoint describes a statically configured service endpoint.
type StaticEndpoint struct {
	Name     string `mapstructure:"name"`
	Address  string `mapstructure:"address"`
	Port     int    `mapstructure:"port"`
	Protocol string `mapstructure:"protocol"`
}

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
	if c.HealthCheckPath == "" {
		c.HealthCheckPath = "/healthz"
	}
	if c.HealthCheckInterval == 0 {
		c.HealthCheckInterval = 10 * time.Second
	}
	if c.HealthCheckTimeout == 0 {
		c.HealthCheckTimeout = 5 * time.Second
	}
	if c.DeregisterAfter == 0 {
		c.DeregisterAfter = 1 * time.Minute
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
