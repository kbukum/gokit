package providers

import (
	"net/url"
	"strings"
)

// BuildAuthURL constructs a standard OAuth2 authorization URL from req.
func BuildAuthURL(req AuthURLRequest) string {
	clientIDParam := req.ClientIDParam
	if clientIDParam == "" {
		clientIDParam = "client_id"
	}
	scopeSeparator := req.ScopeSeparator
	if scopeSeparator == "" {
		scopeSeparator = " "
	}

	opts := req.Options
	scopes := req.Config.Scopes
	if len(opts.Scopes) > 0 {
		scopes = opts.Scopes
	}
	redirectURI := req.Config.RedirectURL
	if opts.RedirectURI != "" {
		redirectURI = opts.RedirectURI
	}

	params := url.Values{
		clientIDParam:   {req.Config.ClientID},
		"redirect_uri":  {redirectURI},
		"response_type": {"code"},
		"state":         {req.State},
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
	for k, v := range req.ExtraParams {
		params.Set(k, v)
	}
	for k, v := range opts.ExtraParams {
		params.Set(k, v)
	}

	return req.Endpoint + "?" + params.Encode()
}
