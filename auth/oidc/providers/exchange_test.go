package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kbukum/gokit/auth/oidc"
	"github.com/kbukum/gokit/auth/oidc/testutil"
)

func TestExchangeFormEncoded(t *testing.T) {
	srv := testutil.NewMockOAuthServer()
	defer srv.Close()

	p := NewGeneric(GenericConfig{
		ProviderConfig: mockProviderConfig(),
		ProviderName:   "form-test",
		TokenEndpoint:  srv.TokenURL(),
	})

	tokens, err := p.Exchange(context.Background(), "my-code")
	if err != nil {
		t.Fatal(err)
	}
	if tokens.AccessToken != "mock-access-token" {
		t.Errorf("AccessToken = %q, want 'mock-access-token'", tokens.AccessToken)
	}
	if tokens.RefreshToken != "mock-refresh-token" {
		t.Errorf("RefreshToken = %q, want 'mock-refresh-token'", tokens.RefreshToken)
	}

	req := srv.LastTokenRequest()
	if req == nil {
		t.Fatal("no token request recorded")
	}
	if req["code"] != "my-code" {
		t.Errorf("token request code = %q, want 'my-code'", req["code"])
	}
	if req["grant_type"] != "authorization_code" {
		t.Errorf("grant_type = %q, want 'authorization_code'", req["grant_type"])
	}
	if req["client_id"] != "test-client-id" {
		t.Errorf("client_id = %q, want 'test-client-id'", req["client_id"])
	}
}

func TestExchangeJSONFormat(t *testing.T) {
	srv := testutil.NewMockOAuthServer()
	defer srv.Close()

	p := NewGeneric(GenericConfig{
		ProviderConfig:     mockProviderConfig(),
		ProviderName:       "json-test",
		TokenEndpoint:      srv.TokenURL(),
		TokenRequestFormat: "json",
		ClientIDParam:      "client_key",
	})

	tokens, err := p.Exchange(context.Background(), "json-code")
	if err != nil {
		t.Fatal(err)
	}
	if tokens.AccessToken != "mock-access-token" {
		t.Errorf("AccessToken = %q, want 'mock-access-token'", tokens.AccessToken)
	}

	// Verify request used correct client ID param name
	req := srv.LastTokenRequest()
	if req == nil {
		t.Fatal("no token request recorded")
	}
	if req["client_key"] != "test-client-id" {
		t.Errorf("expected client_key='test-client-id', got %q", req["client_key"])
	}
	if _, hasOld := req["client_id"]; hasOld {
		t.Error("JSON exchange should use client_key, not client_id")
	}
}

func TestExchangeTokenError(t *testing.T) {
	srv := testutil.NewMockOAuthServer()
	defer srv.Close()
	srv.FailToken(true)

	p := NewGeneric(GenericConfig{
		ProviderConfig: mockProviderConfig(),
		ProviderName:   "fail-test",
		TokenEndpoint:  srv.TokenURL(),
	})

	_, err := p.Exchange(context.Background(), "bad-code")
	if err == nil {
		t.Fatal("expected error for failed token exchange")
	}
	if !strings.Contains(err.Error(), "fail-test") {
		t.Errorf("error should include provider name, got: %s", err)
	}
}

func TestExchangeWithExtraHeaders(t *testing.T) {
	var receivedAccept string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAccept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "tok",
			"token_type":   "Bearer",
		})
	}))
	defer server.Close()

	p := NewGeneric(GenericConfig{
		ProviderConfig: ProviderConfig{ClientID: "id", ClientSecret: "s", RedirectURL: "http://test"},
		ProviderName:   "header-test",
		TokenEndpoint:  server.URL + "/token",
		TokenExtraHeaders: map[string]string{
			"Accept": "application/json",
		},
	})

	_, err := p.Exchange(context.Background(), "code")
	if err != nil {
		t.Fatal(err)
	}
	if receivedAccept != "application/json" {
		t.Errorf("Accept header = %q, want 'application/json'", receivedAccept)
	}
}

func TestToTokenResult(t *testing.T) {
	tok := &tokenResponse{
		AccessToken:  "at",
		RefreshToken: "rt",
		IDToken:      "idt",
		TokenType:    "Bearer",
		ExpiresIn:    3600,
		Scope:        "openid email profile",
	}
	result := ToTokenResult(tok)

	if result.AccessToken != "at" {
		t.Errorf("AccessToken = %q", result.AccessToken)
	}
	if result.RefreshToken != "rt" {
		t.Errorf("RefreshToken = %q", result.RefreshToken)
	}
	if result.IDToken != "idt" {
		t.Errorf("IDToken = %q", result.IDToken)
	}
	if len(result.Scopes) != 3 {
		t.Errorf("Scopes = %v, want 3 items", result.Scopes)
	}
	if result.ExpiresAt.IsZero() {
		t.Error("ExpiresAt should be set")
	}
}

