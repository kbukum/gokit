package storage

import (
	"fmt"
	"time"
)

// Provider constants for well-known storage backends.
const (
	ProviderLocal    = "local"
	ProviderS3       = "s3"
	ProviderSupabase = "supabase"
)

// Default configuration values.
const (
	DefaultProvider     = ProviderLocal
	DefaultMaxFileSize  = int64(100 * 1024 * 1024) // 100 MB
	DefaultPresignedTTL = time.Hour
)

// Config holds provider-agnostic storage configuration.
// Provider-specific settings are passed separately via providerCfg (any).
type Config struct {
	// Name identifies this adapter instance (used by provider.Provider interface).
	Name string `mapstructure:"name" json:"name"`

	// Provider selects the storage backend (e.g. "local", "s3", "supabase").
	Provider string `mapstructure:"provider" json:"provider"`

	// Enabled controls whether the storage component is active.
	Enabled bool `mapstructure:"enabled" json:"enabled"`

	// MaxFileSize is the maximum allowed file size in bytes.
	MaxFileSize int64 `mapstructure:"max_file_size" json:"max_file_size"`

	// PublicURL is the base URL for public access to stored objects.
	PublicURL string `mapstructure:"public_url" json:"public_url"`

	// PresignedTTL is the default duration for pre-signed URLs.
	PresignedTTL time.Duration `mapstructure:"presigned_ttl" json:"presigned_ttl"`

	// AllowedTypes is an optional whitelist of allowed MIME types for uploads.
	AllowedTypes []string `mapstructure:"allowed_types" json:"allowed_types"`
}

// ApplyDefaults fills in zero-valued fields with sensible defaults.
func (c *Config) ApplyDefaults() {
	if c.Provider == "" {
		c.Provider = DefaultProvider
	}
	if c.MaxFileSize <= 0 {
		c.MaxFileSize = DefaultMaxFileSize
	}
	if c.PresignedTTL <= 0 {
		c.PresignedTTL = DefaultPresignedTTL
	}
}

// Validate checks that the core configuration is valid.
// Provider-specific validation is handled by each provider's own config.
func (c *Config) Validate() error {
	if c.Provider == "" {
		return fmt.Errorf("storage: provider is required")
	}
	return nil
}
