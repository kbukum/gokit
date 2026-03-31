package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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

// =============================================================================
// Built-in Provider Constructors
// =============================================================================

func TestGoogleConstructor(t *testing.T) {
	g := NewGoogle(ProviderConfig{ClientID: "id", ClientSecret: "secret"})

	if g.Name() != "google" {
		t.Errorf("Name() = %q, want 'google'", g.Name())
	}
	if g.Label() != "Google" {
		t.Errorf("Label() = %q, want 'Google'", g.Label())
	}
	if g.ProviderType() != "identity" {
		t.Errorf("ProviderType() = %q, want 'identity'", g.ProviderType())
	}
}

func TestGitHubConstructor(t *testing.T) {
	g := NewGitHub(ProviderConfig{ClientID: "id"})

	if g.Name() != "github" {
		t.Errorf("Name() = %q, want 'github'", g.Name())
	}
	if g.Label() != "GitHub" {
		t.Errorf("Label() = %q, want 'GitHub'", g.Label())
	}
	if g.ProviderType() != "identity" {
		t.Errorf("ProviderType() = %q, want 'identity'", g.ProviderType())
	}
}

func TestAppleConstructor(t *testing.T) {
	a := NewApple(AppleConfig{ProviderConfig: ProviderConfig{ClientID: "id"}})

	if a.Name() != "apple" {
		t.Errorf("Name() = %q, want 'apple'", a.Name())
	}
	if a.Label() != "Apple" {
		t.Errorf("Label() = %q, want 'Apple'", a.Label())
	}
	if a.ProviderType() != "identity" {
		t.Errorf("ProviderType() = %q, want 'identity'", a.ProviderType())
	}
}

// =============================================================================
// Default Scopes
// =============================================================================

func TestDefaultScopes(t *testing.T) {
	tests := []struct {
		name   string
		scopes []string
		want   string
	}{
		{"google", NewGoogle(ProviderConfig{ClientID: "id"}).cfg.Scopes, "openid"},
		{"github", NewGitHub(ProviderConfig{ClientID: "id"}).cfg.Scopes, "read:user"},
		{"apple", NewApple(AppleConfig{ProviderConfig: ProviderConfig{ClientID: "id"}}).cfg.Scopes, "name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.scopes) == 0 || tt.scopes[0] != tt.want {
				t.Errorf("scopes = %v, want first=%q", tt.scopes, tt.want)
			}
		})
	}
}

func TestCustomScopesPreserved(t *testing.T) {
	custom := []string{"my:scope", "other:scope"}
	g := NewGoogle(ProviderConfig{ClientID: "id", Scopes: custom})

	if len(g.cfg.Scopes) != 2 || g.cfg.Scopes[0] != "my:scope" {
		t.Errorf("custom scopes overwritten: got %v, want %v", g.cfg.Scopes, custom)
	}
}

func TestWithDefaultScopes(t *testing.T) {
	// Empty scopes → defaults applied
	cfg := WithDefaultScopes(ProviderConfig{ClientID: "id"}, "a", "b")
	if len(cfg.Scopes) != 2 || cfg.Scopes[0] != "a" {
		t.Errorf("expected defaults [a b], got %v", cfg.Scopes)
	}

	// Non-empty scopes → preserved
	cfg = WithDefaultScopes(ProviderConfig{ClientID: "id", Scopes: []string{"x"}}, "a", "b")
	if len(cfg.Scopes) != 1 || cfg.Scopes[0] != "x" {
		t.Errorf("expected custom [x], got %v", cfg.Scopes)
	}
}

// =============================================================================
// GenericProvider Meta Defaults
// =============================================================================

func TestGenericProviderMetaDefaults(t *testing.T) {
	// No label → defaults to name
	p := NewGeneric(GenericConfig{ProviderName: "test"})
	if p.Label() != "test" {
		t.Errorf("Label() = %q, want 'test' (default from name)", p.Label())
	}
	if p.ProviderType() != "identity" {
		t.Errorf("ProviderType() = %q, want 'identity' (default)", p.ProviderType())
	}

	// Custom label and type
	p2 := NewGeneric(GenericConfig{ProviderName: "yt", Label: "YouTube", Type: "social"})
	if p2.Label() != "YouTube" {
		t.Errorf("Label() = %q, want 'YouTube'", p2.Label())
	}
	if p2.ProviderType() != "social" {
		t.Errorf("ProviderType() = %q, want 'social'", p2.ProviderType())
	}
}

// =============================================================================
// Auth URL Generation
// =============================================================================

func TestGoogleAuthURL(t *testing.T) {
	g := NewGoogle(ProviderConfig{
		ClientID:    "my-id",
		RedirectURL: "http://localhost/callback",
	})
	u := g.AuthURL("test-state")

	for _, want := range []string{
		"accounts.google.com",
		"client_id=my-id",
		"state=test-state",
		"access_type=offline",
		"prompt=consent",
	} {
		if !strings.Contains(u, want) {
			t.Errorf("AuthURL missing %q", want)
		}
	}
}

