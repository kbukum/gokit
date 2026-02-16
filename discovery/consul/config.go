package consul

import (
	"fmt"
	"time"
)

// Config holds Consul connection and client settings.
type Config struct {
	// Address is the Consul agent address (default: localhost:8500).
	Address string `yaml:"address" mapstructure:"address"`

	// Scheme is the URI scheme (http/https).
	Scheme string `yaml:"scheme" mapstructure:"scheme"`

	// Datacenter to use.
	Datacenter string `yaml:"datacenter" mapstructure:"datacenter"`

	// Token is the ACL token for authentication.
	Token string `yaml:"token" mapstructure:"token"`

	// Namespace for Consul Enterprise.
	Namespace string `yaml:"namespace" mapstructure:"namespace"`

	// Partition for Consul Enterprise.
	Partition string `yaml:"partition" mapstructure:"partition"`

	// TLS configuration.
	TLS *TLSConfig `yaml:"tls" mapstructure:"tls"`

	// Pool holds connection pool settings.
	Pool *PoolConfig `yaml:"pool" mapstructure:"pool"`

	// ConnectTimeout is the connection timeout.
	ConnectTimeout time.Duration `yaml:"connect_timeout" mapstructure:"connect_timeout"`

	// ReadTimeout is the read timeout.
	ReadTimeout time.Duration `yaml:"read_timeout" mapstructure:"read_timeout"`

	// WriteTimeout is the write timeout.
	WriteTimeout time.Duration `yaml:"write_timeout" mapstructure:"write_timeout"`
}

// TLSConfig holds TLS configuration for Consul connections.
type TLSConfig struct {
	// Enabled toggles TLS.
	Enabled bool `yaml:"enabled" mapstructure:"enabled"`

	// CACert is the path to CA certificate.
	CACert string `yaml:"ca_cert" mapstructure:"ca_cert"`

	// CAPath is the path to a directory of CA certificates.
	CAPath string `yaml:"ca_path" mapstructure:"ca_path"`

	// ClientCert is the path to client certificate.
	ClientCert string `yaml:"client_cert" mapstructure:"client_cert"`

	// ClientKey is the path to client key.
	ClientKey string `yaml:"client_key" mapstructure:"client_key"`

	// InsecureSkipVerify skips TLS verification (not recommended for production).
	InsecureSkipVerify bool `yaml:"insecure_skip_verify" mapstructure:"insecure_skip_verify"`

	// ServerName is the server name for TLS verification.
	ServerName string `yaml:"server_name" mapstructure:"server_name"`
}

// PoolConfig holds connection pool settings.
type PoolConfig struct {
	// MaxIdleConns controls maximum idle connections.
	MaxIdleConns int `yaml:"max_idle_conns" mapstructure:"max_idle_conns"`

	// MaxIdleConnsPerHost controls max idle connections per host.
	MaxIdleConnsPerHost int `yaml:"max_idle_conns_per_host" mapstructure:"max_idle_conns_per_host"`

	// MaxConnsPerHost controls max connections per host.
	MaxConnsPerHost int `yaml:"max_conns_per_host" mapstructure:"max_conns_per_host"`

	// IdleConnTimeout is how long connections stay idle.
	IdleConnTimeout time.Duration `yaml:"idle_conn_timeout" mapstructure:"idle_conn_timeout"`
}

// ApplyDefaults sets sensible defaults for Config.
func (c *Config) ApplyDefaults() {
	if c.Address == "" {
		c.Address = "localhost:8500"
	}
	if c.Scheme == "" {
		c.Scheme = "http"
	}
	if c.Datacenter == "" {
		c.Datacenter = "dc1"
	}
	if c.ConnectTimeout == 0 {
		c.ConnectTimeout = 10 * time.Second
	}
	if c.ReadTimeout == 0 {
		c.ReadTimeout = 30 * time.Second
	}
	if c.WriteTimeout == 0 {
		c.WriteTimeout = 30 * time.Second
	}
	if c.Pool == nil {
		c.Pool = &PoolConfig{}
	}
	c.Pool.ApplyDefaults()
}

// ApplyDefaults sets sensible defaults for PoolConfig.
func (c *PoolConfig) ApplyDefaults() {
	if c.MaxIdleConns == 0 {
		c.MaxIdleConns = 100
	}
	if c.MaxIdleConnsPerHost == 0 {
		c.MaxIdleConnsPerHost = 10
	}
	if c.MaxConnsPerHost == 0 {
		c.MaxConnsPerHost = 100
	}
	if c.IdleConnTimeout == 0 {
		c.IdleConnTimeout = 90 * time.Second
	}
}

// Validate checks if the Consul configuration is valid.
func (c *Config) Validate() error {
	if c.Address == "" {
		return fmt.Errorf("consul address is required")
	}
	if c.Scheme != "http" && c.Scheme != "https" {
		return fmt.Errorf("consul scheme must be 'http' or 'https', got '%s'", c.Scheme)
	}
	if c.TLS != nil && c.TLS.Enabled && c.Scheme != "https" {
		return fmt.Errorf("TLS enabled but scheme is not https")
	}
	if c.ConnectTimeout < 0 {
		return fmt.Errorf("connect_timeout must be non-negative")
	}
	if c.ReadTimeout < 0 {
		return fmt.Errorf("read_timeout must be non-negative")
	}
	if c.WriteTimeout < 0 {
		return fmt.Errorf("write_timeout must be non-negative")
	}
	return nil
}