func TestToTokenResultZeroExpiry(t *testing.T) {
	tok := &tokenResponse{AccessToken: "at"}
	result := ToTokenResult(tok)
	if !result.ExpiresAt.IsZero() {
		t.Error("ExpiresAt should be zero when ExpiresIn is 0")
	}
}

func TestExchangeCodeDirect(t *testing.T) {
	srv := testutil.NewMockOAuthServer()
	defer srv.Close()

	cfg := mockProviderConfig()
	tok, err := ExchangeCode(context.Background(), ExchangeRequest{TokenURL: srv.TokenURL(), Config: cfg, Code: "code"})
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "mock-access-token" {
		t.Errorf("AccessToken = %q", tok.AccessToken)
	}
}

func TestExchangeCodeWithCodeVerifier(t *testing.T) {
	srv := testutil.NewMockOAuthServer()
	defer srv.Close()

	cfg := mockProviderConfig()
	opts := oidc.ExchangeOptions{CodeVerifier: "my-verifier"}
	_, err := ExchangeCode(context.Background(), ExchangeRequest{TokenURL: srv.TokenURL(), Config: cfg, Code: "code", Options: opts})
	if err != nil {
		t.Fatal(err)
	}

	req := srv.LastTokenRequest()
	if req["code_verifier"] != "my-verifier" {
		t.Errorf("code_verifier = %q, want 'my-verifier'", req["code_verifier"])
	}
}

func TestExchangeCodeRedirectOverride(t *testing.T) {
	srv := testutil.NewMockOAuthServer()
	defer srv.Close()

	cfg := mockProviderConfig()
	opts := oidc.ExchangeOptions{RedirectURI: "http://overridden/callback"}
	_, err := ExchangeCode(context.Background(), ExchangeRequest{TokenURL: srv.TokenURL(), Config: cfg, Code: "code", Options: opts})
	if err != nil {
		t.Fatal(err)
	}

	req := srv.LastTokenRequest()
	if req["redirect_uri"] != "http://overridden/callback" {
		t.Errorf("redirect_uri = %q, want 'http://overridden/callback'", req["redirect_uri"])
	}
}

func TestExchangeJSONDirect(t *testing.T) {
	srv := testutil.NewMockOAuthServer()
	defer srv.Close()

	cfg := mockProviderConfig()
	tok, err := ExchangeJSON(context.Background(), ExchangeRequest{TokenURL: srv.TokenURL(), Config: cfg, Code: "json-code"})
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "mock-access-token" {
		t.Errorf("AccessToken = %q", tok.AccessToken)
	}
}

func TestExchangeJSONCustomClientIDParam(t *testing.T) {
	srv := testutil.NewMockOAuthServer()
	defer srv.Close()

	cfg := mockProviderConfig()
	_, err := ExchangeJSON(context.Background(), ExchangeRequest{TokenURL: srv.TokenURL(), Config: cfg, Code: "code", ClientIDParam: "client_key"})
	if err != nil {
		t.Fatal(err)
	}

	req := srv.LastTokenRequest()
	if req["client_key"] != "test-client-id" {
		t.Errorf("client_key = %q, want 'test-client-id'", req["client_key"])
	}
}

func TestResolveClient_NonNil(t *testing.T) {
	c := &http.Client{}
	if resolveClient(c) != c {
		t.Fatal("expected provided client to be returned")
	}
	if resolveClient(nil) != DefaultHTTPClient {
		t.Fatal("expected DefaultHTTPClient for nil")
	}
}

func TestExchangeCode_BadURL(t *testing.T) {
	_, err := ExchangeCode(context.Background(), ExchangeRequest{TokenURL: "http://\x7f/bad", Config: mockProviderConfig(), Code: "code"})
	if err == nil {
		t.Fatal("expected request-construction error")
	}
}

func TestExchangeCode_ConnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()
	_, err := ExchangeCode(context.Background(), ExchangeRequest{TokenURL: url, Config: mockProviderConfig(), Code: "code"})
	if err == nil {
		t.Fatal("expected connection error")
	}
}

func TestExchangeCode_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()
	_, err := ExchangeCode(context.Background(), ExchangeRequest{TokenURL: srv.URL, Config: mockProviderConfig(), Code: "code"})
	if err == nil {
		t.Fatal("expected decode error")
	}
}

func TestExchangeJSON_Errors(t *testing.T) {
	_, err := ExchangeJSON(context.Background(), ExchangeRequest{TokenURL: "http://\x7f/bad", Config: mockProviderConfig(), Code: "code"})
	if err == nil {
		t.Fatal("expected request-construction error")
	}

	fail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no", http.StatusBadRequest)
	}))
	defer fail.Close()
	_, err = ExchangeJSON(context.Background(), ExchangeRequest{TokenURL: fail.URL, Config: mockProviderConfig(), Code: "code"})
	if err == nil {
		t.Fatal("expected HTTP error")
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer bad.Close()
	_, err = ExchangeJSON(context.Background(), ExchangeRequest{TokenURL: bad.URL, Config: mockProviderConfig(), Code: "code"})
	if err == nil {
		t.Fatal("expected parse error")
	}
}