func TestGitHubAuthURL(t *testing.T) {
	g := NewGitHub(ProviderConfig{
		ClientID:    "gh-id",
		RedirectURL: "http://localhost/callback",
		Scopes:      []string{"read:user", "user:email"},
	})
	u := g.AuthURL("my-state")

	if !strings.Contains(u, "github.com/login/oauth/authorize") {
		t.Error("expected github auth URL")
	}
	if !strings.Contains(u, "client_id=gh-id") {
		t.Error("expected client_id")
	}
}

func TestAppleAuthURL(t *testing.T) {
	a := NewApple(AppleConfig{
		ProviderConfig: ProviderConfig{
			ClientID:    "apple-id",
			RedirectURL: "http://localhost/callback",
		},
	})
	u := a.AuthURL("apple-state")

	if !strings.Contains(u, "appleid.apple.com") {
		t.Error("expected apple auth URL")
	}
	if !strings.Contains(u, "response_mode=form_post") {
		t.Error("expected response_mode=form_post")
	}
}

func TestAuthURLWithPKCE(t *testing.T) {
	pkce, err := oidc.NewPKCE()
	if err != nil {
		t.Fatal(err)
	}

	g := NewGoogle(ProviderConfig{ClientID: "id", RedirectURL: "http://test"})
	u := g.AuthURL("state", oidc.WithPKCE(pkce))

	if !strings.Contains(u, "code_challenge=") {
		t.Error("expected code_challenge in URL")
	}
	if !strings.Contains(u, "code_challenge_method=S256") {
		t.Error("expected code_challenge_method=S256")
	}
}

func TestAuthURLWithOverrides(t *testing.T) {
	g := NewGoogle(ProviderConfig{
		ClientID:    "id",
		RedirectURL: "http://original",
		Scopes:      []string{"openid"},
	})

	u := g.AuthURL("state",
		oidc.WithRedirectURI("http://override"),
		oidc.WithScopes("openid", "email"),
		oidc.WithExtraParam("login_hint", "user@example.com"),
	)

	if !strings.Contains(u, "redirect_uri=http%3A%2F%2Foverride") {
		t.Error("expected overridden redirect_uri")
	}
	if !strings.Contains(u, "login_hint=user%40example.com") {
		t.Error("expected login_hint")
	}
}

func TestAuthURLWithNonce(t *testing.T) {
	g := NewGoogle(ProviderConfig{ClientID: "id", RedirectURL: "http://test"})
	u := g.AuthURL("state", oidc.WithNonce("my-nonce"))

	if !strings.Contains(u, "nonce=my-nonce") {
		t.Errorf("expected nonce param in URL, got: %s", u)
	}
}

func TestAuthURLCustomClientIDParam(t *testing.T) {
	p := NewGeneric(GenericConfig{
		ProviderConfig: ProviderConfig{ClientID: "tk-key", RedirectURL: "http://test"},
		ProviderName:   "tiktok-like",
		AuthEndpoint:   "https://example.com/auth",
		ClientIDParam:  "client_key",
	})
	u := p.AuthURL("state")

	if !strings.Contains(u, "client_key=tk-key") {
		t.Errorf("expected client_key param, got: %s", u)
	}
	if strings.Contains(u, "client_id=") {
		t.Errorf("should not have client_id param when ClientIDParam is set")
	}
}

func TestAuthURLCustomScopeSeparator(t *testing.T) {
	p := NewGeneric(GenericConfig{
		ProviderConfig: ProviderConfig{
			ClientID:    "id",
			RedirectURL: "http://test",
			Scopes:      []string{"user.info.basic", "video.list"},
		},
		ProviderName:   "tiktok-like",
		AuthEndpoint:   "https://example.com/auth",
		ScopeSeparator: ",",
	})
	u := p.AuthURL("state")

	// Scopes joined by comma (URL-encoded as %2C)
	if !strings.Contains(u, "user.info.basic%2Cvideo.list") && !strings.Contains(u, "user.info.basic,video.list") {
		t.Errorf("expected comma-separated scopes, got: %s", u)
	}
}

