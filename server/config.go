package server

import (
	"fmt"

	"github.com/skillsenselab/gokit/server/middleware"
)

// Config holds HTTP server configuration.
type Config struct {
	Host         string                `yaml:"host" mapstructure:"host"`
	Port         int                   `yaml:"port" mapstructure:"port"`
	ReadTimeout  int                   `yaml:"read_timeout" mapstructure:"read_timeout"`   // seconds
	WriteTimeout int                   `yaml:"write_timeout" mapstructure:"write_timeout"` // seconds
	IdleTimeout  int                   `yaml:"idle_timeout" mapstructure:"idle_timeout"`   // seconds
	MaxBodySize  string                `yaml:"max_body_size" mapstructure:"max_body_size"` // e.g. "10MB"
	CORS         middleware.CORSConfig `yaml:"cors" mapstructure:"cors"`
	Enabled      bool                  `yaml:"enabled" mapstructure:"enabled"`
}

// ApplyDefaults sets sensible default values for unset fields.
func (c *Config) ApplyDefaults() {
	if c.Port == 0 {
		c.Port = 8080
	}
	if c.ReadTimeout == 0 {
		c.ReadTimeout = 15
	}
	if c.WriteTimeout == 0 {
		c.WriteTimeout = 15
	}
	if c.IdleTimeout == 0 {
		c.IdleTimeout = 60
	}
	if c.MaxBodySize == "" {
		c.MaxBodySize = "10MB"
	}
	if len(c.CORS.AllowedOrigins) == 0 {
		c.CORS.AllowedOrigins = []string{"*"}
	}
	if len(c.CORS.AllowedMethods) == 0 {
		c.CORS.AllowedMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
	}
	if len(c.CORS.AllowedHeaders) == 0 {
		c.CORS.AllowedHeaders = []string{"Origin", "Content-Type", "Accept", "Authorization"}
	}
}

// Validate checks the configuration for invalid values.
func (c *Config) Validate() error {
	if c.Port < 0 || c.Port > 65535 {
		return fmt.Errorf("server.port must be between 0 and 65535 (got: %d)", c.Port)
	}
	if c.ReadTimeout < 0 {
		return fmt.Errorf("server.read_timeout must be non-negative (got: %d)", c.ReadTimeout)
	}
	if c.WriteTimeout < 0 {
		return fmt.Errorf("server.write_timeout must be non-negative (got: %d)", c.WriteTimeout)
	}
	if c.IdleTimeout < 0 {
		return fmt.Errorf("server.idle_timeout must be non-negative (got: %d)", c.IdleTimeout)
	}
	return nil
}
