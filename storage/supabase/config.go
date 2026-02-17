package supabase

import (
	"errors"
	"fmt"
)

// ApplyDefaults fills in zero-valued fields with sensible defaults.
func (c *Config) ApplyDefaults() {
	// No defaults to apply — all fields are required.
}

// Validate checks that the Supabase configuration is valid.
func (c *Config) Validate() error {
	var errs []error
	if c.URL == "" {
		errs = append(errs, errors.New("supabase: url is required"))
	}
	if c.Bucket == "" {
		errs = append(errs, errors.New("supabase: bucket is required"))
	}
	if c.SecretKey == "" {
		errs = append(errs, errors.New("supabase: secret_key is required"))
	}
	if len(errs) > 0 {
		return fmt.Errorf("supabase: invalid config: %w", errors.Join(errs...))
	}
	return nil
}

// GetBucket returns the bucket name.
func (c *Config) GetBucket() string { return c.Bucket }