// =============================================================================
// Token Exchange (Form + JSON)
// =============================================================================

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
		json.NewEncoder(w).Encode(map[string]interface{}{
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

// =============================================================================
// ClientSecretFunc (Apple-style dynamic secrets)
// =============================================================================

func TestClientSecretFunc(t *testing.T) {
	srv := testutil.NewMockOAuthServer()
	defer srv.Close()

	secretCalled := false
	p := NewGeneric(GenericConfig{
		ProviderConfig: ProviderConfig{
			ClientID:     "apple-client",
			ClientSecret: "static-secret-should-be-overridden",
			RedirectURL:  "http://test",
		},
		ProviderName:  "apple-like",
		TokenEndpoint: srv.TokenURL(),
		ClientSecretFunc: func() (string, error) {
			secretCalled = true
			return "dynamic-jwt-secret", nil
		},
	})

	_, err := p.Exchange(context.Background(), "code")
	if err != nil {
		t.Fatal(err)
	}
	if !secretCalled {
		t.Error("ClientSecretFunc was not called")
	}

	req := srv.LastTokenRequest()
	if req["client_secret"] != "dynamic-jwt-secret" {
		t.Errorf("client_secret = %q, want 'dynamic-jwt-secret'", req["client_secret"])
	}
}

func TestClientSecretFuncError(t *testing.T) {
	srv := testutil.NewMockOAuthServer()
	defer srv.Close()

	p := NewGeneric(GenericConfig{
		ProviderConfig: mockProviderConfig(),
		ProviderName:   "secret-fail",
		TokenEndpoint:  srv.TokenURL(),
		ClientSecretFunc: func() (string, error) {
			return "", fmt.Errorf("key expired")
		},
	})

	_, err := p.Exchange(context.Background(), "code")
	if err == nil {
		t.Fatal("expected error when ClientSecretFunc fails")
	}
	if !strings.Contains(err.Error(), "key expired") {
		t.Errorf("error = %q, should contain 'key expired'", err)
	}
}

// =============================================================================
// UserInfo Fetching
// =============================================================================

func TestUserInfo(t *testing.T) {
	srv := testutil.NewMockOAuthServer()
	defer srv.Close()

	p := NewGeneric(GenericConfig{
		ProviderConfig:   mockProviderConfig(),
		ProviderName:     "test",
		UserInfoEndpoint: srv.UserInfoURL(),
		UserInfo:         standardMapper(),
	})

	user, err := p.UserInfo(context.Background(), "test-token")
	if err != nil {
		t.Fatal(err)
	}
	if user.Subject != "user-123" {
		t.Errorf("Subject = %q, want 'user-123'", user.Subject)
	}
	if user.Email != "user@example.com" {
		t.Errorf("Email = %q, want 'user@example.com'", user.Email)
	}
	if !user.EmailVerified {
		t.Error("expected EmailVerified=true")
	}
	if user.Name != "Test User" {
		t.Errorf("Name = %q, want 'Test User'", user.Name)
	}
	if user.GivenName != "Test" {
		t.Errorf("GivenName = %q, want 'Test'", user.GivenName)
	}
	if user.FamilyName != "User" {
		t.Errorf("FamilyName = %q, want 'User'", user.FamilyName)
	}
	if user.Picture != "https://example.com/photo.jpg" {
		t.Errorf("Picture = %q, want 'https://example.com/photo.jpg'", user.Picture)
	}
	if user.Locale != "en" {
		t.Errorf("Locale = %q, want 'en'", user.Locale)
	}
}

func TestUserInfoError(t *testing.T) {
	srv := testutil.NewMockOAuthServer()
	defer srv.Close()
	srv.FailUserInfo(true)

	p := NewGeneric(GenericConfig{
		ProviderConfig:   mockProviderConfig(),
		ProviderName:     "fail-test",
		UserInfoEndpoint: srv.UserInfoURL(),
		UserInfo:         standardMapper(),
	})

	_, err := p.UserInfo(context.Background(), "bad-token")
	if err == nil {
		t.Fatal("expected error for failed userinfo")
	}
}

func TestUserInfoNoEndpoint(t *testing.T) {
	p := NewGeneric(GenericConfig{
		ProviderConfig: ProviderConfig{ClientID: "id"},
		ProviderName:   "no-endpoint",
	})

	_, err := p.UserInfo(context.Background(), "token")
	if err == nil {
		t.Fatal("expected error when no userinfo endpoint")
	}
}

func TestUserInfoWithAccessTokenPlaceholder(t *testing.T) {
	var receivedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path + "?" + r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "u1",
		})
	}))
	defer server.Close()

	p := NewGeneric(GenericConfig{
		ProviderConfig:   ProviderConfig{ClientID: "id", RedirectURL: "http://test"},
		ProviderName:     "placeholder-test",
		UserInfoEndpoint: server.URL + "/me?access_token={access_token}",
		UserInfo:         UserInfoMapper{SubjectKey: "id"},
	})

	_, err := p.UserInfo(context.Background(), "my-tok-123")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(receivedPath, "access_token=my-tok-123") {
		t.Errorf("expected access_token in URL, got path: %s", receivedPath)
	}
}

