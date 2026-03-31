package oidc

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// ParseIDTokenClaims extracts standard user info claims from a JWT ID token
// without full signature verification. This is safe when the token was just
// received directly from the provider's token endpoint over HTTPS.
//
// For tokens received from untrusted sources, use the Verifier to validate
// the signature first.
//
// This is a general OIDC utility — any provider that returns an ID token
// can use this to extract user information (Apple, Google, Azure AD, etc.).
func ParseIDTokenClaims(idToken string) (*UserInfo, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("oidc: invalid ID token format (expected 3 parts, got %d)", len(parts))
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("oidc: decode ID token payload: %w", err)
	}

	var claims struct {
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified any    `json:"email_verified"`
		Name          string `json:"name"`
		GivenName     string `json:"given_name"`
		FamilyName    string `json:"family_name"`
		Picture       string `json:"picture"`
		Locale        string `json:"locale"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("oidc: unmarshal ID token claims: %w", err)
	}

	emailVerified := false
	switch v := claims.EmailVerified.(type) {
	case bool:
		emailVerified = v
	case string:
		emailVerified = v == "true"
	}

	return &UserInfo{
		Subject:       claims.Sub,
		Email:         claims.Email,
		EmailVerified: emailVerified,
		Name:          claims.Name,
		GivenName:     claims.GivenName,
		FamilyName:    claims.FamilyName,
		Picture:       claims.Picture,
		Locale:        claims.Locale,
	}, nil
}
