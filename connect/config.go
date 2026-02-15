package connect

import "fmt"

// Config holds configuration for Connect-Go integration.
type Config struct {
	// Enabled controls whether Connect-Go services are active.
	Enabled bool `yaml:"enabled" mapstructure:"enabled"`
	// SendMaxBytes is the maximum size of a message the server can send (bytes).
	SendMaxBytes int `yaml:"send_max_bytes" mapstructure:"send_max_bytes"`
	// ReadMaxBytes is the maximum size of a message the server can receive (bytes).
	ReadMaxBytes int `yaml:"read_max_bytes" mapstructure:"read_max_bytes"`
}

const defaultMaxBytes = 4 * 1024 * 1024 // 4 MB

// ApplyDefaults fills in zero-value fields with sensible defaults.
func (c *Config) ApplyDefaults() {
	if c.SendMaxBytes == 0 {
		c.SendMaxBytes = defaultMaxBytes
	}
	if c.ReadMaxBytes == 0 {
		c.ReadMaxBytes = defaultMaxBytes
	}
}

// Validate checks that the configuration is valid.
func (c *Config) Validate() error {
	if c.SendMaxBytes < 0 {
		return fmt.Errorf("connectrpc: send_max_bytes must be non-negative, got %d", c.SendMaxBytes)
	}
	if c.ReadMaxBytes < 0 {
		return fmt.Errorf("connectrpc: read_max_bytes must be non-negative, got %d", c.ReadMaxBytes)
	}
	return nil
}