func TestUserInfoIDTokenAsUserInfo(t *testing.T) {
	p := NewGeneric(GenericConfig{
		ProviderConfig:    ProviderConfig{ClientID: "id"},
		ProviderName:      "apple-like",
		IDTokenAsUserInfo: true,
	})

	_, err := p.UserInfo(context.Background(), "token")
	if err == nil {
		t.Fatal("expected error for IDTokenAsUserInfo provider")
	}
	if !strings.Contains(err.Error(), "use ID token claims") {
		t.Errorf("error = %q, should mention ID token", err)
	}
}

// =============================================================================
// PostUserInfoHook (GitHub email fallback pattern)
// =============================================================================

func TestPostUserInfoHook(t *testing.T) {
	srv := testutil.NewMockOAuthServer()
	defer srv.Close()

	hookCalled := false
	p := NewGeneric(GenericConfig{
		ProviderConfig:   mockProviderConfig(),
		ProviderName:     "hook-test",
		TokenEndpoint:    srv.TokenURL(),
		UserInfoEndpoint: srv.UserInfoURL(),
		UserInfo:         standardMapper(),
		PostUserInfoHook: func(_ context.Context, token string, info *oidc.UserInfo) error {
			hookCalled = true
			if token == "" {
				t.Error("hook received empty token")
			}
			info.Email = "enriched@test.com"
			return nil
		},
	})

	user, err := p.UserInfo(context.Background(), "tok")
	if err != nil {
		t.Fatal(err)
	}
	if !hookCalled {
		t.Error("PostUserInfoHook was not called")
	}
	if user.Email != "enriched@test.com" {
		t.Errorf("Email = %q, want 'enriched@test.com'", user.Email)
	}
}

func TestPostUserInfoHookErrorNonFatal(t *testing.T) {
	srv := testutil.NewMockOAuthServer()
	defer srv.Close()

	p := NewGeneric(GenericConfig{
		ProviderConfig:   mockProviderConfig(),
		ProviderName:     "hook-err",
		UserInfoEndpoint: srv.UserInfoURL(),
		UserInfo:         standardMapper(),
		PostUserInfoHook: func(_ context.Context, _ string, _ *oidc.UserInfo) error {
			return fmt.Errorf("secondary API failed")
		},
	})

	// Hook error is non-fatal — should still return base user info
	user, err := p.UserInfo(context.Background(), "tok")
	if err != nil {
		t.Fatal(err)
	}
	if user.Subject != "user-123" {
		t.Errorf("should still get base userinfo, got Subject=%q", user.Subject)
	}
}

// =============================================================================
// PostExchangeHook (Instagram long-lived token pattern)
// =============================================================================

func TestPostExchangeHook(t *testing.T) {
	srv := testutil.NewMockOAuthServer()
	defer srv.Close()

	hookCalled := false
	p := NewGeneric(GenericConfig{
		ProviderConfig: mockProviderConfig(),
		ProviderName:   "ig-like",
		TokenEndpoint:  srv.TokenURL(),
		PostExchangeHook: func(_ context.Context, cfg ProviderConfig, token *oidc.TokenResult) (*oidc.TokenResult, error) {
			hookCalled = true
			// Simulate exchanging short-lived for long-lived token
			return &oidc.TokenResult{
				AccessToken:  "long-lived-token",
				RefreshToken: token.RefreshToken,
				TokenType:    token.TokenType,
			}, nil
		},
	})

	tokens, err := p.Exchange(context.Background(), "short-code")
	if err != nil {
		t.Fatal(err)
	}
	if !hookCalled {
		t.Error("PostExchangeHook was not called")
	}
	if tokens.AccessToken != "long-lived-token" {
		t.Errorf("AccessToken = %q, want 'long-lived-token'", tokens.AccessToken)
	}
}

func TestPostExchangeHookErrorFallsBack(t *testing.T) {
	srv := testutil.NewMockOAuthServer()
	defer srv.Close()

	p := NewGeneric(GenericConfig{
		ProviderConfig: mockProviderConfig(),
		ProviderName:   "ig-fail",
		TokenEndpoint:  srv.TokenURL(),
		PostExchangeHook: func(_ context.Context, _ ProviderConfig, _ *oidc.TokenResult) (*oidc.TokenResult, error) {
			return nil, fmt.Errorf("long-lived exchange failed")
		},
	})

	// Hook error is non-fatal — should fall back to original token
	tokens, err := p.Exchange(context.Background(), "code")
	if err != nil {
		t.Fatal(err)
	}
	if tokens.AccessToken != "mock-access-token" {
		t.Errorf("should fall back to original token, got %q", tokens.AccessToken)
	}
}

// =============================================================================
// NestedMap & ResponsePath (TikTok nested user data pattern)
// =============================================================================

func TestNestedMap(t *testing.T) {
	raw := map[string]interface{}{
		"data": map[string]interface{}{
			"user": map[string]interface{}{
				"open_id":      "tk-123",
				"display_name": "TikToker",
			},
		},
	}

	nested := NestedMap(raw, "data.user")
	if nested == nil {
		t.Fatal("NestedMap returned nil")
	}
	if StrVal(nested, "open_id") != "tk-123" {
		t.Errorf("open_id = %q, want 'tk-123'", StrVal(nested, "open_id"))
	}
}

