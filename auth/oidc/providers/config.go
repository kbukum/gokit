// Package providers implements OAuth2/OIDC providers for gokit.
//
// All providers use GenericProvider —
// the single implementation that handles any OAuth2/OIDC provider through configuration.
// Built-in providers (Google, GitHub, Apple)
// and custom providers (YouTube, TikTok, Instagram) are all constructor functions that return NewGeneric(GenericConfig{...}).
//
// Adding a new provider requires only defining its endpoints, field mappings,
// and optional hooks for provider-specific quirks. No HTTP code needed.
//
// Usage:
//
//	google := providers.NewGoogle(providers.ProviderConfig{
//	    ClientID:     "your-client-id",
//	    ClientSecret: "your-client-secret",
//	    RedirectURL:  "http://localhost:8381/api/v1/auth/oauth/google/callback",
//	})
//	authURL := google.AuthURL(state)
//	tokens, err := google.Exchange(ctx, code)
//	user, err := google.UserInfo(ctx, tokens.AccessToken)
package providers

import (
	"context"
	"net/http"
	"time"

	"github.com/kbukum/gokit/auth/oidc"
)

// DefaultHTTPClient is the default HTTP client used when GenericConfig.HTTPClient is nil.
var DefaultHTTPClient = &http.Client{Timeout: 10 * time.Second}

// ProviderConfig holds common OAuth2 credentials for all providers.
type ProviderConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Scopes       []string
}

// UserInfoMapper maps provider-specific JSON fields to standard UserInfo fields.
// Each field specifies the JSON key to extract. Empty means the field is not available.
type UserInfoMapper struct {
	SubjectKey       string // JSON key for unique user ID (e.g., "sub", "id", "open_id")
	EmailKey         string // JSON key for email (e.g., "email")
	EmailVerifiedKey string // JSON key for email_verified (e.g., "email_verified")
	NameKey          string // JSON key for display name (e.g., "name", "display_name")
	GivenNameKey     string // JSON key for first name
	FamilyNameKey    string // JSON key for last name
	PictureKey       string // JSON key for avatar URL (e.g., "picture", "avatar_url")
	LocaleKey        string // JSON key for locale
	ResponsePath     string // Dot-separated path to user data in nested responses (e.g., "data.user")
}

