package jwt

import (
	"crypto/ecdsa"
	"crypto/rsa"
	"errors"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"
)

// SigningMethod defines supported JWT signing algorithms.
type SigningMethod string

const (
	HS256 SigningMethod = "HS256"
	HS384 SigningMethod = "HS384"
	HS512 SigningMethod = "HS512"
	RS256 SigningMethod = "RS256"
	RS384 SigningMethod = "RS384"
	RS512 SigningMethod = "RS512"
	ES256 SigningMethod = "ES256"
	ES384 SigningMethod = "ES384"
	ES512 SigningMethod = "ES512"
)

// Config configures the JWT token service.
// Loadable from YAML/env via mapstructure tags.
type Config struct {
	// Secret is the HMAC signing key (required for HS* methods).
	Secret string `mapstructure:"secret"`

	// RefreshSecret is an optional separate secret for refresh tokens.
	// If empty, Secret is used for both access and refresh tokens.
	RefreshSecret string `mapstructure:"refresh_secret"`

	// PrivateKeyPath is the path to an RSA or ECDSA private key PEM file.
	// Used for RS*/ES* methods. Alternative to Secret for asymmetric signing.
	PrivateKeyPath string `mapstructure:"private_key_path"`

	// PublicKeyPath is the path to the corresponding public key PEM file.
	// If empty, the public key is derived from the private key.
	PublicKeyPath string `mapstructure:"public_key_path"`

	// Method is the signing algorithm (default: "HS256").
	Method SigningMethod `mapstructure:"method"`

	// Issuer is the "iss" claim value (optional).
	Issuer string `mapstructure:"issuer"`

	// Audience is the "aud" claim value (optional).
	Audience []string `mapstructure:"audience"`

	// AccessTokenTTL is the lifetime of access tokens (default: "15m").
	AccessTokenTTL time.Duration `mapstructure:"access_token_ttl"`

	// RefreshTokenTTL is the lifetime of refresh tokens (default: "168h" / 7 days).
	RefreshTokenTTL time.Duration `mapstructure:"refresh_token_ttl"`

	// --- Runtime fields (not from config files) ---

	// PrivateKey is the parsed RSA or ECDSA private key (set programmatically).
	PrivateKey interface{} `mapstructure:"-"`

	// PublicKey is the parsed RSA or ECDSA public key (set programmatically).
	PublicKey interface{} `mapstructure:"-"`
}

// ApplyDefaults sets sensible defaults for zero-valued fields.
func (c *Config) ApplyDefaults() {
	if c.Method == "" {
		c.Method = HS256
	}
	if c.AccessTokenTTL == 0 {
		c.AccessTokenTTL = 15 * time.Minute
	}
	if c.RefreshTokenTTL == 0 {
		c.RefreshTokenTTL = 7 * 24 * time.Hour
	}
}

// Validate checks required fields based on the signing method.
func (c *Config) Validate() error {
	switch c.Method {
	case HS256, HS384, HS512:
		if c.Secret == "" {
			return errors.New("secret is required for HMAC signing methods")
		}
	case RS256, RS384, RS512:
		if c.PrivateKey == nil && c.PrivateKeyPath == "" {
			return errors.New("private_key or private_key_path is required for RSA signing methods")
		}
		if c.PrivateKey != nil {
			if _, ok := c.PrivateKey.(*rsa.PrivateKey); !ok {
				return errors.New("private_key must be *rsa.PrivateKey for RSA signing methods")
			}
		}
	case ES256, ES384, ES512:
		if c.PrivateKey == nil && c.PrivateKeyPath == "" {
			return errors.New("private_key or private_key_path is required for ECDSA signing methods")
		}
		if c.PrivateKey != nil {
			if _, ok := c.PrivateKey.(*ecdsa.PrivateKey); !ok {
				return errors.New("private_key must be *ecdsa.PrivateKey for ECDSA signing methods")
			}
		}
	default:
		return errors.New("unsupported signing method: " + string(c.Method))
	}
	return nil
}

// signingMethod returns the golang-jwt SigningMethod instance.
func (c *Config) signingMethod() gojwt.SigningMethod {
	switch c.Method {
	case HS256:
		return gojwt.SigningMethodHS256
	case HS384:
		return gojwt.SigningMethodHS384
	case HS512:
		return gojwt.SigningMethodHS512
	case RS256:
		return gojwt.SigningMethodRS256
	case RS384:
		return gojwt.SigningMethodRS384
	case RS512:
		return gojwt.SigningMethodRS512
	case ES256:
		return gojwt.SigningMethodES256
	case ES384:
		return gojwt.SigningMethodES384
	case ES512:
		return gojwt.SigningMethodES512
	default:
		return gojwt.SigningMethodHS256
	}
}

// signKey returns the key used for signing tokens.
func (c *Config) signKey() interface{} {
	switch c.Method {
	case HS256, HS384, HS512:
		return []byte(c.Secret)
	default:
		return c.PrivateKey
	}
}

// refreshSignKey returns the key for signing refresh tokens.
func (c *Config) refreshSignKey() interface{} {
	switch c.Method {
	case HS256, HS384, HS512:
		if c.RefreshSecret != "" {
			return []byte(c.RefreshSecret)
		}
		return []byte(c.Secret)
	default:
		return c.PrivateKey
	}
}

// verifyKey returns the key used for verifying tokens.
func (c *Config) verifyKey() interface{} {
	switch c.Method {
	case HS256, HS384, HS512:
		return []byte(c.Secret)
	case RS256, RS384, RS512:
		if c.PublicKey != nil {
			return c.PublicKey
		}
		if pk, ok := c.PrivateKey.(*rsa.PrivateKey); ok {
			return &pk.PublicKey
		}
		return c.PrivateKey
	case ES256, ES384, ES512:
		if c.PublicKey != nil {
			return c.PublicKey
		}
		if pk, ok := c.PrivateKey.(*ecdsa.PrivateKey); ok {
			return &pk.PublicKey
		}
		return c.PrivateKey
	default:
		return []byte(c.Secret)
	}
}

// refreshVerifyKey returns the key for verifying refresh tokens.
func (c *Config) refreshVerifyKey() interface{} {
	switch c.Method {
	case HS256, HS384, HS512:
		if c.RefreshSecret != "" {
			return []byte(c.RefreshSecret)
		}
		return []byte(c.Secret)
	default:
		return c.verifyKey()
	}
}
