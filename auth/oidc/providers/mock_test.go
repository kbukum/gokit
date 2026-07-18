package providers

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/kbukum/gokit/auth/oidc"
	"github.com/kbukum/gokit/auth/oidc/testutil"
)

// standardMapper returns a UserInfoMapper matching the mock server's default response.
func standardMapper() UserInfoMapper {
	return UserInfoMapper{
		SubjectKey:       "sub",
		EmailKey:         "email",
		EmailVerifiedKey: "email_verified",
		NameKey:          "name",
		GivenNameKey:     "given_name",
		FamilyNameKey:    "family_name",
		PictureKey:       "picture",
		LocaleKey:        "locale",
	}
}

// mockProviderConfig returns a ProviderConfig with test credentials.
func mockProviderConfig() ProviderConfig {
	return ProviderConfig{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		RedirectURL:  "http://localhost/callback",
	}
}

// mockGenericConfig returns a GenericConfig pre-configured to talk to a mock server.
func mockGenericConfig(srv *testutil.MockOAuthServer, name string) GenericConfig {
	return GenericConfig{
		ProviderConfig:   mockProviderConfig(),
		ProviderName:     name,
		Label:            name,
		AuthEndpoint:     srv.AuthURL(),
		TokenEndpoint:    srv.TokenURL(),
		UserInfoEndpoint: srv.UserInfoURL(),
		UserInfo:         standardMapper(),
	}
}

func TestFullFlowGoogleLike(t *testing.T) {
	srv := testutil.NewMockOAuthServer()
	defer srv.Close()

	p := NewGeneric(GenericConfig{
		ProviderConfig:   mockProviderConfig(),
		ProviderName:     "google-mock",
		AuthEndpoint:     srv.AuthURL(),
		TokenEndpoint:    srv.TokenURL(),
		UserInfoEndpoint: srv.UserInfoURL(),
		UserInfo:         standardMapper(),
		AuthExtraParams:  map[string]string{"access_type": "offline"},
	})

	// 1. Generate auth URL
	u := p.AuthURL("state123")
	if !strings.Contains(u, "state=state123") {
		t.Error("missing state")
	}
	if !strings.Contains(u, "access_type=offline") {
		t.Error("missing access_type")
	}

	// 2. Exchange code
	tokens, err := p.Exchange(context.Background(), "auth-code")
	if err != nil {
		t.Fatal(err)
	}
	if tokens.AccessToken == "" {
		t.Error("empty access token")
	}

	// 3. Get user info
	user, err := p.UserInfo(context.Background(), tokens.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if user.Subject == "" || user.Email == "" {
		t.Errorf("incomplete user info: sub=%q email=%q", user.Subject, user.Email)
	}
}

func TestFullFlowTikTokLike(t *testing.T) {
	srv := testutil.NewMockOAuthServer()
	defer srv.Close()
	srv.SetTokenResponse(map[string]any{
		"access_token": "tiktok-token",
		"token_type":   "Bearer",
		"scope":        "user.info.basic,video.list",
	})
	srv.SetUserResponse(map[string]any{
		"data": map[string]any{
			"user": map[string]any{
				"open_id":      "tk-789",
				"display_name": "TikToker",
				"avatar_url":   "https://tiktok.photo",
			},
		},
	})

	p := NewGeneric(GenericConfig{
		ProviderConfig: ProviderConfig{
			ClientID:     "tk-key",
			ClientSecret: "tk-secret",
			RedirectURL:  "http://test/callback",
			Scopes:       []string{"user.info.basic", "video.list"},
		},
		ProviderName:       "tiktok-mock",
		Label:              "TikTok",
		Type:               "social",
		AuthEndpoint:       srv.AuthURL(),
		TokenEndpoint:      srv.TokenURL(),
		UserInfoEndpoint:   srv.UserInfoURL(),
		ClientIDParam:      "client_key",
		ScopeSeparator:     ",",
		TokenRequestFormat: "json",
		UserInfo: UserInfoMapper{
			SubjectKey:   "open_id",
			NameKey:      "display_name",
			PictureKey:   "avatar_url",
			ResponsePath: "data.user",
		},
	})

	// Auth URL uses client_key
	u := p.AuthURL("state")
	if !strings.Contains(u, "client_key=tk-key") {
		t.Error("expected client_key in auth URL")
	}

	// Exchange
	tokens, err := p.Exchange(context.Background(), "code")
	if err != nil {
		t.Fatal(err)
	}
	if tokens.AccessToken != "tiktok-token" {
		t.Errorf("AccessToken = %q", tokens.AccessToken)
	}

	// UserInfo with nested response
	user, err := p.UserInfo(context.Background(), tokens.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if user.Subject != "tk-789" {
		t.Errorf("Subject = %q, want 'tk-789'", user.Subject)
	}
	if user.Name != "TikToker" {
		t.Errorf("Name = %q, want 'TikToker'", user.Name)
	}

	// Meta
	if p.Label() != "TikTok" {
		t.Errorf("Label = %q", p.Label())
	}
	if p.ProviderType() != "social" {
		t.Errorf("ProviderType = %q", p.ProviderType())
	}
}

func TestMockServerReset(t *testing.T) {
	srv := testutil.NewMockOAuthServer()
	defer srv.Close()

	cfg := mockProviderConfig()
	ExchangeCode(context.Background(), ExchangeRequest{TokenURL: srv.TokenURL(), Config: cfg, Code: "code1"})
	ExchangeCode(context.Background(), ExchangeRequest{TokenURL: srv.TokenURL(), Config: cfg, Code: "code2"})

	if len(srv.TokenRequests()) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(srv.TokenRequests()))
	}

	srv.Reset()
	if len(srv.TokenRequests()) != 0 {
		t.Error("expected 0 requests after reset")
	}
}

