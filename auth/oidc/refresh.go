package oidc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// RefreshConfig holds the parameters for a token refresh request.
type RefreshConfig struct {
	// TokenEndpoint is the OAuth2 token endpoint URL
	// (e.g., "https://oauth2.googleapis.com/token").
	TokenEndpoint string

	// ClientID is the OAuth2 client identifier.
	ClientID string

	// ClientSecret is the OAuth2 client secret.
	ClientSecret string

	// RefreshToken is the refresh token to exchange.
	RefreshToken string

	// Scopes is an optional set of scopes to request.
	Scopes []string

	// ExtraParams holds optional platform-specific parameters.
	ExtraParams map[string]string
}

// RefreshToken exchanges a refresh token for new access + refresh tokens.
// It supports both JSON and form-encoded response formats from the provider.
func RefreshToken(ctx context.Context, cfg RefreshConfig) (*TokenResult, error) {
	if cfg.TokenEndpoint == "" {
		return nil, fmt.Errorf("oidc: token endpoint is required")
	}
	if cfg.RefreshToken == "" {
		return nil, fmt.Errorf("oidc: refresh token is required")
	}

	form := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {cfg.ClientID},
		"refresh_token": {cfg.RefreshToken},
	}
	if cfg.ClientSecret != "" {
		form.Set("client_secret", cfg.ClientSecret)
	}
	if len(cfg.Scopes) > 0 {
		form.Set("scope", strings.Join(cfg.Scopes, " "))
	}
	for k, v := range cfg.ExtraParams {
		form.Set(k, v)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("oidc: creating refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("oidc: sending refresh request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("oidc: reading refresh response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("oidc: refresh failed (status %d): %s", resp.StatusCode, body)
	}

	return parseTokenResponse(body)
}

// refreshResponse is the raw OAuth2 token response structure.
type refreshResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	Scope        string `json:"scope"`
}

// parseTokenResponse parses a token response body, trying JSON first then
// falling back to form-encoded for compatibility with older providers.
func parseTokenResponse(body []byte) (*TokenResult, error) {
	// Try JSON first — most OAuth2 providers return JSON.
	var rr refreshResponse
	if err := json.Unmarshal(body, &rr); err == nil && rr.AccessToken != "" {
		return refreshResponseToResult(&rr), nil
	}

	// Fall back to form-encoded (e.g., older Facebook API).
	values, err := url.ParseQuery(string(body))
	if err != nil {
		return nil, fmt.Errorf("oidc: unable to parse token response")
	}

	accessToken := values.Get("access_token")
	if accessToken == "" {
		return nil, fmt.Errorf("oidc: no access_token in response")
	}

	expiresIn, _ := strconv.ParseInt(values.Get("expires_in"), 10, 64)
	rr = refreshResponse{
		AccessToken:  accessToken,
		TokenType:    values.Get("token_type"),
		ExpiresIn:    expiresIn,
		RefreshToken: values.Get("refresh_token"),
		IDToken:      values.Get("id_token"),
		Scope:        values.Get("scope"),
	}

	return refreshResponseToResult(&rr), nil
}

// refreshResponseToResult converts the raw response into a TokenResult.
func refreshResponseToResult(rr *refreshResponse) *TokenResult {
	result := &TokenResult{
		AccessToken:  rr.AccessToken,
		RefreshToken: rr.RefreshToken,
		IDToken:      rr.IDToken,
		TokenType:    rr.TokenType,
	}
	if result.TokenType == "" {
		result.TokenType = "Bearer"
	}
	if rr.ExpiresIn > 0 {
		result.ExpiresAt = time.Now().Add(time.Duration(rr.ExpiresIn) * time.Second)
	}
	if rr.Scope != "" {
		result.Scopes = strings.Split(rr.Scope, " ")
	}
	return result
}
