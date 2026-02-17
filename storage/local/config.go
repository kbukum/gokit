package local

import "fmt"

// DefaultBasePath is the default root directory for local storage.
const DefaultBasePath = "/tmp/storage"

// Config holds local filesystem storage configuration.
type Config struct {
	// BasePath is the root directory for local storage.
	BasePath string `mapstructure:"base_path" json:"base_path"`
}

// ApplyDefaults fills in zero-valued fields with sensible defaults.
func (c *Config) ApplyDefaults() {
	if c.BasePath == "" {
		c.BasePath = DefaultBasePath
	}
}

// Validate checks that the local configuration is valid.
func (c *Config) Validate() error {
	if c.BasePath == "" {
		return fmt.Errorf("local: base_path is required")
	}
	return nil
}