// GenericConfig configures any OAuth2/OIDC provider through configuration alone.
// This is the single configuration type that powers ALL providers —
// built-in (Google, GitHub, Apple) and custom (YouTube, TikTok, Instagram, etc.).
//
// The design follows OAuth 2.0 (RFC 6749) with configurable knobs for the common provider-specific deviations:
//   - ClientIDParam: TikTok uses "client_key" instead of "client_id"
//   - ScopeSeparator: TikTok uses "," instead of " "
//   - TokenRequestFormat: TikTok requires JSON body instead of form-encoded
//   - ClientSecretFunc: Apple generates a JWT-signed secret per exchange
//   - IDTokenAsUserInfo: Apple has no userinfo endpoint
//   - PostUserInfoHook: GitHub needs a secondary /user/emails API call
//   - PostExchangeHook: Instagram exchanges short-lived for long-lived tokens
type GenericConfig struct {
	ProviderConfig

	// --- Identity ---

	// ProviderName is the identifier returned by Name() (e.g., "google", "tiktok").
	ProviderName string

	// Label is the human-readable display name (e.g., "Google", "Sign in with Apple").
	// Used by Manager.ListProviders() for UI rendering. Defaults to ProviderName if empty.
	Label string

	// Type categorizes the provider for UI grouping. Convention: "identity" for login-only,
	// "social" for platform-connected. Defaults to "identity" if empty.
	Type string

	// --- Endpoints ---

	// AuthEndpoint is the OAuth2 authorization URL.
	AuthEndpoint string

	// TokenEndpoint is the OAuth2 token exchange URL.
	TokenEndpoint string

	// UserInfoEndpoint is the URL to fetch user profile data.
	// Empty if IDTokenAsUserInfo is true (e.g., Apple).
	UserInfoEndpoint string

	// --- Auth URL Customization ---

	// AuthExtraParams are additional static params added to every auth URL. Example: {"access_type":
	// "offline", "prompt": "consent"} for Google.
	AuthExtraParams map[string]string

	// ClientIDParam overrides the query param name for client_id (default: "client_id").
	// TikTok uses "client_key".
	ClientIDParam string

	// ScopeSeparator is the character used to join scopes (default: " ").
	// Some providers like TikTok use ",".
	ScopeSeparator string

	// --- Token Exchange Customization ---

	// TokenRequestFormat controls how the token exchange body is sent. "form" (default):
	// application/x-www-form-urlencoded (standard OAuth2) "json":
	// application/json (required by some providers like TikTok)
	TokenRequestFormat string

	// TokenExtraHeaders are additional headers for the token exchange request. Example: {"Accept":
	// "application/json"} for GitHub.
	TokenExtraHeaders map[string]string

	// ClientSecretFunc generates a dynamic client secret per exchange. Apple requires this —
	// it generates a JWT signed with ES256 per request. If nil,
	// uses ProviderConfig.ClientSecret as-is (the standard approach).
	ClientSecretFunc func() (string, error)

	// --- UserInfo Customization ---

	// UserInfo maps provider-specific JSON fields to standard UserInfo.
	UserInfo UserInfoMapper

	// IDTokenAsUserInfo when true,
	// extracts user info from the ID token instead of calling UserInfoEndpoint.
	// Use for providers that don't have a separate userinfo endpoint (e.g., Apple, some enterprise OIDC providers).
	IDTokenAsUserInfo bool

	// PostUserInfoHook is called after user info is fetched,
	// allowing secondary API calls to enrich the data.
	// Return nil error to keep the (possibly modified) UserInfo. Example:
	// GitHub uses this to fetch /user/emails when email isn't public.
	PostUserInfoHook func(ctx context.Context, accessToken string, info *oidc.UserInfo) error

	// --- Post-Exchange Customization ---

	// PostExchangeHook is called after a successful token exchange.
	// Use for provider-specific post-processing (e.g., Instagram long-lived token exchange).
	// The client parameter is the resolved HTTP client from GenericConfig.HTTPClient.
	// Return the (possibly modified) TokenResult, or an error. If nil,
	// the token exchange result is used as-is.
	PostExchangeHook func(ctx context.Context, client *http.Client, cfg ProviderConfig, token *oidc.TokenResult) (*oidc.TokenResult, error)

	// --- Token Refresh Customization ---

	// RefreshEndpoint is the OAuth2 token refresh URL.
	// Defaults to TokenEndpoint for standard OAuth2 providers.
	// Override for providers that use a different endpoint for refresh (e.g., Instagram uses graph.instagram.com/refresh_access_token).
	RefreshEndpoint string

	// RefreshFunc is a custom refresh implementation for providers that don't follow standard OAuth2 refresh_token grant (RFC 6749 §6).
	// Examples: Facebook fb_exchange_token, Instagram ig_refresh_token,
	// TikTok JSON body with client_key. If nil,
	// uses standard form-encoded refresh_token grant via oidc.RefreshToken().
	RefreshFunc func(ctx context.Context, client *http.Client, cfg ProviderConfig, token oidc.RefreshInput) (*oidc.TokenResult, error)

	// --- HTTP Client ---

	// HTTPClient is the HTTP client used for token exchange, user info, and any hook requests.
	// Default: &http.Client{Timeout: 10 * time.Second}.
	HTTPClient *http.Client
}

// WithDefaultScopes returns a copy of cfg with default scopes applied if cfg.Scopes is empty.
// Provider constructors (GitHub, Google, Apple, etc.) use this to layer their well-known default scope set without overriding caller-supplied values.
func WithDefaultScopes(cfg ProviderConfig, defaults ...string) ProviderConfig {
	if len(cfg.Scopes) == 0 {
		cfg.Scopes = defaults
	}
	return cfg
}
