package gcs

import (
	"errors"
	"fmt"
)

// DefaultEndpoint is the default Google Cloud Storage API endpoint.
const DefaultEndpoint = "https://storage.googleapis.com"

// Config holds Google Cloud Storage-specific configuration.
type Config struct {
	// Bucket is the Google Cloud Storage bucket name.
	Bucket string `mapstructure:"bucket" json:"bucket"`
	// ProjectID is the optional Google Cloud project ID used by the SDK.
	ProjectID string `mapstructure:"project_id" json:"project_id"`
	// Endpoint is an optional API endpoint override for emulators or tests.
	Endpoint string `mapstructure:"endpoint" json:"endpoint"`
	// CredentialsFile is an optional path to a service account JSON file.
	CredentialsFile string `mapstructure:"credentials_file" json:"credentials_file"`
	// CredentialsJSON is optional raw service account JSON.
	CredentialsJSON []byte `mapstructure:"credentials_json" json:"-"`
	// GoogleAccessID is the service account email used for signed URLs.
	GoogleAccessID string `mapstructure:"google_access_id" json:"google_access_id"`
	// PrivateKey is the PEM private key used for signed URLs.
	PrivateKey []byte `mapstructure:"private_key" json:"-"`
	// PublicURL is an optional public base URL override.
	PublicURL string `mapstructure:"public_url" json:"public_url"`
}

// ApplyDefaults fills zero-valued fields with sensible defaults.
func (c *Config) ApplyDefaults() {
	if c.Endpoint == "" {
		c.Endpoint = DefaultEndpoint
	}
}

// Validate checks that the GCS configuration is valid.
func (c *Config) Validate() error {
	var errs []error
	if c.Bucket == "" {
		errs = append(errs, errors.New("gcs: bucket is required"))
	}
	if c.CredentialsFile != "" && len(c.CredentialsJSON) > 0 {
		errs = append(errs, errors.New("gcs: credentials_file and credentials_json are mutually exclusive"))
	}
	if (c.GoogleAccessID == "") != (len(c.PrivateKey) == 0) {
		errs = append(errs, errors.New("gcs: google_access_id and private_key must be provided together"))
	}
	if len(errs) > 0 {
		return fmt.Errorf("gcs: invalid config: %w", errors.Join(errs...))
	}
	return nil
}

// GetBucket returns the bucket name.
func (c *Config) GetBucket() string { return c.Bucket }
