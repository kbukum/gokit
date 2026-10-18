package s3

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// DefaultRegion is the default AWS region.
const DefaultRegion = "us-east-1"

// Config holds S3-specific storage configuration.
type Config struct {
	// Bucket is the S3 bucket name.
	Bucket string `mapstructure:"bucket" json:"bucket"`

	// Region is the AWS region.
	Region string `mapstructure:"region" json:"region"`

	// Endpoint is a custom S3-compatible origin, https://host[:port], without credentials, path, query or fragment.
	Endpoint string `mapstructure:"endpoint" json:"endpoint"`

	// AllowInsecureDev allows an http:// Endpoint for local development.
	AllowInsecureDev bool `mapstructure:"allow_insecure_dev" json:"allow_insecure_dev"`

	// Resolve is the discovery service name for the S3 endpoint. Empty = use static Endpoint.
	// Set = resolve from discovery provider.
	Resolve string `mapstructure:"resolve" json:"resolve"`

	// AccessKeyID is the AWS access key ID.
	AccessKeyID string `mapstructure:"access_key_id" json:"access_key_id"`

	// SecretAccessKey is the AWS secret access key.
	SecretAccessKey string `mapstructure:"secret_access_key" json:"secret_access_key"`

	// AccessKeyFile and SecretKeyFile select bounded mounted credentials instead of inline values or AWS discovery.
	AccessKeyFile string `mapstructure:"access_key_file" json:"access_key_file"`
	SecretKeyFile string `mapstructure:"secret_key_file" json:"secret_key_file"`

	// ForcePathStyle forces path-style URLs instead of virtual-hosted-style.
	ForcePathStyle bool `mapstructure:"force_path_style" json:"force_path_style"`
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
	if (c.AccessKeyFile == "") != (c.SecretKeyFile == "") || (c.AccessKeyID == "") != (c.SecretAccessKey == "") {
		errs = append(errs, errors.New("s3: both access and secret credential sources are required"))
	}
	if (c.AccessKeyFile != "" || c.SecretKeyFile != "") && (c.AccessKeyID != "" || c.SecretAccessKey != "") {
		errs = append(errs, errors.New("s3: mounted and inline credentials cannot be mixed"))
	}
	if c.Endpoint != "" {
		if err := validateEndpoint(c.Endpoint, c.AllowInsecureDev); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("s3: invalid config: %w", errors.Join(errs...))
	}
	return nil
}

func validateEndpoint(raw string, allowHTTP bool) error {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.Opaque != "" || u.User != nil || u.RawQuery != "" || u.ForceQuery ||
		strings.Contains(raw, "#") || (u.Path != "" && u.Path != "/") || u.RawPath != "" ||
		strings.HasSuffix(u.Hostname(), ".") || strings.HasSuffix(u.Host, ":") {
		return errors.New("s3: endpoint must be a credential-free origin without path, query or fragment")
	}
	if port := u.Port(); port != "" {
		if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
			return errors.New("s3: endpoint port must be between 1 and 65535")
		}
	}
	if u.Scheme != "https" && (!allowHTTP || u.Scheme != "http") {
		return errors.New("s3: endpoint must use https unless allow_insecure_dev is set for http")
	}
	return nil
}

// GetBucket returns the bucket name.
func (c *Config) GetBucket() string { return c.Bucket }
