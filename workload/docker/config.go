package docker

import (
	"errors"
	"fmt"
)

// Config holds Docker-specific workload configuration.
type Config struct {
	Host       string     `mapstructure:"host" json:"host"`
	APIVersion string     `mapstructure:"api_version" json:"api_version"`
	TLS        *TLSConfig `mapstructure:"tls" json:"tls"`
	Network    string     `mapstructure:"network" json:"network"`
	Registry   string     `mapstructure:"registry" json:"registry"`
	Platform   string     `mapstructure:"platform" json:"platform"`
}

// TLSConfig holds Docker TLS settings.
type TLSConfig struct {
	CACert string `mapstructure:"ca_cert" json:"ca_cert"`
	Cert   string `mapstructure:"cert" json:"cert"`
	Key    string `mapstructure:"key" json:"key"`
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
		if c.TLS.Cert == "" || c.TLS.Key == "" {
			return fmt.Errorf("docker: tls cert and key are both required when tls is enabled")
		}
	}
	return nil
}