func TestNestedMapEmptyPath(t *testing.T) {
	raw := map[string]interface{}{"key": "val"}
	result := NestedMap(raw, "")
	if result == nil || StrVal(result, "key") != "val" {
		t.Error("empty path should return the input map")
	}
}

func TestNestedMapMissing(t *testing.T) {
	raw := map[string]interface{}{"other": "data"}
	if NestedMap(raw, "data.user") != nil {
		t.Error("expected nil for missing path")
	}
}

func TestNestedMapNonMapSegment(t *testing.T) {
	raw := map[string]interface{}{"data": "not-a-map"}
	if NestedMap(raw, "data.user") != nil {
		t.Error("expected nil when path segment is not a map")
	}
}

func TestResponsePath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"user": map[string]interface{}{
					"open_id":      "tk-456",
					"display_name": "TikTok User",
					"avatar_url":   "https://photo.tiktok.com/pic",
				},
			},
		})
	}))
	defer server.Close()

	p := NewGeneric(GenericConfig{
		ProviderConfig:   ProviderConfig{ClientID: "id", RedirectURL: "http://test"},
		ProviderName:     "nested-test",
		UserInfoEndpoint: server.URL + "/userinfo",
		UserInfo: UserInfoMapper{
			SubjectKey:   "open_id",
			NameKey:      "display_name",
			PictureKey:   "avatar_url",
			ResponsePath: "data.user",
		},
	})

	user, err := p.UserInfo(context.Background(), "tok")
	if err != nil {
		t.Fatal(err)
	}
	if user.Subject != "tk-456" {
		t.Errorf("Subject = %q, want 'tk-456'", user.Subject)
	}
	if user.Name != "TikTok User" {
		t.Errorf("Name = %q, want 'TikTok User'", user.Name)
	}
}

// =============================================================================
// Numeric Subject ID (GitHub returns int ID)
// =============================================================================

func TestNumericSubjectFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":   42, // numeric, not string
			"name": "Octocat",
		})
	}))
	defer server.Close()

	p := NewGeneric(GenericConfig{
		ProviderConfig:   ProviderConfig{ClientID: "id", RedirectURL: "http://test"},
		ProviderName:     "num-id",
		UserInfoEndpoint: server.URL + "/user",
		UserInfo:         UserInfoMapper{SubjectKey: "id", NameKey: "name"},
	})

	user, err := p.UserInfo(context.Background(), "tok")
	if err != nil {
		t.Fatal(err)
	}
	if user.Subject != "42" {
		t.Errorf("Subject = %q, want '42' (converted from numeric)", user.Subject)
	}
}

// =============================================================================
// Helper Functions
// =============================================================================

func TestStrVal(t *testing.T) {
	m := map[string]interface{}{"name": "Alice", "count": 42}
	if StrVal(m, "name") != "Alice" {
		t.Error("expected 'Alice'")
	}
	if StrVal(m, "count") != "" {
		t.Error("expected '' for non-string value")
	}
	if StrVal(m, "missing") != "" {
		t.Error("expected '' for missing key")
	}
}

