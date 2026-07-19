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

// resolveClient returns the given client if non-nil, otherwise DefaultHTTPClient.
func resolveClient(client *http.Client) *http.Client {
	if client != nil {
		return client
	}
	return DefaultHTTPClient
}

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
// This is a low-level helper used by GenericProvider.
// Custom providers that can't use GenericConfig can call this directly.
func ExchangeCode(ctx context.Context, req ExchangeRequest) (*tokenResponse, error) {
	redirectURI := req.Options.RedirectURI
	if redirectURI == "" {
		redirectURI = req.Config.RedirectURL
	}

	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {req.Code},
		"client_id":     {req.Config.ClientID},
		"client_secret": {req.Config.ClientSecret},
		"redirect_uri":  {redirectURI},
	}
	if req.Options.CodeVerifier != "" {
		data.Set("code_verifier", req.Options.CodeVerifier)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, req.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create token request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for k, v := range req.ExtraHeaders {
		httpReq.Header.Set(k, v)
	}

	resp, err := resolveClient(req.Client).Do(httpReq)
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
func ExchangeJSON(ctx context.Context, req ExchangeRequest) (*tokenResponse, error) {
	redirectURI := req.Options.RedirectURI
	if redirectURI == "" {
		redirectURI = req.Config.RedirectURL
	}

	clientIDKey := "client_id"
	if req.ClientIDParam != "" {
		clientIDKey = req.ClientIDParam
	}

	payload := map[string]string{
		clientIDKey:     req.Config.ClientID,
		"client_secret": req.Config.ClientSecret,
		"code":          req.Code,
		"grant_type":    "authorization_code",
		"redirect_uri":  redirectURI,
	}
	if req.Options.CodeVerifier != "" {
		payload["code_verifier"] = req.Options.CodeVerifier
	}

	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal token request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, req.TokenURL, strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, fmt.Errorf("build token request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	for k, v := range req.ExtraHeaders {
		httpReq.Header.Set(k, v)
	}

	resp, err := resolveClient(req.Client).Do(httpReq)
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
