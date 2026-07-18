package jwt

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"errors"
	"fmt"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"
)

// SigningMethod defines the supported JWT signing algorithms.
type SigningMethod string

const (
	// HS256 is only permitted for explicitly opted-in internal HMAC deployments.
	HS256 SigningMethod = "HS256"

	// RS256 is the default asymmetric signing method for service-issued tokens.
	RS256 SigningMethod = "RS256"

	// ES256 is the supported ECDSA signing method.
	ES256 SigningMethod = "ES256"

	// EdDSA uses Ed25519 keys.
	EdDSA SigningMethod = "EdDSA"
)

const (
	defaultAccessTokenTTL  = 15 * time.Minute
	defaultRefreshTokenTTL = 7 * 24 * time.Hour
	defaultClockSkew       = 30 * time.Second
	maxClockSkew           = time.Minute
	minHMACSecretLength    = 32
)

// Config configures the JWT token service. Loadable from YAML/env via mapstructure tags.
type Config struct {
	// Secret is the HMAC signing key (required for HS256).
	Secret string `mapstructure:"secret"`

	// RefreshSecret is an optional separate secret for refresh tokens. If empty,
	// Secret is used for both access and refresh tokens.
	RefreshSecret string `mapstructure:"refresh_secret"`

	// PrivateKeyPath is the path to an RSA, ECDSA, or Ed25519 private key PEM file.
	// Used for RS256/ES256/EdDSA methods.
	PrivateKeyPath string `mapstructure:"private_key_path"`

	// PublicKeyPath is the path to the corresponding public key PEM file. If empty,
	// the public key is derived from the private key where possible.
	PublicKeyPath string `mapstructure:"public_key_path"`

	// Method is the signing algorithm (default: "RS256").
	Method SigningMethod `mapstructure:"method"`

	// AllowSymmetricHMAC explicitly opts into HS256 for internal-only deployments.
	AllowSymmetricHMAC bool `mapstructure:"allow_symmetric_hmac"`

	// Issuer is the required "iss" claim value.
	Issuer string `mapstructure:"issuer"`

	// Audience is the required "aud" claim value.
	Audience []string `mapstructure:"audience"`

	// AccessTokenTTL is the lifetime of access tokens (default: "15m").
	AccessTokenTTL time.Duration `mapstructure:"access_token_ttl"`

	// RefreshTokenTTL is the lifetime of refresh tokens (default: "168h" / 7 days).
	RefreshTokenTTL time.Duration `mapstructure:"refresh_token_ttl"`

	// ClockSkew is the accepted validation leeway for time-based claims. Secure default: 30s.
	// Maximum allowed: 60s.
	ClockSkew time.Duration `mapstructure:"clock_skew"`

	// --- Runtime fields (not from config files) ---

	// PrivateKey is the parsed RSA, ECDSA, or Ed25519 private key (set programmatically).
	PrivateKey any `mapstructure:"-"`

	// PublicKey is the parsed RSA, ECDSA, or Ed25519 public key (set programmatically).
	PublicKey any `mapstructure:"-"`
}

// ApplyDefaults sets sensible defaults for zero-valued fields.
func (c *Config) ApplyDefaults() {
	if c.Method == "" {
		c.Method = RS256
	}
	if c.AccessTokenTTL == 0 {
		c.AccessTokenTTL = defaultAccessTokenTTL
	}
	if c.RefreshTokenTTL == 0 {
		c.RefreshTokenTTL = defaultRefreshTokenTTL
	}
	if c.ClockSkew == 0 {
		c.ClockSkew = defaultClockSkew
	}
}