func TestMockServerCustomResponses(t *testing.T) {
	srv := testutil.NewMockOAuthServer()
	defer srv.Close()

	srv.SetTokenResponse(map[string]any{
		"access_token": "custom-token",
		"token_type":   "Bearer",
	})

	cfg := mockProviderConfig()
	tok, err := ExchangeCode(context.Background(), ExchangeRequest{TokenURL: srv.TokenURL(), Config: cfg, Code: "code"})
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "custom-token" {
		t.Errorf("AccessToken = %q, want 'custom-token'", tok.AccessToken)
	}
}

func TestMockServerGenericConfig(t *testing.T) {
	srv := testutil.NewMockOAuthServer()
	defer srv.Close()

	cfg := mockGenericConfig(srv, "my-provider")
	if cfg.ProviderName != "my-provider" {
		t.Errorf("ProviderName = %q", cfg.ProviderName)
	}
	if cfg.ClientID != "test-client-id" {
		t.Errorf("ClientID = %q", cfg.ClientID)
	}
	if cfg.TokenEndpoint != srv.TokenURL() {
		t.Error("TokenEndpoint should point to mock server")
	}
}

func TestMockProviderBasic(t *testing.T) {
	mock := &testutil.MockProvider{
		ProviderName: "test",
		ExchangeResult: &oidc.TokenResult{
			AccessToken: "mock-tok",
		},
		UserInfoResult: &oidc.UserInfo{
			Subject: "u1",
			Email:   "u1@test.com",
		},
	}

	// Satisfies oidc.Provider
	var p oidc.Provider = mock
	if p.Name() != "test" {
		t.Errorf("Name() = %q", p.Name())
	}

	// Exchange
	tok, err := p.Exchange(context.Background(), "code")
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "mock-tok" {
		t.Errorf("AccessToken = %q", tok.AccessToken)
	}

	// UserInfo
	user, err := p.UserInfo(context.Background(), "tok")
	if err != nil {
		t.Fatal(err)
	}
	if user.Email != "u1@test.com" {
		t.Errorf("Email = %q", user.Email)
	}

	// Inspect calls
	if calls := mock.ExchangeCalls(); len(calls) != 1 || calls[0] != "code" {
		t.Errorf("ExchangeCalls = %v", calls)
	}
	if calls := mock.UserInfoCalls(); len(calls) != 1 || calls[0] != "tok" {
		t.Errorf("UserInfoCalls = %v", calls)
	}
}

