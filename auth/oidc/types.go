package oidc

import "time"

// TokenResult holds the tokens returned from an OAuth2/OIDC exchange.
type TokenResult struct {
	// AccessToken is the OAuth2 access token.
	AccessToken string

	// RefreshToken is the OAuth2 refresh token (may be empty).
	RefreshToken string

	// IDToken is the raw OIDC ID token JWT string (empty for non-OIDC providers).
	IDToken string

	// TokenType is typically "Bearer".
	TokenType string

	// ExpiresAt is when the access token expires.
	ExpiresAt time.Time

	// Scopes are the granted scopes (may differ from requested).
	Scopes []string
}

// UserInfo represents the standard OIDC UserInfo claims.
// Only standard fields are included — projects extract provider-specific
// claims by parsing the ID token or calling provider-specific APIs.
type UserInfo struct {
	// Subject is the provider's unique identifier for the user.
	Subject string `json:"sub"`

	// Email is the user's email address (may be empty if not in scope).
	Email string `json:"email,omitempty"`

	// EmailVerified indicates if the provider has verified the email.
	EmailVerified bool `json:"email_verified,omitempty"`

	// Name is the user's full display name.
	Name string `json:"name,omitempty"`

	// GivenName is the user's first name.
	GivenName string `json:"given_name,omitempty"`

	// FamilyName is the user's last name.
	FamilyName string `json:"family_name,omitempty"`

	// Picture is a URL to the user's profile picture.
	Picture string `json:"picture,omitempty"`

	// Locale is the user's locale (e.g., "en-US").
	Locale string `json:"locale,omitempty"`

	// Raw holds all claims from the provider for project-specific extraction.
	Raw map[string]interface{} `json:"raw,omitempty"`
}
