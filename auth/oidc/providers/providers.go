// Package providers implements concrete OAuth2/OIDC providers for gokit.
//
// Each provider implements the oidc.Provider interface and handles
// provider-specific quirks (token exchange, user info fetching).
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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/kbukum/gokit/auth/oidc"
)

// ProviderConfig holds common OAuth2 configuration for all providers.
type ProviderConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Scopes       []string
}

// httpClient is a shared HTTP client with sensible defaults.
var httpClient = &http.Client{Timeout: 10 * time.Second}

// tokenResponse is the standard OAuth2 token response.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
}

// exchangeCode performs a standard OAuth2 authorization code exchange.
func exchangeCode(ctx context.Context, tokenURL string, cfg ProviderConfig, code string, opts oidc.ExchangeOptions, extraHeaders map[string]string) (*tokenResponse, error) {
	redirectURI := opts.RedirectURI
	if redirectURI == "" {
		redirectURI = cfg.RedirectURL
	}

	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"client_id":     {cfg.ClientID},
		"client_secret": {cfg.ClientSecret},
		"redirect_uri":  {redirectURI},
	}
	if opts.CodeVerifier != "" {
		data.Set("code_verifier", opts.CodeVerifier)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token exchange: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var tok tokenResponse
	if err := json.Unmarshal(body, &tok); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}
	return &tok, nil
}

// fetchJSON performs a GET request with a Bearer token and decodes JSON.
func fetchJSON(ctx context.Context, endpoint, accessToken string, result interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return json.Unmarshal(body, result)
}

// toTokenResult converts a tokenResponse to an oidc.TokenResult.
func toTokenResult(tok *tokenResponse) *oidc.TokenResult {
	result := &oidc.TokenResult{
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		IDToken:      tok.IDToken,
		TokenType:    tok.TokenType,
	}
	if tok.ExpiresIn > 0 {
		result.ExpiresAt = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
	}
	if tok.Scope != "" {
		result.Scopes = strings.Split(tok.Scope, " ")
	}
	return result
}

// buildAuthURL constructs a standard OAuth2 authorization URL.
func buildAuthURL(authEndpoint string, cfg ProviderConfig, state string, opts oidc.AuthURLOptions, extraParams map[string]string) string {
	scopes := cfg.Scopes
	if len(opts.Scopes) > 0 {
		scopes = opts.Scopes
	}
	redirectURI := cfg.RedirectURL
	if opts.RedirectURI != "" {
		redirectURI = opts.RedirectURI
	}

	params := url.Values{
		"client_id":     {cfg.ClientID},
		"redirect_uri":  {redirectURI},
		"response_type": {"code"},
		"state":         {state},
	}
	if len(scopes) > 0 {
		params.Set("scope", strings.Join(scopes, " "))
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
