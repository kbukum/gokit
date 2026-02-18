package oidc

import "context"

// Provider defines the contract for an OAuth2/OIDC authentication provider.
// Projects implement this per-provider (Google, GitHub, Microsoft, etc.)
// or use the generic OIDC implementation for standard-compliant providers.
//
// This interface covers the Authorization Code flow — the most common
// server-side OAuth2 pattern.
type Provider interface {
	// Name returns the provider identifier (e.g., "google", "github").
	Name() string

	// AuthURL returns the authorization URL for initiating the OAuth2 flow.
	// The state parameter should be a cryptographically random value for CSRF protection.
	// Options allow passing additional parameters (PKCE, nonce, login_hint, etc.).
	AuthURL(state string, opts ...AuthURLOption) string

	// Exchange trades an authorization code for tokens.
	// Returns the token set (access, refresh, ID token if OIDC) and user information.
	Exchange(ctx context.Context, code string, opts ...ExchangeOption) (*TokenResult, error)

	// UserInfo fetches the user's profile from the provider using an access token.
	// Not all providers support this endpoint (GitHub uses a different API).
	UserInfo(ctx context.Context, accessToken string) (*UserInfo, error)
}

// AuthURLOption configures authorization URL generation.
type AuthURLOption func(*AuthURLOptions)

// AuthURLOptions holds the configuration for authorization URL generation.
type AuthURLOptions struct {
	RedirectURI string
	Scopes      []string
	Nonce       string
	PKCE        *PKCE
	ExtraParams map[string]string
}

// WithRedirectURI overrides the configured redirect URI for this request.
func WithRedirectURI(uri string) AuthURLOption {
	return func(o *AuthURLOptions) { o.RedirectURI = uri }
}

// WithScopes overrides the default scopes for this request.
func WithScopes(scopes ...string) AuthURLOption {
	return func(o *AuthURLOptions) { o.Scopes = scopes }
}

// WithNonce adds an OIDC nonce parameter for replay protection.
func WithNonce(nonce string) AuthURLOption {
	return func(o *AuthURLOptions) { o.Nonce = nonce }
}

// WithPKCE adds PKCE (Proof Key for Code Exchange) parameters.
func WithPKCE(pkce *PKCE) AuthURLOption {
	return func(o *AuthURLOptions) { o.PKCE = pkce }
}

// WithExtraParam adds a custom query parameter to the authorization URL.
func WithExtraParam(key, value string) AuthURLOption {
	return func(o *AuthURLOptions) {
		if o.ExtraParams == nil {
			o.ExtraParams = make(map[string]string)
		}
		o.ExtraParams[key] = value
	}
}

// ApplyAuthURLOptions applies options and returns the resolved configuration.
// This is a helper for Provider implementations to process AuthURLOption parameters.
func ApplyAuthURLOptions(opts []AuthURLOption) AuthURLOptions {
	var o AuthURLOptions
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// ExchangeOption configures the token exchange.
type ExchangeOption func(*ExchangeOptions)

// ExchangeOptions holds the configuration for token exchange.
type ExchangeOptions struct {
	RedirectURI  string
	CodeVerifier string
}

// WithExchangeRedirectURI sets the redirect URI for the exchange (must match the one used in AuthURL).
func WithExchangeRedirectURI(uri string) ExchangeOption {
	return func(o *ExchangeOptions) { o.RedirectURI = uri }
}

// WithCodeVerifier adds the PKCE code verifier for the exchange.
func WithCodeVerifier(verifier string) ExchangeOption {
	return func(o *ExchangeOptions) { o.CodeVerifier = verifier }
}

// ApplyExchangeOptions applies options and returns the resolved configuration.
// This is a helper for Provider implementations to process ExchangeOption parameters.
func ApplyExchangeOptions(opts []ExchangeOption) ExchangeOptions {
	var o ExchangeOptions
	for _, opt := range opts {
		opt(&o)
	}
	return o
}
