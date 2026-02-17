package s3

import (
	"errors"
	"fmt"
)

// DefaultRegion is the default AWS region.
const DefaultRegion = "us-east-1"

// Config holds S3-specific storage configuration.
type Config struct {
	// Bucket is the S3 bucket name.
	Bucket string `mapstructure:"bucket" json:"bucket"`

	// Region is the AWS region.
	Region string `mapstructure:"region" json:"region"`

	// Endpoint is a custom S3-compatible endpoint (e.g. MinIO).
	Endpoint string `mapstructure:"endpoint" json:"endpoint"`

	// AccessKey is the AWS access key ID.
	AccessKey string `mapstructure:"access_key" json:"access_key"`

	// SecretKey is the AWS secret access key.
	SecretKey string `mapstructure:"secret_key" json:"secret_key"`

	// ForcePathStyle forces path-style URLs instead of virtual-hosted-style.
	ForcePathStyle bool `mapstructure:"force_path_style" json:"force_path_style"`

	// UseSSL enables SSL/TLS connections.
	UseSSL bool `mapstructure:"use_ssl" json:"use_ssl"`
}

// ApplyDefaults fills in zero-valued fields with sensible defaults.
func (c *Config) ApplyDefaults() {
	if c.Region == "" {
		c.Region = DefaultRegion
	}
}

// Validate checks that the S3 configuration is valid.
func (c *Config) Validate() error {
	var errs []error
	if c.Bucket == "" {
		errs = append(errs, errors.New("s3: bucket is required"))
	}
	if c.Region == "" {
		errs = append(errs, errors.New("s3: region is required"))
	}
	if len(errs) > 0 {
		return fmt.Errorf("s3: invalid config: %w", errors.Join(errs...))
	}
	return nil
}

// GetBucket returns the bucket name.
func (c *Config) GetBucket() string { return c.Bucket }
