package workload

import "fmt"

const (
	DefaultProvider = ProviderDocker
)

// Config holds provider-agnostic workload configuration.
type Config struct {
	Provider      string            `mapstructure:"provider" json:"provider"`
	Enabled       bool              `mapstructure:"enabled" json:"enabled"`
	DefaultLabels map[string]string `mapstructure:"default_labels" json:"default_labels"`
}

// ApplyDefaults fills in zero-valued fields.
func (c *Config) ApplyDefaults() {
	if c.Provider == "" {
		c.Provider = DefaultProvider
	}
}

// Validate checks that the core configuration is valid.
func (c *Config) Validate() error {
	if c.Provider == "" {
		return fmt.Errorf("workload: provider is required")
	}
	return nil
}
