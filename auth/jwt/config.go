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
type Config struct {
	// Secret is the HMAC signing key (required for HS* methods).
	Secret string

	// PrivateKey is the RSA or ECDSA private key (required for RS*/ES* methods).
	PrivateKey interface{}

	// PublicKey is the RSA or ECDSA public key for verification.
	// If not set, PrivateKey is used to derive it (RSA) or the same key is used (HMAC).
	PublicKey interface{}

	// Method is the signing algorithm (default: HS256).
	Method SigningMethod

	// Issuer is the "iss" claim (optional).
	Issuer string

	// Audience is the "aud" claim (optional).
	Audience []string

	// AccessTokenTTL is the lifetime of access tokens (default: 15m).
	AccessTokenTTL time.Duration

	// RefreshTokenTTL is the lifetime of refresh tokens (default: 7d).
	RefreshTokenTTL time.Duration
}

// applyDefaults fills in zero-value fields with sensible defaults.
func (c *Config) applyDefaults() {
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

// validate checks required fields based on the signing method.
func (c *Config) validate() error {
	switch c.Method {
	case HS256, HS384, HS512:
		if c.Secret == "" {
			return errors.New("jwt: secret is required for HMAC signing methods")
		}
	case RS256, RS384, RS512:
		if c.PrivateKey == nil {
			return errors.New("jwt: private key is required for RSA signing methods")
		}
		if _, ok := c.PrivateKey.(*rsa.PrivateKey); !ok {
			return errors.New("jwt: private key must be *rsa.PrivateKey for RSA signing methods")
		}
	case ES256, ES384, ES512:
		if c.PrivateKey == nil {
			return errors.New("jwt: private key is required for ECDSA signing methods")
		}
		if _, ok := c.PrivateKey.(*ecdsa.PrivateKey); !ok {
			return errors.New("jwt: private key must be *ecdsa.PrivateKey for ECDSA signing methods")
		}
	default:
		return errors.New("jwt: unsupported signing method: " + string(c.Method))
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
