package jwt

import (
	"fmt"
	"slices"
	"strings"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"
)

// SigningMethod defines the supported JWT signing algorithms.
type SigningMethod string

const (
	// HS256 is only permitted for explicitly opted-in internal HMAC deployments.
	HS256 SigningMethod = "HS256"

	// RS256 is the default asymmetric signing method.
	RS256 SigningMethod = "RS256"

	// ES256 uses ECDSA P-256 keys.
	ES256 SigningMethod = "ES256"

	// EdDSA uses Ed25519 keys.
	EdDSA SigningMethod = "EdDSA"
)

const (
	maxLeeway        = time.Minute
	maxTypeBytes     = 64
	maxAudienceBytes = 256
)

// Config is one token profile. Every token a Service issues or accepts carries exactly this signing method, issuer,
// type and audience set, and never lives longer than MaxLifetime. Use one Service per token profile: access and refresh
// tokens, for example, are two profiles with two types. Key material is not configuration; it is supplied as a [KeySet].
type Config struct {
	// Method is the only accepted signing algorithm (default: "RS256").
	Method SigningMethod `mapstructure:"method"`

	// AllowSymmetricHMAC explicitly opts into HS256 for internal-only deployments.
	AllowSymmetricHMAC bool `mapstructure:"allow_symmetric_hmac"`

	// Issuer is the required "iss" claim value.
	Issuer string `mapstructure:"issuer"`

	// Audience lists the accepted "aud" values. A token must name at least one of them and issued tokens may name
	// only these.
	Audience []string `mapstructure:"audience"`

	// SingleAudience requires every token to carry exactly one audience.
	SingleAudience bool `mapstructure:"single_audience"`

	// Type is the required "typ" header, for example "at+jwt". It is compared case-insensitively and the
	// "application/" prefix is optional, as RFC 7515 allows.
	Type string `mapstructure:"type"`

	// MaxLifetime bounds exp - iat. Issued tokens default to it when they set no expiry.
	MaxLifetime time.Duration `mapstructure:"max_lifetime"`

	// Leeway is the accepted clock difference for exp, nbf and iat. Zero means none; at most one minute.
	Leeway time.Duration `mapstructure:"leeway"`
}

// ApplyDefaults selects RS256 when no method is set. Leeway has no default: zero means none.
func (c *Config) ApplyDefaults() {
	if c.Method == "" {
		c.Method = RS256
	}
}

// Validate checks the profile without key material.
func (c *Config) Validate() error {
	if c.Issuer == "" {
		return fmt.Errorf("issuer is required")
	}
	if len(c.Audience) == 0 {
		return fmt.Errorf("audience is required")
	}
	for i, aud := range c.Audience {
		if aud == "" || len(aud) > maxAudienceBytes {
			return fmt.Errorf("audience values must be 1..%d bytes", maxAudienceBytes)
		}
		if slices.Contains(c.Audience[:i], aud) {
			return fmt.Errorf("audience %q is listed twice", aud)
		}
	}
	if c.Type == "" || len(c.Type) > maxTypeBytes || strings.ContainsFunc(c.Type, func(r rune) bool { return r <= ' ' || r > '~' }) {
		return fmt.Errorf("type must be 1..%d visible ASCII characters", maxTypeBytes)
	}
	if c.MaxLifetime <= 0 {
		return fmt.Errorf("max_lifetime must be positive")
	}
	if c.Leeway < 0 || c.Leeway > maxLeeway {
		return fmt.Errorf("leeway must be between 0 and %s", maxLeeway)
	}
	switch c.Method {
	case RS256, ES256, EdDSA:
	case HS256:
		if !c.AllowSymmetricHMAC {
			return fmt.Errorf("HS256 requires allow_symmetric_hmac=true and is intended for internal-only deployments")
		}
	default:
		return fmt.Errorf("unsupported signing method: %q", c.Method)
	}
	return nil
}

func (c *Config) signingMethod() gojwt.SigningMethod {
	switch c.Method {
	case HS256:
		return gojwt.SigningMethodHS256
	case ES256:
		return gojwt.SigningMethodES256
	case EdDSA:
		return gojwt.SigningMethodEdDSA
	default:
		return gojwt.SigningMethodRS256
	}
}

func normalizeType(typ string) string {
	typ = strings.ToLower(typ)
	return strings.TrimPrefix(typ, "application/")
}
