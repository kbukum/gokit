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

	// --- Registration (self) ---

	// ServiceName is the name used when registering this service.
	ServiceName string `mapstructure:"service_name"`

	// ServiceID is the unique instance ID; defaults to ServiceName if empty.
	ServiceID string `mapstructure:"service_id"`

	// ServiceAddress is the address advertised to other services.
	ServiceAddress string `mapstructure:"service_address"`

	// ServicePort is the port advertised to other services.
	ServicePort int `mapstructure:"service_port"`

	// Tags are metadata tags attached to the service registration.
	Tags []string `mapstructure:"tags"`

	// Metadata is arbitrary key-value metadata for the service.
	Metadata map[string]string `mapstructure:"metadata"`

	// --- Health checks ---

	// HealthCheckType is the type of health check: "http", "grpc", "tcp", or "ttl".
	// Defaults to "http".
	HealthCheckType string `mapstructure:"health_check_type"`

	// HealthCheckPath is the HTTP path for health checks (e.g. "/healthz").
	HealthCheckPath string `mapstructure:"health_check_path"`

	// HealthCheckInterval controls how often health is polled.
	HealthCheckInterval time.Duration `mapstructure:"health_check_interval"`

	// HealthCheckTimeout is the timeout for a single health check.
	HealthCheckTimeout time.Duration `mapstructure:"health_check_timeout"`

	// DeregisterAfter removes the service after being critical for this duration.
	DeregisterAfter time.Duration `mapstructure:"deregister_after"`

	// --- Discovery (others) ---

	// CacheTTL controls how long discovered endpoints are cached by the
	// high-level Client. Default: 30s.
	CacheTTL time.Duration `mapstructure:"cache_ttl"`

	// Services lists remote services this application depends on.
	Services []DiscoveredService `mapstructure:"services"`

	// StaticEndpoints provides endpoints for the static provider or as fallback.
	StaticEndpoints []StaticEndpoint `mapstructure:"static_endpoints"`
}

// DiscoveredService describes a remote service dependency.
type DiscoveredService struct {
	Name        string      `mapstructure:"name"`
	Protocol    string      `mapstructure:"protocol"`
	Criticality Criticality `mapstructure:"criticality"`
}

// StaticEndpoint describes a statically configured service endpoint.
type StaticEndpoint struct {
	Name     string            `mapstructure:"name"`
	Address  string            `mapstructure:"address"`
	Port     int               `mapstructure:"port"`
	Protocol string            `mapstructure:"protocol"`
	Tags     []string          `mapstructure:"tags"`
	Metadata map[string]string `mapstructure:"metadata"`
	Weight   int               `mapstructure:"weight"`
	Healthy  bool              `mapstructure:"healthy"`
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

// BuildClientConfig derives a ClientConfig from this Config, avoiding duplication.
func (c *Config) BuildClientConfig() ClientConfig {
	cc := ClientConfig{
		CacheTTL:        c.CacheTTL,
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
