package oidc

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Config configures OIDC provider verification.
// Loadable from YAML/env via mapstructure tags.
type Config struct {
	// Enabled controls whether OIDC verification is active.
	Enabled bool `mapstructure:"enabled"`

	// Issuer is the OIDC provider's issuer URL (e.g., "https://accounts.google.com").
	// Used for auto-discovery of JWKS endpoint, authorization endpoint, etc.
	Issuer string `mapstructure:"issuer"`

	// ClientID is the OAuth2 client ID (also used as expected "aud" claim).
	ClientID string `mapstructure:"client_id"`

	// ClientSecret is the OAuth2 client secret (for confidential clients).
	ClientSecret string `mapstructure:"client_secret"`

	// RedirectURL is the OAuth2 callback URL.
	RedirectURL string `mapstructure:"redirect_url"`

	// PublicClient enables OAuth 2.1 public-client validation rules.
	PublicClient bool `mapstructure:"public_client"`

	// RequirePKCE enforces PKCE for authorization-code flows.
	RequirePKCE bool `mapstructure:"require_pkce"`

	// Scopes are the OAuth2 scopes to request (default: ["openid", "email", "profile"]).
	Scopes []string `mapstructure:"scopes"`

	// SupportedSigningAlgs restricts allowed ID token signing algorithms
	// (default: ["RS256", "ES256", "EdDSA"]).
	SupportedSigningAlgs []string `mapstructure:"supported_signing_algs"`

	// JWKSCacheDuration controls how long JWKS keys are cached (default: "1h").
	JWKSCacheDuration time.Duration `mapstructure:"jwks_cache_duration"`

	// HTTPTimeout is the timeout for discovery and JWKS HTTP requests (default: "10s").
	HTTPTimeout time.Duration `mapstructure:"http_timeout"`

	// SkipIssuerCheck skips issuer validation (for testing only).
	SkipIssuerCheck bool `mapstructure:"skip_issuer_check"`
}

// ApplyDefaults sets sensible defaults for zero-valued fields.
func (c *Config) ApplyDefaults() {
	if len(c.Scopes) == 0 {
		c.Scopes = []string{"openid", "email", "profile"}
	}
	if c.PublicClient && !c.RequirePKCE {
		c.RequirePKCE = true
	}
	if len(c.SupportedSigningAlgs) == 0 {
		c.SupportedSigningAlgs = []string{"RS256", "ES256", "EdDSA"}
	}
	if c.JWKSCacheDuration == 0 {
		c.JWKSCacheDuration = time.Hour
	}
	if c.HTTPTimeout == 0 {
		c.HTTPTimeout = 10 * time.Second
	}
}

// Validate checks required fields.
func (c *Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	if c.Issuer == "" {
		return fmt.Errorf("issuer is required")
	}
	if c.ClientID == "" {
		return fmt.Errorf("client_id is required")
	}
	if c.RedirectURL != "" {
		parsed, err := url.Parse(c.RedirectURL)
		if err != nil || !parsed.IsAbs() || parsed.Fragment != "" || strings.Contains(c.RedirectURL, "*") {
			return fmt.Errorf("redirect_url must be an exact absolute URI without fragments or wildcards")
		}
	}
	if c.PublicClient && c.ClientSecret != "" {
		return fmt.Errorf("public clients must not configure client_secret")
	}
	return nil
}

// ToVerifierConfig converts to a VerifierConfig for creating a Verifier.
func (c *Config) ToVerifierConfig() VerifierConfig {
	return VerifierConfig{
		ClientID:             c.ClientID,
		SupportedSigningAlgs: c.SupportedSigningAlgs,
		JWKSCacheDuration:    c.JWKSCacheDuration,
		SkipIssuerCheck:      c.SkipIssuerCheck,
	}
}
