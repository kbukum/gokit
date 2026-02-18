package oidc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Verifier validates OIDC ID tokens using auto-discovery and JWKS key rotation.
// It discovers the issuer's OpenID configuration and caches the JWKS for
// efficient token verification.
type Verifier struct {
	issuer   string
	clientID string
	config   VerifierConfig
	disco    *discoveryDoc
	jwks     *jwksCache
}

// VerifierConfig configures the OIDC token verifier.
type VerifierConfig struct {
	// ClientID is the expected "aud" claim in the ID token (required).
	ClientID string

	// SkipExpiryCheck skips the expiry validation (for testing only).
	SkipExpiryCheck bool

	// SkipIssuerCheck skips the issuer validation.
	SkipIssuerCheck bool

	// SupportedSigningAlgs restricts allowed signing algorithms.
	// Default: ["RS256"].
	SupportedSigningAlgs []string

	// HTTPClient is an optional HTTP client for discovery and JWKS requests.
	// Useful for testing or custom TLS configurations.
	HTTPClient *http.Client

	// JWKSCacheDuration controls how long JWKS keys are cached (default: 1h).
	JWKSCacheDuration time.Duration
}

func (c *VerifierConfig) applyDefaults() {
	if len(c.SupportedSigningAlgs) == 0 {
		c.SupportedSigningAlgs = []string{"RS256"}
	}
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{Timeout: 10 * time.Second}
	}
	if c.JWKSCacheDuration == 0 {
		c.JWKSCacheDuration = time.Hour
	}
}

// NewVerifier creates a new OIDC token verifier for the given issuer.
// It performs OIDC discovery to find the JWKS endpoint.
func NewVerifier(ctx context.Context, issuer string, cfg VerifierConfig) (*Verifier, error) {
	if cfg.ClientID == "" {
		return nil, errors.New("oidc: client ID is required")
	}
	cfg.applyDefaults()

	v := &Verifier{
		issuer:   strings.TrimRight(issuer, "/"),
		clientID: cfg.ClientID,
		config:   cfg,
	}

	if err := v.discover(ctx); err != nil {
		return nil, fmt.Errorf("oidc: discovery failed for %s: %w", issuer, err)
	}

	return v, nil
}

// Verify validates a raw ID token string and returns the parsed claims.
// It checks the signature, issuer, audience, and expiry.
func (v *Verifier) Verify(ctx context.Context, rawIDToken string) (*IDToken, error) {
	parts := strings.Split(rawIDToken, ".")
	if len(parts) != 3 {
		return nil, errors.New("oidc: malformed JWT — expected 3 parts")
	}

	// Decode header to get key ID and algorithm
	header, err := decodeJWTSegment(parts[0])
	if err != nil {
		return nil, fmt.Errorf("oidc: decode header: %w", err)
	}

	alg, _ := header["alg"].(string)
	kid, _ := header["kid"].(string)

	// Verify algorithm is supported
	if !v.isAlgSupported(alg) {
		return nil, fmt.Errorf("oidc: unsupported signing algorithm: %s", alg)
	}

	// Get the signing key from JWKS
	key, err := v.getKey(ctx, kid)
	if err != nil {
		return nil, fmt.Errorf("oidc: get signing key: %w", err)
	}

	// Verify signature
	if err := verifySignature(rawIDToken, alg, key); err != nil {
		return nil, fmt.Errorf("oidc: verify signature: %w", err)
	}

	// Decode payload
	payload, err := decodeJWTSegment(parts[1])
	if err != nil {
		return nil, fmt.Errorf("oidc: decode payload: %w", err)
	}

	token := &IDToken{
		Issuer:   getString(payload, "iss"),
		Subject:  getString(payload, "sub"),
		Audience: getAudience(payload),
		Nonce:    getString(payload, "nonce"),
		Claims:   payload,
	}

	if exp, ok := getFloat64(payload, "exp"); ok {
		token.ExpiresAt = time.Unix(int64(exp), 0)
	}
	if iat, ok := getFloat64(payload, "iat"); ok {
		token.IssuedAt = time.Unix(int64(iat), 0)
	}

	// Validate claims
	if err := v.validateClaims(token); err != nil {
		return nil, err
	}

	return token, nil
}