// Validate checks required fields based on the signing method.
func (c *Config) Validate() error {
	if c.Issuer == "" {
		return errors.New("issuer is required")
	}
	if len(c.Audience) == 0 {
		return errors.New("audience is required")
	}
	if c.ClockSkew < 0 {
		return errors.New("clock_skew must be >= 0")
	}
	if c.ClockSkew > maxClockSkew {
		return fmt.Errorf("clock_skew must be <= %s", maxClockSkew)
	}

	switch c.Method {
	case HS256:
		if !c.AllowSymmetricHMAC {
			return errors.New("HS256 requires allow_symmetric_hmac=true and is intended for internal-only deployments")
		}
		if len(c.Secret) < minHMACSecretLength {
			return fmt.Errorf("secret must be at least %d bytes for HS256", minHMACSecretLength)
		}
		if c.RefreshSecret != "" && len(c.RefreshSecret) < minHMACSecretLength {
			return fmt.Errorf("refresh_secret must be at least %d bytes for HS256", minHMACSecretLength)
		}
	case RS256:
		if c.PrivateKey == nil && c.PrivateKeyPath == "" {
			return errors.New("private_key or private_key_path is required for RS256 signing")
		}
		if c.PrivateKey != nil {
			if _, ok := c.PrivateKey.(*rsa.PrivateKey); !ok {
				return errors.New("private_key must be *rsa.PrivateKey for RS256")
			}
		}
		if c.PublicKey != nil {
			if _, ok := c.PublicKey.(*rsa.PublicKey); !ok {
				return errors.New("public_key must be *rsa.PublicKey for RS256")
			}
		}
	case ES256:
		if c.PrivateKey == nil && c.PrivateKeyPath == "" {
			return errors.New("private_key or private_key_path is required for ES256 signing")
		}
		if c.PrivateKey != nil {
			if _, ok := c.PrivateKey.(*ecdsa.PrivateKey); !ok {
				return errors.New("private_key must be *ecdsa.PrivateKey for ES256")
			}
		}
		if c.PublicKey != nil {
			if _, ok := c.PublicKey.(*ecdsa.PublicKey); !ok {
				return errors.New("public_key must be *ecdsa.PublicKey for ES256")
			}
		}
	case EdDSA:
		if c.PrivateKey == nil && c.PrivateKeyPath == "" {
			return errors.New("private_key or private_key_path is required for EdDSA signing")
		}
		if c.PrivateKey != nil {
			pk, ok := c.PrivateKey.(ed25519.PrivateKey)
			if !ok {
				return errors.New("private_key must be ed25519.PrivateKey for EdDSA")
			}
			if len(pk) != ed25519.PrivateKeySize {
				return fmt.Errorf("private_key has incorrect length for ed25519: got %d, want %d", len(pk), ed25519.PrivateKeySize)
			}
		}
		if c.PublicKey != nil {
			pk, ok := c.PublicKey.(ed25519.PublicKey)
			if !ok {
				return errors.New("public_key must be ed25519.PublicKey for EdDSA")
			}
			if len(pk) != ed25519.PublicKeySize {
				return fmt.Errorf("public_key has incorrect length for ed25519: got %d, want %d", len(pk), ed25519.PublicKeySize)
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
	case RS256:
		return gojwt.SigningMethodRS256
	case ES256:
		return gojwt.SigningMethodES256
	case EdDSA:
		return gojwt.SigningMethodEdDSA
	default:
		return gojwt.SigningMethodRS256
	}
}

// signKey returns the key used for signing tokens.
func (c *Config) signKey() any {
	switch c.Method {
	case HS256:
		return []byte(c.Secret)
	default:
		return c.PrivateKey
	}
}

func (c *Config) refreshSignKey() any {
	if c.Method == HS256 && c.RefreshSecret != "" {
		return []byte(c.RefreshSecret)
	}
	return c.signKey()
}

// verifyKey returns the key used for verifying tokens.
func (c *Config) verifyKey() any {
	switch c.Method {
	case HS256:
		return []byte(c.Secret)
	case RS256:
		if c.PublicKey != nil {
			return c.PublicKey
		}
		if pk, ok := c.PrivateKey.(*rsa.PrivateKey); ok {
			return &pk.PublicKey
		}
		return c.PrivateKey
	case ES256:
		if c.PublicKey != nil {
			return c.PublicKey
		}
		if pk, ok := c.PrivateKey.(*ecdsa.PrivateKey); ok {
			return &pk.PublicKey
		}
		return c.PrivateKey
	case EdDSA:
		if c.PublicKey != nil {
			return c.PublicKey
		}
		if pk, ok := c.PrivateKey.(ed25519.PrivateKey); ok {
			return pk.Public().(ed25519.PublicKey)
		}
		return c.PrivateKey
	default:
		return []byte(c.Secret)
	}
}

func (c *Config) refreshVerifyKey() any {
	if c.Method == HS256 && c.RefreshSecret != "" {
		return []byte(c.RefreshSecret)
	}
	return c.verifyKey()
}