func TestMockProviderMeta(t *testing.T) {
	mock := &testutil.MockProvider{
		ProviderName:    "youtube",
		ProviderLabel:   "YouTube",
		ProviderTypeStr: "social",
	}

	var meta oidc.ProviderMeta = mock
	if meta.Label() != "YouTube" {
		t.Errorf("Label() = %q", meta.Label())
	}
	if meta.ProviderType() != "social" {
		t.Errorf("ProviderType() = %q", meta.ProviderType())
	}
}

func TestMockProviderMetaDefaults(t *testing.T) {
	mock := &testutil.MockProvider{ProviderName: "test"}
	if mock.Label() != "test" {
		t.Errorf("Label() should default to name, got %q", mock.Label())
	}
	if mock.ProviderType() != "identity" {
		t.Errorf("ProviderType() should default to 'identity', got %q", mock.ProviderType())
	}
}

func TestMockProviderWithFuncs(t *testing.T) {
	callCount := 0
	mock := &testutil.MockProvider{
		ProviderName: "dynamic",
		ExchangeFunc: func(_ context.Context, code string, _ ...oidc.ExchangeOption) (*oidc.TokenResult, error) {
			callCount++
			return &oidc.TokenResult{AccessToken: "dyn-" + code}, nil
		},
	}

	tok, err := mock.Exchange(context.Background(), "abc")
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "dyn-abc" {
		t.Errorf("AccessToken = %q", tok.AccessToken)
	}
	if callCount != 1 {
		t.Errorf("callCount = %d", callCount)
	}
}

func TestMockProviderErrors(t *testing.T) {
	mock := &testutil.MockProvider{
		ProviderName: "err",
		ExchangeErr:  fmt.Errorf("exchange failed"),
		UserInfoErr:  fmt.Errorf("userinfo failed"),
	}

	_, err := mock.Exchange(context.Background(), "code")
	if err == nil || !strings.Contains(err.Error(), "exchange failed") {
		t.Errorf("Exchange error = %v", err)
	}

	_, err = mock.UserInfo(context.Background(), "tok")
	if err == nil || !strings.Contains(err.Error(), "userinfo failed") {
		t.Errorf("UserInfo error = %v", err)
	}
}

func TestMockProviderReset(t *testing.T) {
	mock := &testutil.MockProvider{
		ProviderName:   "reset-test",
		ExchangeResult: &oidc.TokenResult{AccessToken: "t"},
		UserInfoResult: &oidc.UserInfo{Subject: "u"},
	}

	mock.Exchange(context.Background(), "c1")
	mock.Exchange(context.Background(), "c2")
	mock.UserInfo(context.Background(), "t1")
	mock.AuthURL("state")

	mock.Reset()

	if len(mock.ExchangeCalls()) != 0 {
		t.Error("expected 0 exchange calls after reset")
	}
	if len(mock.UserInfoCalls()) != 0 {
		t.Error("expected 0 userinfo calls after reset")
	}
	if len(mock.AuthURLCalls()) != 0 {
		t.Error("expected 0 authurl calls after reset")
	}
}

func TestMockProviderAuthURL(t *testing.T) {
	mock := &testutil.MockProvider{
		ProviderName:  "test",
		AuthURLResult: "https://custom.auth/url",
	}

	u := mock.AuthURL("state123", oidc.WithScopes("scope1", "scope2"))
	if u != "https://custom.auth/url" {
		t.Errorf("AuthURL = %q", u)
	}

	calls := mock.AuthURLCalls()
	if len(calls) != 1 {
		t.Fatalf("len(AuthURLCalls) = %d", len(calls))
	}
	if calls[0].State != "state123" {
		t.Errorf("State = %q", calls[0].State)
	}
	if len(calls[0].Opts) != 1 {
		t.Errorf("Opts = %v, want 1 option", calls[0].Opts)
	}
}

func TestMockProviderNoConfigError(t *testing.T) {
	mock := &testutil.MockProvider{ProviderName: "empty"}

	_, err := mock.Exchange(context.Background(), "code")
	if err == nil {
		t.Error("expected error when no exchange result configured")
	}

	_, err = mock.UserInfo(context.Background(), "tok")
	if err == nil {
		t.Error("expected error when no userinfo result configured")
	}
}