// IDToken represents a parsed and verified OIDC ID token.
type IDToken struct {
	// Issuer is the "iss" claim.
	Issuer string

	// Subject is the "sub" claim (provider's unique user ID).
	Subject string

	// Audience is the "aud" claim.
	Audience []string

	// ExpiresAt is the "exp" claim.
	ExpiresAt time.Time

	// IssuedAt is the "iat" claim.
	IssuedAt time.Time

	// Nonce is the "nonce" claim (for replay protection).
	Nonce string

	// Claims holds all token claims for project-specific extraction.
	Claims map[string]interface{}
}

// ToUserInfo extracts standard OIDC UserInfo claims from the ID token.
func (t *IDToken) ToUserInfo() *UserInfo {
	return &UserInfo{
		Subject:       t.Subject,
		Email:         getString(t.Claims, "email"),
		EmailVerified: getBool(t.Claims, "email_verified"),
		Name:          getString(t.Claims, "name"),
		GivenName:     getString(t.Claims, "given_name"),
		FamilyName:    getString(t.Claims, "family_name"),
		Picture:       getString(t.Claims, "picture"),
		Locale:        getString(t.Claims, "locale"),
		Raw:           t.Claims,
	}
}

// --- Discovery ---

type discoveryDoc struct {
	Issuer                string   `json:"issuer"`
	AuthorizationEndpoint string   `json:"authorization_endpoint"`
	TokenEndpoint         string   `json:"token_endpoint"`
	UserInfoEndpoint      string   `json:"userinfo_endpoint"`
	JWKSUri               string   `json:"jwks_uri"`
	SupportedScopes       []string `json:"scopes_supported"`
	SupportedAlgs         []string `json:"id_token_signing_alg_values_supported"`
}

func (v *Verifier) discover(ctx context.Context) error {
	wellKnown := v.issuer + "/.well-known/openid-configuration"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, wellKnown, http.NoBody)
	if err != nil {
		return err
	}

	resp, err := v.config.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck // Error on close is safe to ignore for read operations

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("discovery returned %d: %s", resp.StatusCode, string(body))
	}

	var doc discoveryDoc
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return fmt.Errorf("decode discovery document: %w", err)
	}

	if doc.JWKSUri == "" {
		return errors.New("discovery document missing jwks_uri")
	}

	v.disco = &doc
	v.jwks = &jwksCache{
		jwksURI:  doc.JWKSUri,
		client:   v.config.HTTPClient,
		cacheTTL: v.config.JWKSCacheDuration,
	}

	return nil
}

// DiscoveryEndpoints returns the discovered OAuth2/OIDC endpoints.
// Useful for projects that need to build OAuth2 configs from discovery.
func (v *Verifier) DiscoveryEndpoints() DiscoveryEndpoints {
	if v.disco == nil {
		return DiscoveryEndpoints{}
	}
	return DiscoveryEndpoints{
		Authorization: v.disco.AuthorizationEndpoint,
		Token:         v.disco.TokenEndpoint,
		UserInfo:      v.disco.UserInfoEndpoint,
		JWKS:          v.disco.JWKSUri,
	}
}

// DiscoveryEndpoints holds the discovered OIDC endpoints.
type DiscoveryEndpoints struct {
	Authorization string
	Token         string
	UserInfo      string
	JWKS          string
}

// --- Validation ---

func (v *Verifier) validateClaims(token *IDToken) error {
	// Check issuer
	if !v.config.SkipIssuerCheck {
		if token.Issuer != v.issuer {
			return fmt.Errorf("oidc: issuer mismatch: got %q, expected %q", token.Issuer, v.issuer)
		}
	}

	// Check audience
	if !containsString(token.Audience, v.clientID) {
		return fmt.Errorf("oidc: audience mismatch: token audience %v does not contain %q", token.Audience, v.clientID)
	}

	// Check expiry
	if !v.config.SkipExpiryCheck {
		if time.Now().After(token.ExpiresAt) {
			return errors.New("oidc: token has expired")
		}
	}

	return nil
}

func (v *Verifier) isAlgSupported(alg string) bool {
	for _, a := range v.config.SupportedSigningAlgs {
		if a == alg {
			return true
		}
	}
	return false
}

// --- Helpers ---

func getString(m map[string]interface{}, key string) string {
	v, _ := m[key].(string)
	return v
}

func getBool(m map[string]interface{}, key string) bool {
	v, _ := m[key].(bool)
	return v
}

func getFloat64(m map[string]interface{}, key string) (float64, bool) {
	v, ok := m[key].(float64)
	return v, ok
}

func getAudience(m map[string]interface{}) []string {
	switch v := m["aud"].(type) {
	case string:
		return []string{v}
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, a := range v {
			if s, ok := a.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
