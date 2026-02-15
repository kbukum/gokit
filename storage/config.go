package storage

import (
	"errors"
	"fmt"
)

// Provider constants for supported storage backends.
const (
	ProviderLocal    = "local"
	ProviderS3       = "s3"
	ProviderSupabase = "supabase"
)

// Default configuration values.
const (
	DefaultProvider    = ProviderLocal
	DefaultBasePath    = "/tmp/storage"
	DefaultRegion      = "us-east-1"
	DefaultMaxFileSize = int64(100 * 1024 * 1024) // 100 MB
)

// Config holds storage configuration.
type Config struct {
	// Provider selects the storage backend: "local" or "s3".
	Provider string `mapstructure:"provider" json:"provider"`

	// BasePath is the root directory for local storage.
	BasePath string `mapstructure:"base_path" json:"base_path"`

	// Bucket is the S3 bucket name.
	Bucket string `mapstructure:"bucket" json:"bucket"`

	// Region is the AWS region for S3.
	Region string `mapstructure:"region" json:"region"`

	// Endpoint is a custom S3-compatible endpoint (e.g. MinIO).
	Endpoint string `mapstructure:"endpoint" json:"endpoint"`

	// AccessKey is the AWS access key ID.
	AccessKey string `mapstructure:"access_key" json:"access_key"`

	// SecretKey is the AWS secret access key.
	SecretKey string `mapstructure:"secret_key" json:"secret_key"`

	// URL is the base project URL for Supabase storage.
	URL string `mapstructure:"url" json:"url"`

	// MaxFileSize is the maximum allowed file size in bytes.
	MaxFileSize int64 `mapstructure:"max_file_size" json:"max_file_size"`

	// Enabled controls whether the storage component is active.
	Enabled bool `mapstructure:"enabled" json:"enabled"`
}

// ApplyDefaults fills in zero-valued fields with sensible defaults.
func (c *Config) ApplyDefaults() {
	if c.Provider == "" {
		c.Provider = DefaultProvider
	}
	if c.BasePath == "" {
		c.BasePath = DefaultBasePath
	}
	if c.Region == "" {
		c.Region = DefaultRegion
	}
	if c.MaxFileSize <= 0 {
		c.MaxFileSize = DefaultMaxFileSize
	}
}

// Validate checks that the configuration is valid for the selected provider.
func (c *Config) Validate() error {
	switch c.Provider {
	case ProviderLocal:
		if c.BasePath == "" {
			return errors.New("storage: base_path is required for local provider")
		}
	case ProviderS3:
		var errs []error
		if c.Bucket == "" {
			errs = append(errs, errors.New("storage: bucket is required for s3 provider"))
		}
		if c.Region == "" {
			errs = append(errs, errors.New("storage: region is required for s3 provider"))
		}
		if len(errs) > 0 {
			return fmt.Errorf("storage: invalid s3 config: %w", errors.Join(errs...))
		}
	case ProviderSupabase:
		var errs []error
		if c.URL == "" {
			errs = append(errs, errors.New("storage: url is required for supabase provider"))
		}
		if c.Bucket == "" {
			errs = append(errs, errors.New("storage: bucket is required for supabase provider"))
		}
		if c.SecretKey == "" {
			errs = append(errs, errors.New("storage: secret_key is required for supabase provider"))
		}
		if len(errs) > 0 {
			return fmt.Errorf("storage: invalid supabase config: %w", errors.Join(errs...))
		}
	default:
		return fmt.Errorf("storage: unsupported provider %q", c.Provider)
	}
	return nil
}
