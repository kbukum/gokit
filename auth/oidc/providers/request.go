package providers

import (
	"net/http"

	"github.com/kbukum/gokit/auth/oidc"
)

// AuthURLRequest bundles the inputs for [BuildAuthURL]. ClientIDParam
// and ScopeSeparator are optional overrides with documented defaults.
type AuthURLRequest struct {
	// Endpoint is the provider authorization endpoint.
	Endpoint string
	// Config carries the client credentials and default scopes/redirect.
	Config ProviderConfig
	// State is the opaque CSRF state value.
	State string
	// Options are the per-request auth URL options (scopes, PKCE, nonce, extra params).
	Options oidc.AuthURLOptions
	// ExtraParams are static params added to every auth URL.
	ExtraParams map[string]string
	// ClientIDParam overrides the client_id query param name ("" → "client_id").
	ClientIDParam string
	// ScopeSeparator overrides the scope join character ("" → " ").
	ScopeSeparator string
}

// ExchangeRequest bundles the inputs for [ExchangeCode] and [ExchangeJSON].
// Client may be nil to use [DefaultHTTPClient]. ClientIDParam is honored only by [ExchangeJSON].
type ExchangeRequest struct {
	// Client is the HTTP client to use; nil selects DefaultHTTPClient.
	Client *http.Client
	// TokenURL is the provider token endpoint.
	TokenURL string
	// Config carries the client credentials and default redirect.
	Config ProviderConfig
	// Code is the authorization code being exchanged.
	Code string
	// Options are the per-request exchange options (redirect override, PKCE verifier).
	Options oidc.ExchangeOptions
	// ExtraHeaders are additional headers for the token request.
	ExtraHeaders map[string]string
	// ClientIDParam overrides the client_id field name ("" → "client_id"); JSON exchange only.
	ClientIDParam string
}

// FetchRequest bundles the inputs for [FetchJSON]. Client may be nil to use [DefaultHTTPClient].
type FetchRequest struct {
	// Client is the HTTP client to use; nil selects DefaultHTTPClient.
	Client *http.Client
	// Endpoint is the resource endpoint to GET.
	Endpoint string
	// AccessToken is the bearer token sent in the Authorization header.
	AccessToken string
}