func TestBoolVal(t *testing.T) {
	m := map[string]interface{}{"verified": true, "active": false, "name": "test"}
	if !BoolVal(m, "verified") {
		t.Error("expected true")
	}
	if BoolVal(m, "active") {
		t.Error("expected false")
	}
	if BoolVal(m, "name") {
		t.Error("expected false for non-bool")
	}
	if BoolVal(m, "missing") {
		t.Error("expected false for missing key")
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

// =============================================================================
// ParseIDTokenClaims
// =============================================================================

func TestParseIDTokenClaims(t *testing.T) {
	idToken := testutil.BuildTestIDToken(map[string]interface{}{
		"sub":            "user-123",
		"email":          "user@example.com",
		"email_verified": true,
		"name":           "Test User",
		"given_name":     "Test",
		"family_name":    "User",
		"picture":        "https://example.com/photo.jpg",
		"locale":         "en",
	})

	user, err := oidc.ParseIDTokenClaims(idToken)
	if err != nil {
		t.Fatal(err)
	}
	if user.Subject != "user-123" {
		t.Errorf("Subject = %q, want 'user-123'", user.Subject)
	}
	if user.Email != "user@example.com" {
		t.Errorf("Email = %q", user.Email)
	}
	if !user.EmailVerified {
		t.Error("expected email_verified=true")
	}
	if user.Name != "Test User" {
		t.Errorf("Name = %q", user.Name)
	}
	if user.GivenName != "Test" {
		t.Errorf("GivenName = %q", user.GivenName)
	}
	if user.FamilyName != "User" {
		t.Errorf("FamilyName = %q", user.FamilyName)
	}
	if user.Picture != "https://example.com/photo.jpg" {
		t.Errorf("Picture = %q", user.Picture)
	}
}

func TestParseIDTokenClaims_EmailVerifiedAsString(t *testing.T) {
	idToken := testutil.BuildTestIDToken(map[string]interface{}{
		"sub":            "u1",
		"email_verified": "true",
	})
	user, err := oidc.ParseIDTokenClaims(idToken)
	if err != nil {
		t.Fatal(err)
	}
	if !user.EmailVerified {
		t.Error("expected email_verified=true when value is string 'true'")
	}
}

func TestParseIDTokenClaims_InvalidFormat(t *testing.T) {
	_, err := oidc.ParseIDTokenClaims("not-a-jwt")
	if err == nil {
		t.Error("expected error for invalid token")
	}
}

// =============================================================================
// Low-level Helpers (ExchangeCode, ExchangeJSON, FetchJSON)
// =============================================================================

func TestExchangeCodeDirect(t *testing.T) {
	srv := testutil.NewMockOAuthServer()
	defer srv.Close()

	cfg := mockProviderConfig()
	tok, err := ExchangeCode(context.Background(), srv.TokenURL(), cfg, "code", oidc.ExchangeOptions{}, nil)
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
	_, err := ExchangeCode(context.Background(), srv.TokenURL(), cfg, "code", opts, nil)
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
	_, err := ExchangeCode(context.Background(), srv.TokenURL(), cfg, "code", opts, nil)
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
	tok, err := ExchangeJSON(context.Background(), srv.TokenURL(), cfg, "json-code", oidc.ExchangeOptions{}, "", nil)
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
	_, err := ExchangeJSON(context.Background(), srv.TokenURL(), cfg, "code", oidc.ExchangeOptions{}, "client_key", nil)
	if err != nil {
		t.Fatal(err)
	}

	req := srv.LastTokenRequest()
	if req["client_key"] != "test-client-id" {
		t.Errorf("client_key = %q, want 'test-client-id'", req["client_key"])
	}
}

func TestFetchJSON(t *testing.T) {
	srv := testutil.NewMockOAuthServer()
	defer srv.Close()

	var raw map[string]interface{}
	err := FetchJSON(context.Background(), srv.UserInfoURL(), "test-token", &raw)
	if err != nil {
		t.Fatal(err)
	}
	if StrVal(raw, "email") != "user@example.com" {
		t.Errorf("email = %q", StrVal(raw, "email"))
	}
}

// =============================================================================
// GitHub Email Fallback (full integration)
// =============================================================================

func TestGitHubEmailFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/token":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token": "gh-tok",
				"token_type":   "bearer",
			})
		case "/user":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":         42,
				"name":       "Octocat",
				"email":      nil, // email is private
				"avatar_url": "https://avatar.url",
			})
		case "/user/emails":
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{"email": "secondary@gh.com", "primary": false, "verified": true},
				{"email": "primary@gh.com", "primary": true, "verified": true},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	p := NewGeneric(GenericConfig{
		ProviderConfig:   ProviderConfig{ClientID: "id", ClientSecret: "s", RedirectURL: "http://test"},
		ProviderName:     "github-mock",
		TokenEndpoint:    server.URL + "/token",
		UserInfoEndpoint: server.URL + "/user",
		TokenExtraHeaders: map[string]string{
			"Accept": "application/json",
		},
		UserInfo: UserInfoMapper{
			SubjectKey: "id",
			EmailKey:   "email",
			NameKey:    "name",
			PictureKey: "avatar_url",
		},
		PostUserInfoHook: newGitHubEmailFallback(server.URL + "/user/emails"),
	})

	tokens, err := p.Exchange(context.Background(), "code")
	if err != nil {
		t.Fatal(err)
	}

	user, err := p.UserInfo(context.Background(), tokens.AccessToken)
	if err != nil {
		t.Fatal(err)
	}

	if user.Subject != "42" {
		t.Errorf("Subject = %q, want '42'", user.Subject)
	}
	if user.Email != "primary@gh.com" {
		t.Errorf("Email = %q, want 'primary@gh.com' (from fallback)", user.Email)
	}
	if !user.EmailVerified {
		t.Error("expected EmailVerified=true")
	}
}

func TestGitHubEmailNotNeeded(t *testing.T) {
	srv := testutil.NewMockOAuthServer()
	defer srv.Close()

	emailEndpointCalled := false
	emailServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		emailEndpointCalled = true
	}))
	defer emailServer.Close()

	hook := newGitHubEmailFallback(emailServer.URL + "/user/emails")
	info := &oidc.UserInfo{Email: "already@here.com"}
	err := hook(context.Background(), "tok", info)
	if err != nil {
		t.Fatal(err)
	}
	if emailEndpointCalled {
		t.Error("should not call email endpoint when email is already present")
	}
	if !info.EmailVerified {
		t.Error("should set EmailVerified=true when email present")
	}
}

