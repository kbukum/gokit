package docker

import (
	"errors"
	"fmt"

	"github.com/kbukum/gokit/security"
)

// Config holds Docker-specific workload configuration.
type Config struct {
	Host       string              `mapstructure:"host" json:"host"`
	APIVersion string              `mapstructure:"api_version" json:"api_version"`
	TLS        *security.TLSConfig `mapstructure:"tls" json:"tls"`
	Network    string              `mapstructure:"network" json:"network"`
	Registry   string              `mapstructure:"registry" json:"registry"`
	Platform   string              `mapstructure:"platform" json:"platform"`
}

// ApplyDefaults fills in zero-valued fields.
func (c *Config) ApplyDefaults() {
	if c.Host == "" {
		c.Host = "unix:///var/run/docker.sock"
	}
}

// Validate checks the Docker configuration.
func (c *Config) Validate() error {
	if c.Host == "" {
		return errors.New("docker: host is required")
	}
	if c.TLS != nil {
		if err := c.TLS.Validate(); err != nil {
			return fmt.Errorf("docker: %w", err)
		}
	}
	return nil
}
