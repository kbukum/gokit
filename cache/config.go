package cache

import (
	"fmt"
	"time"
)

const (
	// ProviderMemory is the lean in-process cache backend shipped by core.
	ProviderMemory = "memory"

	// ProviderRedis is registered by the opt-in cache/redis adapter module.
	ProviderRedis = "redis"

	DefaultProvider = ProviderMemory
)

// Config holds provider-agnostic cache configuration.
type Config struct {
	Name       string        `mapstructure:"name" json:"name" yaml:"name"`
	Provider   string        `mapstructure:"provider" json:"provider" yaml:"provider"`
	Enabled    bool          `mapstructure:"enabled" json:"enabled" yaml:"enabled"`
	DefaultTTL time.Duration `mapstructure:"default_ttl" json:"default_ttl" yaml:"default_ttl"`
}

// ApplyDefaults fills zero-valued fields.
func (c *Config) ApplyDefaults() {
	if c.Provider == "" {
		c.Provider = DefaultProvider
	}
}

// Validate checks provider-agnostic settings.
func (c *Config) Validate() error {
	if c.Provider == "" {
		return fmt.Errorf("cache: provider is required")
	}
	if c.DefaultTTL < 0 {
		return fmt.Errorf("cache: default_ttl must be >= 0")
	}
	return nil
}