// =============================================================================
// Manager
// =============================================================================

func TestManagerRegisterAndList(t *testing.T) {
	g := NewGoogle(ProviderConfig{ClientID: "g"})
	gh := NewGitHub(ProviderConfig{ClientID: "gh"})
	m := NewManager(g, gh)

	names := m.List()
	if len(names) != 2 {
		t.Errorf("len(List()) = %d, want 2", len(names))
	}

	p, err := m.Get("google")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name() != "google" {
		t.Errorf("Get('google').Name() = %q", p.Name())
	}

	_, err = m.Get("unknown")
	if err == nil {
		t.Error("expected error for unknown provider")
	}
}

func TestManagerRegisterDynamic(t *testing.T) {
	m := NewManager()
	if len(m.List()) != 0 {
		t.Error("expected empty manager")
	}

	m.Register(NewGoogle(ProviderConfig{ClientID: "g"}))
	if len(m.List()) != 1 {
		t.Error("expected 1 provider after register")
	}
}

func TestManagerListProviders(t *testing.T) {
	g := NewGoogle(ProviderConfig{ClientID: "g"})
	gh := NewGitHub(ProviderConfig{ClientID: "gh"})
	m := NewManager(g, gh)

	infos := m.ListProviders()
	if len(infos) != 2 {
		t.Fatalf("len(ListProviders()) = %d, want 2", len(infos))
	}

	// Sorted by name: github, google
	if infos[0].Name != "github" || infos[0].Label != "GitHub" || infos[0].Type != "identity" {
		t.Errorf("unexpected github info: %+v", infos[0])
	}
	if infos[1].Name != "google" || infos[1].Label != "Google" || infos[1].Type != "identity" {
		t.Errorf("unexpected google info: %+v", infos[1])
	}
}

func TestManagerListProvidersWithSocial(t *testing.T) {
	yt := NewGeneric(GenericConfig{ProviderName: "youtube", Label: "YouTube", Type: "social"})
	g := NewGoogle(ProviderConfig{ClientID: "g"})
	m := NewManager(g, yt)

	infos := m.ListProviders()
	// Sorted: google, youtube
	if infos[1].Name != "youtube" || infos[1].Type != "social" {
		t.Errorf("unexpected youtube info: %+v", infos[1])
	}
}

func TestManagerAuthURL(t *testing.T) {
	g := NewGoogle(ProviderConfig{ClientID: "id", RedirectURL: "http://test"})
	m := NewManager(g)

	u, err := m.AuthURL("google", "state123")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(u, "state=state123") {
		t.Error("expected state in URL")
	}
}

func TestManagerAuthURLUnknown(t *testing.T) {
	m := NewManager()
	_, err := m.AuthURL("nonexistent", "state")
	if err == nil {
		t.Error("expected error for unknown provider")
	}
}

func TestManagerExchange(t *testing.T) {
	srv := testutil.NewMockOAuthServer()
	defer srv.Close()

	cfg := mockGenericConfig(srv, "test")
	p := NewGeneric(cfg)
	m := NewManager(p)

	tokens, err := m.Exchange(context.Background(), "test", "code")
	if err != nil {
		t.Fatal(err)
	}
	if tokens.AccessToken != "mock-access-token" {
		t.Errorf("AccessToken = %q", tokens.AccessToken)
	}
}

func TestManagerUserInfo(t *testing.T) {
	srv := testutil.NewMockOAuthServer()
	defer srv.Close()

	cfg := mockGenericConfig(srv, "test")
	p := NewGeneric(cfg)
	m := NewManager(p)

	user, err := m.UserInfo(context.Background(), "test", "tok")
	if err != nil {
		t.Fatal(err)
	}
	if user.Subject != "user-123" {
		t.Errorf("Subject = %q", user.Subject)
	}
}

// =============================================================================
// Manager.ExchangeAndUserInfo + ID Token Fallback
// =============================================================================

func TestManagerExchangeAndUserInfo(t *testing.T) {
	srv := testutil.NewMockOAuthServer()
	defer srv.Close()

	cfg := mockGenericConfig(srv, "test")
	p := NewGeneric(cfg)
	m := NewManager(p)

	tokens, user, err := m.ExchangeAndUserInfo(context.Background(), "test", "code")
	if err != nil {
		t.Fatal(err)
	}
	if tokens.AccessToken != "mock-access-token" {
		t.Errorf("AccessToken = %q", tokens.AccessToken)
	}
	if user.Subject != "user-123" {
		t.Errorf("Subject = %q", user.Subject)
	}
}

