package providers

import (
	"net/url"
	"strings"

	"github.com/kbukum/gokit/auth/oidc"
)

// BuildAuthURL constructs a standard OAuth2 authorization URL.
// clientIDParam overrides the query param name for client_id (pass "" for default "client_id").
// scopeSeparator overrides the scope join character (pass "" for default " ").
func BuildAuthURL(authEndpoint string, cfg ProviderConfig, state string, opts oidc.AuthURLOptions, extraParams map[string]string, clientIDParam, scopeSeparator string) string {
	if clientIDParam == "" {
		clientIDParam = "client_id"
	}
	if scopeSeparator == "" {
		scopeSeparator = " "
	}

	scopes := cfg.Scopes
	if len(opts.Scopes) > 0 {
		scopes = opts.Scopes
	}
	redirectURI := cfg.RedirectURL
	if opts.RedirectURI != "" {
		redirectURI = opts.RedirectURI
	}

	params := url.Values{
		clientIDParam:   {cfg.ClientID},
		"redirect_uri":  {redirectURI},
		"response_type": {"code"},
		"state":         {state},
	}
	if len(scopes) > 0 {
		params.Set("scope", strings.Join(scopes, scopeSeparator))
	}
	if opts.PKCE != nil {
		params.Set("code_challenge", opts.PKCE.CodeChallenge)
		params.Set("code_challenge_method", opts.PKCE.CodeChallengeMethod)
	}
	if opts.Nonce != "" {
		params.Set("nonce", opts.Nonce)
	}
	for k, v := range extraParams {
		params.Set(k, v)
	}
	for k, v := range opts.ExtraParams {
		params.Set(k, v)
	}

	return authEndpoint + "?" + params.Encode()
}
