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
type AuthURLOption func(*authURLOptions)

type authURLOptions struct {
	redirectURI string
	scopes      []string
	nonce       string
	pkce        *PKCE
	extraParams map[string]string
}

// WithRedirectURI overrides the configured redirect URI for this request.
func WithRedirectURI(uri string) AuthURLOption {
	return func(o *authURLOptions) { o.redirectURI = uri }
}

// WithScopes overrides the default scopes for this request.
func WithScopes(scopes ...string) AuthURLOption {
	return func(o *authURLOptions) { o.scopes = scopes }
}

// WithNonce adds an OIDC nonce parameter for replay protection.
func WithNonce(nonce string) AuthURLOption {
	return func(o *authURLOptions) { o.nonce = nonce }
}

// WithPKCE adds PKCE (Proof Key for Code Exchange) parameters.
func WithPKCE(pkce *PKCE) AuthURLOption {
	return func(o *authURLOptions) { o.pkce = pkce }
}

// WithExtraParam adds a custom query parameter to the authorization URL.
func WithExtraParam(key, value string) AuthURLOption {
	return func(o *authURLOptions) {
		if o.extraParams == nil {
			o.extraParams = make(map[string]string)
		}
		o.extraParams[key] = value
	}
}

// ApplyAuthURLOptions applies options and returns the resolved configuration.
func ApplyAuthURLOptions(opts []AuthURLOption) authURLOptions {
	var o authURLOptions
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// ExchangeOption configures the token exchange.
type ExchangeOption func(*exchangeOptions)

type exchangeOptions struct {
	redirectURI  string
	codeVerifier string
}

// WithExchangeRedirectURI sets the redirect URI for the exchange (must match the one used in AuthURL).
func WithExchangeRedirectURI(uri string) ExchangeOption {
	return func(o *exchangeOptions) { o.redirectURI = uri }
}

// WithCodeVerifier adds the PKCE code verifier for the exchange.
func WithCodeVerifier(verifier string) ExchangeOption {
	return func(o *exchangeOptions) { o.codeVerifier = verifier }
}

// ApplyExchangeOptions applies options and returns the resolved configuration.
func ApplyExchangeOptions(opts []ExchangeOption) exchangeOptions {
	var o exchangeOptions
	for _, opt := range opts {
		opt(&o)
	}
	return o
}