func TestManagerExchangeAndUserInfo_IDTokenFallback(t *testing.T) {
	srv := testutil.NewMockOAuthServer()
	defer srv.Close()
	// Fail userinfo to trigger fallback
	srv.FailUserInfo(true)
	// Set ID token claims on the token response
	srv.SetIDTokenClaims(map[string]interface{}{
		"sub":            "apple-user-456",
		"email":          "apple@example.com",
		"email_verified": true,
		"name":           "Apple User",
	})

	// Apple-like provider: no userinfo endpoint, uses ID token
	p := NewGeneric(GenericConfig{
		ProviderConfig:    mockProviderConfig(),
		ProviderName:      "apple-mock",
		TokenEndpoint:     srv.TokenURL(),
		UserInfoEndpoint:  srv.UserInfoURL(),
		IDTokenAsUserInfo: true,
	})
	m := NewManager(p)

	tokens, user, err := m.ExchangeAndUserInfo(context.Background(), "apple-mock", "code")
	if err != nil {
		t.Fatal(err)
	}
	if tokens.AccessToken != "mock-access-token" {
		t.Errorf("AccessToken = %q", tokens.AccessToken)
	}
	// User info should come from ID token fallback
	if user.Subject != "apple-user-456" {
		t.Errorf("Subject = %q, want 'apple-user-456' (from ID token)", user.Subject)
	}
	if user.Email != "apple@example.com" {
		t.Errorf("Email = %q, want 'apple@example.com'", user.Email)
	}
}

func TestManagerExchangeAndUserInfo_NoIDTokenNoUserInfo(t *testing.T) {
	srv := testutil.NewMockOAuthServer()
	defer srv.Close()
	srv.FailUserInfo(true)
	// No ID token claims set → no fallback available

	p := NewGeneric(GenericConfig{
		ProviderConfig:   mockProviderConfig(),
		ProviderName:     "broken",
		TokenEndpoint:    srv.TokenURL(),
		UserInfoEndpoint: srv.UserInfoURL(),
		UserInfo:         standardMapper(),
	})
	m := NewManager(p)

	tokens, user, err := m.ExchangeAndUserInfo(context.Background(), "broken", "code")
	if err == nil {
		t.Fatal("expected error when both userinfo and ID token fail")
	}
	if tokens == nil {
		t.Error("should still return tokens even when userinfo fails")
	}
	if user != nil {
		t.Error("user should be nil when all fallbacks fail")
	}
}

// =============================================================================
// Comma-Separated Scopes in Token Response (TikTok pattern)
// =============================================================================

func TestCommaSeparatedScopesInResponse(t *testing.T) {
	srv := testutil.NewMockOAuthServer()
	defer srv.Close()
	srv.SetTokenResponse(map[string]interface{}{
		"access_token": "tok",
		"token_type":   "Bearer",
		"scope":        "user.info.basic,video.list",
	})

	p := NewGeneric(GenericConfig{
		ProviderConfig:     mockProviderConfig(),
		ProviderName:       "tiktok-like",
		TokenEndpoint:      srv.TokenURL(),
		TokenRequestFormat: "json",
		ScopeSeparator:     ",",
	})

	tokens, err := p.Exchange(context.Background(), "code")
	if err != nil {
		t.Fatal(err)
	}
	if len(tokens.Scopes) != 2 {
		t.Fatalf("Scopes = %v, want 2 items", tokens.Scopes)
	}
	if tokens.Scopes[0] != "user.info.basic" || tokens.Scopes[1] != "video.list" {
		t.Errorf("Scopes = %v, want [user.info.basic video.list]", tokens.Scopes)
	}
}

// =============================================================================
// Full Flow Integration Tests
// =============================================================================

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
	srv.SetTokenResponse(map[string]interface{}{
		"access_token": "tiktok-token",
		"token_type":   "Bearer",
		"scope":        "user.info.basic,video.list",
	})
	srv.SetUserResponse(map[string]interface{}{
		"data": map[string]interface{}{
			"user": map[string]interface{}{
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

// =============================================================================
// MockOAuthServer Tests (validate test infrastructure)
// =============================================================================

func TestMockServerReset(t *testing.T) {
	srv := testutil.NewMockOAuthServer()
	defer srv.Close()

	cfg := mockProviderConfig()
	ExchangeCode(context.Background(), srv.TokenURL(), cfg, "code1", oidc.ExchangeOptions{}, nil)
	ExchangeCode(context.Background(), srv.TokenURL(), cfg, "code2", oidc.ExchangeOptions{}, nil)

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

	srv.SetTokenResponse(map[string]interface{}{
		"access_token": "custom-token",
		"token_type":   "Bearer",
	})

	cfg := mockProviderConfig()
	tok, err := ExchangeCode(context.Background(), srv.TokenURL(), cfg, "code", oidc.ExchangeOptions{}, nil)
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

// =============================================================================
// MockProvider Tests (validate mock for user testing)
// =============================================================================

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
