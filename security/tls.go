package security

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
)

// TLSConfig holds TLS settings shared across gokit modules.
// Used by httpclient, grpc, kafka, discovery, and other transport layers.
type TLSConfig struct {
	// SkipVerify disables server certificate verification.
	// Not recommended for production.
	SkipVerify bool `yaml:"skip_verify" mapstructure:"skip_verify"`

	// CAFile is the path to the CA certificate file for verifying the server.
	CAFile string `yaml:"ca_file" mapstructure:"ca_file"`

	// CertFile is the path to the client TLS certificate file (for mTLS).
	CertFile string `yaml:"cert_file" mapstructure:"cert_file"`

	// KeyFile is the path to the client TLS key file (for mTLS).
	KeyFile string `yaml:"key_file" mapstructure:"key_file"`

	// ServerName overrides the server name used for certificate verification.
	ServerName string `yaml:"server_name" mapstructure:"server_name"`

	// MinVersion is the minimum TLS version (e.g., tls.VersionTLS12).
	// Defaults to TLS 1.2 if not set.
	MinVersion uint16 `yaml:"min_version" mapstructure:"min_version"`
}

// Build creates a *tls.Config from the configuration.
// Returns nil if no TLS settings are configured (all fields are zero values).
func (c *TLSConfig) Build() (*tls.Config, error) {
	if c == nil {
		return nil, nil
	}

	// Check if any TLS option is set
	if !c.hasSettings() {
		return nil, nil
	}

	minVersion := c.MinVersion
	if minVersion == 0 {
		minVersion = tls.VersionTLS12
	}

	cfg := &tls.Config{
		InsecureSkipVerify: c.SkipVerify,
		ServerName:         c.ServerName,
		MinVersion:         minVersion,
	}

	if err := c.loadCA(cfg); err != nil {
		return nil, err
	}

	if err := c.loadClientCert(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate checks that the TLS configuration is consistent.
func (c *TLSConfig) Validate() error {
	if c == nil {
		return nil
	}
	// If one of cert/key is set, both must be set
	if (c.CertFile != "") != (c.KeyFile != "") {
		return fmt.Errorf("security/tls: both cert_file and key_file must be provided together")
	}
	return nil
}

// IsEnabled returns true if any TLS setting is configured.
func (c *TLSConfig) IsEnabled() bool {
	if c == nil {
		return false
	}
	return c.hasSettings()
}

// hasSettings checks if any TLS field is set.
func (c *TLSConfig) hasSettings() bool {
	return c.SkipVerify || c.CAFile != "" || c.CertFile != "" || c.ServerName != ""
}

// loadCA loads the CA certificate into the TLS config.
func (c *TLSConfig) loadCA(cfg *tls.Config) error {
	if c.CAFile == "" {
		return nil
	}
	ca, err := os.ReadFile(c.CAFile)
	if err != nil {
		return fmt.Errorf("security/tls: failed to read CA file: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		return fmt.Errorf("security/tls: failed to parse CA certificate")
	}
	cfg.RootCAs = pool
	return nil
}

// loadClientCert loads the client certificate and key into the TLS config.
func (c *TLSConfig) loadClientCert(cfg *tls.Config) error {
	if c.CertFile == "" || c.KeyFile == "" {
		return nil
	}
	cert, err := tls.LoadX509KeyPair(c.CertFile, c.KeyFile)
	if err != nil {
		return fmt.Errorf("security/tls: failed to load client certificate: %w", err)
	}
	cfg.Certificates = []tls.Certificate{cert}
	return nil
}
