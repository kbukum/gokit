package server

import (
	"fmt"

	"github.com/kbukum/gokit/security"
	"github.com/kbukum/gokit/server/middleware"
)

// Config holds HTTP server configuration.
type Config struct {
	Host            string                `yaml:"host" mapstructure:"host"`
	Port            int                   `yaml:"port" mapstructure:"port"`
	ReadTimeout     int                   `yaml:"read_timeout" mapstructure:"read_timeout"`         // seconds
	WriteTimeout    int                   `yaml:"write_timeout" mapstructure:"write_timeout"`       // seconds
	IdleTimeout     int                   `yaml:"idle_timeout" mapstructure:"idle_timeout"`         // seconds
	RequestTimeout  int                   `yaml:"request_timeout" mapstructure:"request_timeout"`   // seconds; 0 disables the per-request timeout. Applies to REST routes only, not RPC/streaming mounts; keep it off in front of SSE/streaming handlers.
	ShutdownTimeout int                   `yaml:"shutdown_timeout" mapstructure:"shutdown_timeout"` // seconds
	MaxBodyBytes    int64                 `yaml:"max_body_bytes" mapstructure:"max_body_bytes"`     // request body cap in bytes
	EnableH2C       *bool                 `yaml:"enable_h2c" mapstructure:"enable_h2c"`             // serve HTTP/2 cleartext when TLS is off (default true)
	TLS             *security.TLSConfig   `yaml:"tls" mapstructure:"tls"`
	CORS            middleware.CORSConfig `yaml:"cors" mapstructure:"cors"`
	// SecurityHeaders configures the secure-by-default response headers applied to every route.
	// Zero value yields secure defaults; set Disabled to opt out.
	SecurityHeaders security.HeadersConfig `yaml:"security_headers" mapstructure:"security_headers"`
	Docs            DocsConfig             `yaml:"docs" mapstructure:"docs"`
	Enabled         bool                   `yaml:"enabled" mapstructure:"enabled"`
}

// DocsConfig controls API documentation serving.
type DocsConfig struct {
	// Enabled controls whether API docs are served.
	Enabled bool `yaml:"enabled" mapstructure:"enabled"`
	// SpecPath is the route for the raw OpenAPI spec (default: "/docs/openapi.json").
	SpecPath string `yaml:"spec_path" mapstructure:"spec_path"`
	// UIPath is the route for the interactive Scalar UI (default: "/docs").
	UIPath string `yaml:"ui_path" mapstructure:"ui_path"`
	// Title is shown in the browser tab (default: "API Reference").
	Title string `yaml:"title" mapstructure:"title"`
	// SpecFile is an optional path to an OpenAPI spec file on disk. If set,
	// the spec is loaded from this file at startup.
	SpecFile string `yaml:"spec_file" mapstructure:"spec_file"`
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
	if c.ShutdownTimeout == 0 {
		c.ShutdownTimeout = 5
	}
	if c.MaxBodyBytes == 0 {
		c.MaxBodyBytes = 10 * 1024 * 1024
	}
	if c.EnableH2C == nil {
		enabled := true
		c.EnableH2C = &enabled
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

// H2CEnabled reports whether HTTP/2 cleartext should be served when TLS is off.
// It defaults to true when unset.
func (c *Config) H2CEnabled() bool {
	return c.EnableH2C == nil || *c.EnableH2C
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
	if c.RequestTimeout < 0 {
		return fmt.Errorf("server.request_timeout must be non-negative (got: %d)", c.RequestTimeout)
	}
	if c.ShutdownTimeout < 0 {
		return fmt.Errorf("server.shutdown_timeout must be non-negative (got: %d)", c.ShutdownTimeout)
	}
	if c.MaxBodyBytes < 0 {
		return fmt.Errorf("server.max_body_bytes must be non-negative (got: %d)", c.MaxBodyBytes)
	}
	if _, err := c.SecurityHeaders.HeaderMap(); err != nil {
		return err
	}
	return nil
}
