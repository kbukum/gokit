package providers

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"time"
)

// Apple OAuth2/OIDC endpoint defaults.
const (
	AppleAuthEndpoint  = "https://appleid.apple.com/auth/authorize"
	AppleTokenEndpoint = "https://appleid.apple.com/auth/token" //nolint:gosec // OAuth endpoint URL, not a credential
	AppleAudience      = "https://appleid.apple.com"
)

// AppleDefaultScopes are the standard scopes for Sign in with Apple.
var AppleDefaultScopes = []string{"name", "email"}

// AppleConfig extends ProviderConfig with Apple-specific fields needed
// for JWT-based client secret generation.
type AppleConfig struct {
	ProviderConfig

	// TeamID is the Apple Developer Team ID.
	TeamID string

	// KeyID is the ID of the private key from Apple Developer portal.
	KeyID string

	// PrivateKey is the PEM-encoded private key content (P256 / ES256).
	PrivateKey string
}

// NewApple creates a Sign in with Apple provider.
// Default scopes: name, email.
//
// Apple is OIDC-based but with unique requirements:
//   - Client secret is a JWT signed with your private key (ES256)
//   - User info comes from the ID token (no UserInfo endpoint)
//   - User's name is only sent on the first authorization (in the `user` form field)
func NewApple(cfg AppleConfig) *GenericProvider {
	gcfg := GenericConfig{
		ProviderConfig:    WithDefaultScopes(cfg.ProviderConfig, AppleDefaultScopes...),
		ProviderName:      "apple",
		Label:             "Apple",
		Type:              "identity",
		AuthEndpoint:      AppleAuthEndpoint,
		TokenEndpoint:     AppleTokenEndpoint,
		IDTokenAsUserInfo: true,
		AuthExtraParams: map[string]string{
			"response_mode": "form_post",
		},
	}

	// If private key is provided, use JWT-based dynamic secret generation.
	// Otherwise fall back to static client secret (for testing/simple setups).
	if cfg.PrivateKey != "" {
		gcfg.ClientSecretFunc = newAppleSecretFunc(cfg)
	}

	return NewGeneric(gcfg)
}

// newAppleSecretFunc returns a closure that generates Apple's JWT-based client secret.
func newAppleSecretFunc(cfg AppleConfig) func() (string, error) {
	return func() (string, error) {
		block, _ := pem.Decode([]byte(cfg.PrivateKey))
		if block == nil {
			return "", fmt.Errorf("apple: failed to decode PEM private key")
		}

		parsedKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return "", fmt.Errorf("apple: parse private key: %w", err)
		}

		ecKey, ok := parsedKey.(*ecdsa.PrivateKey)
		if !ok {
			return "", fmt.Errorf("apple: private key is not ECDSA")
		}

		now := time.Now()
		header := map[string]interface{}{
			"alg": "ES256",
			"kid": cfg.KeyID,
		}
		claims := map[string]interface{}{
			"iss": cfg.TeamID,
			"iat": now.Unix(),
			"exp": now.Add(5 * time.Minute).Unix(),
			"aud": AppleAudience,
			"sub": cfg.ClientID,
		}

		headerJSON, _ := json.Marshal(header)
		claimsJSON, _ := json.Marshal(claims)
		headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
		claimsB64 := base64.RawURLEncoding.EncodeToString(claimsJSON)
		signingInput := headerB64 + "." + claimsB64

		hash := sha256.Sum256([]byte(signingInput))
		r, s, err := ecdsa.Sign(rand.Reader, ecKey, hash[:])
		if err != nil {
			return "", fmt.Errorf("apple: sign: %w", err)
		}

		curveBits := ecKey.Curve.Params().BitSize
		keyBytes := curveBits / 8
		rBytes := r.Bytes()
		sBytes := s.Bytes()
		sig := make([]byte, 2*keyBytes)
		copy(sig[keyBytes-len(rBytes):keyBytes], rBytes)
		copy(sig[2*keyBytes-len(sBytes):2*keyBytes], sBytes)

		sigB64 := base64.RawURLEncoding.EncodeToString(sig)
		return signingInput + "." + sigB64, nil
	}
}
