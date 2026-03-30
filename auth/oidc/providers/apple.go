package providers

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"strings"
	"time"

	"github.com/kbukum/gokit/auth/oidc"
)

const (
	appleAuthURL  = "https://appleid.apple.com/auth/authorize"
	appleTokenURL = "https://appleid.apple.com/auth/token"
)

// AppleConfig extends ProviderConfig with Apple-specific fields.
type AppleConfig struct {
	ProviderConfig

	// TeamID is the Apple Developer Team ID.
	TeamID string

	// KeyID is the ID of the private key from Apple Developer portal.
	KeyID string

	// PrivateKey is the PEM-encoded private key content (P256 / ES256).
	PrivateKey string
}

// Apple implements the oidc.Provider interface for Sign in with Apple.
// Apple uses OIDC but has unique requirements:
//   - Client secret is a JWT signed with your private key
//   - User info is delivered in the ID token (no UserInfo endpoint)
//   - User name is only sent on first authorization (in `user` form field)
type Apple struct {
	cfg AppleConfig
}

// NewApple creates a new Apple provider.
// Default scopes: name, email.
func NewApple(cfg AppleConfig) *Apple {
	if len(cfg.Scopes) == 0 {
		cfg.Scopes = []string{"name", "email"}
	}
	return &Apple{cfg: cfg}
}

func (a *Apple) Name() string { return "apple" }

func (a *Apple) AuthURL(state string, opts ...oidc.AuthURLOption) string {
	o := oidc.ApplyAuthURLOptions(opts)
	return buildAuthURL(appleAuthURL, a.cfg.ProviderConfig, state, o, map[string]string{
		"response_mode": "form_post",
	})
}

func (a *Apple) Exchange(ctx context.Context, code string, opts ...oidc.ExchangeOption) (*oidc.TokenResult, error) {
	clientSecret, err := a.generateClientSecret()
	if err != nil {
		return nil, fmt.Errorf("apple: generate client secret: %w", err)
	}

	cfg := a.cfg.ProviderConfig
	cfg.ClientSecret = clientSecret

	o := oidc.ApplyExchangeOptions(opts)
	tok, err := exchangeCode(ctx, appleTokenURL, cfg, code, o, nil)
	if err != nil {
		return nil, fmt.Errorf("apple: %w", err)
	}
	return toTokenResult(tok), nil
}

func (a *Apple) UserInfo(_ context.Context, _ string) (*oidc.UserInfo, error) {
	return nil, fmt.Errorf("apple: UserInfo endpoint not supported; use ParseIDTokenClaims on the ID token")
}

// ParseIDTokenClaims extracts user info from an Apple ID token without
// full signature verification (the token was just received from Apple).
// For production, use the oidc.Verifier to verify the token first.
func ParseIDTokenClaims(idToken string) (*oidc.UserInfo, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("apple: invalid ID token format")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("apple: decode ID token payload: %w", err)
	}

	var claims struct {
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified any    `json:"email_verified"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("apple: unmarshal ID token: %w", err)
	}

	emailVerified := false
	switch v := claims.EmailVerified.(type) {
	case bool:
		emailVerified = v
	case string:
		emailVerified = v == "true"
	}

	return &oidc.UserInfo{
		Subject:       claims.Sub,
		Email:         claims.Email,
		EmailVerified: emailVerified,
	}, nil
}

// generateClientSecret creates a signed JWT client secret for Apple.
// Apple requires the client_secret to be a JWT signed with ES256.
func (a *Apple) generateClientSecret() (string, error) {
	if a.cfg.PrivateKey == "" {
		return a.cfg.ClientSecret, nil
	}

	block, _ := pem.Decode([]byte(a.cfg.PrivateKey))
	if block == nil {
		return "", fmt.Errorf("failed to decode PEM private key")
	}

	parsedKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("parse private key: %w", err)
	}

	ecKey, ok := parsedKey.(*ecdsa.PrivateKey)
	if !ok {
		return "", fmt.Errorf("private key is not ECDSA")
	}

	now := time.Now()
	header := map[string]interface{}{
		"alg": "ES256",
		"kid": a.cfg.KeyID,
	}
	claims := map[string]interface{}{
		"iss": a.cfg.TeamID,
		"iat": now.Unix(),
		"exp": now.Add(5 * time.Minute).Unix(),
		"aud": "https://appleid.apple.com",
		"sub": a.cfg.ClientID,
	}

	headerJSON, _ := json.Marshal(header)
	claimsJSON, _ := json.Marshal(claims)
	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	claimsB64 := base64.RawURLEncoding.EncodeToString(claimsJSON)
	signingInput := headerB64 + "." + claimsB64

	hash := sha256.Sum256([]byte(signingInput))
	r, s, err := ecdsa.Sign(rand.Reader, ecKey, hash[:])
	if err != nil {
		return "", fmt.Errorf("sign: %w", err)
	}

	// ES256 signature: r and s as 32-byte big-endian concatenated
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

var _ oidc.Provider = (*Apple)(nil)
