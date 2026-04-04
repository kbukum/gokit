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

// httpClient is a shared HTTP client with sensible defaults.
var httpClient = &http.Client{Timeout: 10 * time.Second}

// TokenResponse is the standard OAuth2 token response.
// Exported for use by custom providers that implement oidc.Provider directly.
type TokenResponse = tokenResponse

// tokenResponse is the standard OAuth2 token response.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
}

// ExchangeCode performs a standard OAuth2 authorization code exchange.
// This is a low-level helper used by GenericProvider. Custom providers
// that can't use GenericConfig can call this directly.
func ExchangeCode(ctx context.Context, tokenURL string, cfg ProviderConfig, code string, opts oidc.ExchangeOptions, extraHeaders map[string]string) (*tokenResponse, error) {
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
	defer func() { _ = resp.Body.Close() }()

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

// ExchangeJSON performs a token exchange using a JSON request body.
// Used by providers like TikTok that require JSON instead of form-encoded.
func ExchangeJSON(ctx context.Context, tokenURL string, cfg ProviderConfig, code string, opts oidc.ExchangeOptions, clientIDParam string, extraHeaders map[string]string) (*tokenResponse, error) {
	redirectURI := opts.RedirectURI
	if redirectURI == "" {
		redirectURI = cfg.RedirectURL
	}

	clientIDKey := "client_id"
	if clientIDParam != "" {
		clientIDKey = clientIDParam
	}

	payload := map[string]string{
		clientIDKey:     cfg.ClientID,
		"client_secret": cfg.ClientSecret,
		"code":          code,
		"grant_type":    "authorization_code",
		"redirect_uri":  redirectURI,
	}
	if opts.CodeVerifier != "" {
		payload["code_verifier"] = opts.CodeVerifier
	}

	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal token request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token exchange: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var tok tokenResponse
	if err := json.Unmarshal(body, &tok); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}
	return &tok, nil
}

// FetchJSON performs a GET request with a Bearer token and decodes JSON.
func FetchJSON(ctx context.Context, endpoint, accessToken string, result interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return json.Unmarshal(body, result)
}

// ToTokenResult converts a tokenResponse to an oidc.TokenResult.
func ToTokenResult(tok *tokenResponse) *oidc.TokenResult {
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

// StrVal extracts a string value from a JSON-decoded map.
// Returns "" if the key is missing or not a string.
func StrVal(m map[string]interface{}, key string) string {
	v, _ := m[key].(string)
	return v
}

// BoolVal extracts a bool value from a JSON-decoded map.
// Returns false if the key is missing or not a bool.
func BoolVal(m map[string]interface{}, key string) bool {
	v, _ := m[key].(bool)
	return v
}

// NestedMap traverses a dot-separated path in a JSON-decoded map.
// For example, NestedMap(m, "data.user") returns m["data"]["user"].
// Returns nil if any segment is missing or not a map.
func NestedMap(m map[string]interface{}, path string) map[string]interface{} {
	if path == "" {
		return m
	}
	parts := strings.Split(path, ".")
	current := m
	for _, part := range parts {
		next, ok := current[part].(map[string]interface{})
		if !ok {
			return nil
		}
		current = next
	}
	return current
}
