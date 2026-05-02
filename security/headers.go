package security

import (
	"fmt"
	"net/http"
	"strconv"
	"time"
)

const (
	defaultHSTSMaxAge          = 2 * 365 * 24 * time.Hour
	defaultCSP                 = "default-src 'self'; base-uri 'self'; frame-ancestors 'none'; object-src 'none'"
	defaultReferrerPolicy      = "strict-origin-when-cross-origin"
	defaultPermissionsPolicy   = "accelerometer=(), camera=(), geolocation=(), gyroscope=(), magnetometer=(), microphone=(), payment=(), usb=()"
	defaultXFrameOptions       = "DENY"
	xContentTypeOptionsNoSniff = "nosniff"
)

// HeadersConfig configures secure-by-default HTTP response headers.
type HeadersConfig struct {
	// Disabled turns off header injection entirely.
	Disabled bool `yaml:"disabled" mapstructure:"disabled"`

	// HSTSMaxAge controls the Strict-Transport-Security max-age value.
	HSTSMaxAge time.Duration `yaml:"hsts_max_age" mapstructure:"hsts_max_age"`

	// DisableHSTSIncludeSubdomains suppresses the includeSubDomains directive.
	DisableHSTSIncludeSubdomains bool `yaml:"disable_hsts_include_subdomains" mapstructure:"disable_hsts_include_subdomains"`

	// DisableHSTSPreload suppresses the preload directive.
	DisableHSTSPreload bool `yaml:"disable_hsts_preload" mapstructure:"disable_hsts_preload"`

	// ContentSecurityPolicy is written to the Content-Security-Policy header.
	ContentSecurityPolicy string `yaml:"content_security_policy" mapstructure:"content_security_policy"`

	// ReferrerPolicy is written to the Referrer-Policy header.
	ReferrerPolicy string `yaml:"referrer_policy" mapstructure:"referrer_policy"`

	// PermissionsPolicy is written to the Permissions-Policy header.
	PermissionsPolicy string `yaml:"permissions_policy" mapstructure:"permissions_policy"`

	// XFrameOptions is written to the X-Frame-Options header.
	XFrameOptions string `yaml:"x_frame_options" mapstructure:"x_frame_options"`
}

// ApplyDefaults populates secure defaults.
func (c *HeadersConfig) ApplyDefaults() {
	if c.HSTSMaxAge == 0 {
		c.HSTSMaxAge = defaultHSTSMaxAge
	}
	if c.ContentSecurityPolicy == "" {
		c.ContentSecurityPolicy = defaultCSP
	}
	if c.ReferrerPolicy == "" {
		c.ReferrerPolicy = defaultReferrerPolicy
	}
	if c.PermissionsPolicy == "" {
		c.PermissionsPolicy = defaultPermissionsPolicy
	}
	if c.XFrameOptions == "" {
		c.XFrameOptions = defaultXFrameOptions
	}
}

// Validate checks configuration consistency.
func (c *HeadersConfig) Validate() error {
	if c == nil || c.Disabled {
		return nil
	}
	if c.HSTSMaxAge < 0 {
		return fmt.Errorf("security/headers: hsts_max_age must be >= 0")
	}
	switch c.XFrameOptions {
	case "", "DENY", "SAMEORIGIN":
	default:
		return fmt.Errorf("security/headers: x_frame_options must be DENY or SAMEORIGIN")
	}
	if c.ContentSecurityPolicy == "" {
		return fmt.Errorf("security/headers: content_security_policy must not be empty")
	}
	if c.ReferrerPolicy == "" {
		return fmt.Errorf("security/headers: referrer_policy must not be empty")
	}
	return nil
}

// HeaderMap returns the configured security headers.
func (c *HeadersConfig) HeaderMap() (map[string]string, error) {
	cfg := HeadersConfig{}
	if c != nil {
		cfg = *c
	}
	if cfg.Disabled {
		return map[string]string{}, nil
	}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	headers := map[string]string{
		"Content-Security-Policy":   cfg.ContentSecurityPolicy,
		"Referrer-Policy":           cfg.ReferrerPolicy,
		"Permissions-Policy":        cfg.PermissionsPolicy,
		"X-Content-Type-Options":    xContentTypeOptionsNoSniff,
		"X-Frame-Options":           cfg.XFrameOptions,
		"Strict-Transport-Security": buildHSTSValue(cfg),
	}
	return headers, nil
}

// Apply writes the configured security headers onto the provided response header map.
func (c *HeadersConfig) Apply(header http.Header) error {
	headers, err := c.HeaderMap()
	if err != nil {
		return err
	}
	for key, value := range headers {
		header.Set(key, value)
	}
	return nil
}

func buildHSTSValue(cfg HeadersConfig) string {
	parts := []string{"max-age=" + strconv.FormatInt(int64(cfg.HSTSMaxAge/time.Second), 10)}
	if !cfg.DisableHSTSIncludeSubdomains {
		parts = append(parts, "includeSubDomains")
	}
	if !cfg.DisableHSTSPreload {
		parts = append(parts, "preload")
	}
	return joinHeaderDirectives(parts)
}

func joinHeaderDirectives(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for _, part := range parts[1:] {
		result += "; " + part
	}
	return result
}
