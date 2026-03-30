package providers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kbukum/gokit/auth/oidc"
)

func TestGoogleName(t *testing.T) {
	g := NewGoogle(ProviderConfig{ClientID: "id", ClientSecret: "secret"})
	if g.Name() != "google" {
		t.Errorf("expected 'google', got %q", g.Name())
	}
}

func TestGoogleAuthURL(t *testing.T) {
	g := NewGoogle(ProviderConfig{
		ClientID:    "my-id",
		RedirectURL: "http://localhost/callback",
	})
	url := g.AuthURL("test-state")
	if !strings.Contains(url, "accounts.google.com") {
		t.Error("expected google auth URL")
	}
	if !strings.Contains(url, "client_id=my-id") {
		t.Error("expected client_id")
	}
	if !strings.Contains(url, "state=test-state") {
		t.Error("expected state")
	}
	if !strings.Contains(url, "access_type=offline") {
		t.Error("expected access_type=offline")
	}
}

func TestGitHubName(t *testing.T) {
	g := NewGitHub(ProviderConfig{ClientID: "id"})
	if g.Name() != "github" {
		t.Errorf("expected 'github', got %q", g.Name())
	}
}

func TestGitHubAuthURL(t *testing.T) {
	g := NewGitHub(ProviderConfig{
		ClientID:    "gh-id",
		RedirectURL: "http://localhost/callback",
		Scopes:      []string{"read:user", "user:email"},
	})
	url := g.AuthURL("my-state")
	if !strings.Contains(url, "github.com/login/oauth/authorize") {
		t.Error("expected github auth URL")
	}
	if !strings.Contains(url, "client_id=gh-id") {
		t.Error("expected client_id")
	}
}

func TestAppleName(t *testing.T) {
	a := NewApple(AppleConfig{ProviderConfig: ProviderConfig{ClientID: "id"}})
	if a.Name() != "apple" {
		t.Errorf("expected 'apple', got %q", a.Name())
	}
}

func TestAppleAuthURL(t *testing.T) {
	a := NewApple(AppleConfig{
		ProviderConfig: ProviderConfig{
			ClientID:    "apple-id",
			RedirectURL: "http://localhost/callback",
		},
	})
	url := a.AuthURL("apple-state")
	if !strings.Contains(url, "appleid.apple.com") {
		t.Error("expected apple auth URL")
	}
	if !strings.Contains(url, "response_mode=form_post") {
		t.Error("expected response_mode=form_post")
	}
}

func TestParseIDTokenClaims(t *testing.T) {
	payload := map[string]interface{}{
		"sub":            "apple-user-123",
		"email":          "user@example.com",
		"email_verified": true,
	}
	payloadJSON, _ := json.Marshal(payload)
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256"}`))
	body := base64.RawURLEncoding.EncodeToString(payloadJSON)
	sig := base64.RawURLEncoding.EncodeToString([]byte("fake-sig"))
	token := header + "." + body + "." + sig

	user, err := ParseIDTokenClaims(token)
	if err != nil {
		t.Fatal(err)
	}
	if user.Subject != "apple-user-123" {
		t.Errorf("expected sub=apple-user-123, got %q", user.Subject)
	}
	if user.Email != "user@example.com" {
		t.Errorf("expected email, got %q", user.Email)
	}
	if !user.EmailVerified {
		t.Error("expected email_verified=true")
	}
}

func TestParseIDTokenClaims_InvalidFormat(t *testing.T) {
	_, err := ParseIDTokenClaims("not-a-jwt")
	if err == nil {
		t.Error("expected error for invalid token")
	}
}

func TestManagerRegisterAndList(t *testing.T) {
	g := NewGoogle(ProviderConfig{ClientID: "g"})
	gh := NewGitHub(ProviderConfig{ClientID: "gh"})
	m := NewManager(g, gh)

	names := m.List()
	if len(names) != 2 {
		t.Errorf("expected 2 providers, got %d", len(names))
	}

	p, err := m.Get("google")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name() != "google" {
		t.Errorf("expected google, got %q", p.Name())
	}

	_, err = m.Get("unknown")
	if err == nil {
		t.Error("expected error for unknown provider")
	}
}

func TestManagerAuthURL(t *testing.T) {
	g := NewGoogle(ProviderConfig{ClientID: "id", RedirectURL: "http://test"})
	m := NewManager(g)

	url, err := m.AuthURL("google", "state123")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(url, "state=state123") {
		t.Error("expected state in URL")
	}
}

// TestGoogleExchange tests Exchange against a mock token server.
func TestGoogleExchange(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token": "mock-access-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
			return
		}
		if r.URL.Path == "/userinfo" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":             "12345",
				"email":          "user@gmail.com",
				"verified_email": true,
				"name":           "Test User",
				"picture":        "https://photo.url",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	// Create a Google provider that talks to our mock server
	g := &mockGoogleProvider{
		Google:       NewGoogle(ProviderConfig{ClientID: "id", ClientSecret: "secret", RedirectURL: "http://test"}),
		tokenURL:     server.URL + "/token",
		userInfoURL:  server.URL + "/userinfo",
	}

	tokens, err := g.Exchange(context.Background(), "auth-code")
	if err != nil {
		t.Fatal(err)
	}
	if tokens.AccessToken != "mock-access-token" {
		t.Errorf("expected mock-access-token, got %q", tokens.AccessToken)
	}

	user, err := g.UserInfo(context.Background(), tokens.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if user.Email != "user@gmail.com" {
		t.Errorf("expected user@gmail.com, got %q", user.Email)
	}
	if user.Subject != "12345" {
		t.Errorf("expected 12345, got %q", user.Subject)
	}
}

// mockGoogleProvider wraps Google to redirect to mock server URLs.
type mockGoogleProvider struct {
	*Google
	tokenURL    string
	userInfoURL string
}

func (m *mockGoogleProvider) Exchange(ctx context.Context, code string, opts ...oidc.ExchangeOption) (*oidc.TokenResult, error) {
	o := oidc.ApplyExchangeOptions(opts)
	tok, err := exchangeCode(ctx, m.tokenURL, m.cfg, code, o, nil)
	if err != nil {
		return nil, err
	}
	return toTokenResult(tok), nil
}

func (m *mockGoogleProvider) UserInfo(ctx context.Context, accessToken string) (*oidc.UserInfo, error) {
	var raw map[string]interface{}
	if err := fetchJSON(ctx, m.userInfoURL, accessToken, &raw); err != nil {
		return nil, err
	}
	return &oidc.UserInfo{
		Subject:       strVal(raw, "id"),
		Email:         strVal(raw, "email"),
		EmailVerified: boolVal(raw, "verified_email"),
		Name:          strVal(raw, "name"),
		Picture:       strVal(raw, "picture"),
		Raw:           raw,
	}, nil
}

func TestGitHubExchange(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token": "gh-token",
				"token_type":   "bearer",
				"scope":        "read:user user:email",
			})
			return
		}
		if r.URL.Path == "/user" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":         42,
				"login":      "octocat",
				"email":      "octocat@github.com",
				"name":       "Octocat",
				"avatar_url": "https://avatar.url",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	gh := NewGitHub(ProviderConfig{ClientID: "id", ClientSecret: "secret", RedirectURL: "http://test"})

	// Exchange using mock
	o := oidc.ExchangeOptions{}
	tok, err := exchangeCode(context.Background(), server.URL+"/token", gh.cfg, "code", o, map[string]string{"Accept": "application/json"})
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "gh-token" {
		t.Errorf("expected gh-token, got %q", tok.AccessToken)
	}

	// UserInfo using mock
	var raw map[string]interface{}
	if err := fetchJSON(context.Background(), server.URL+"/user", "gh-token", &raw); err != nil {
		t.Fatal(err)
	}
	if strVal(raw, "login") != "octocat" {
		t.Errorf("expected octocat, got %q", strVal(raw, "login"))
	}
}

func TestDefaultScopes(t *testing.T) {
	g := NewGoogle(ProviderConfig{ClientID: "id"})
	if len(g.cfg.Scopes) != 3 || g.cfg.Scopes[0] != "openid" {
		t.Errorf("expected default google scopes [openid email profile], got %v", g.cfg.Scopes)
	}

	gh := NewGitHub(ProviderConfig{ClientID: "id"})
	if len(gh.cfg.Scopes) != 2 || gh.cfg.Scopes[0] != "read:user" {
		t.Errorf("expected default github scopes [read:user user:email], got %v", gh.cfg.Scopes)
	}

	a := NewApple(AppleConfig{ProviderConfig: ProviderConfig{ClientID: "id"}})
	if len(a.cfg.Scopes) != 2 || a.cfg.Scopes[0] != "name" {
		t.Errorf("expected default apple scopes [name email], got %v", a.cfg.Scopes)
	}
}

func TestAuthURLWithPKCE(t *testing.T) {
	pkce, err := oidc.NewPKCE()
	if err != nil {
		t.Fatal(err)
	}

	g := NewGoogle(ProviderConfig{ClientID: "id", RedirectURL: "http://test"})
	url := g.AuthURL("state", oidc.WithPKCE(pkce))
	if !strings.Contains(url, "code_challenge=") {
		t.Error("expected code_challenge in URL")
	}
	if !strings.Contains(url, "code_challenge_method=S256") {
		t.Error("expected code_challenge_method=S256")
	}
}

func TestAuthURLWithOverrides(t *testing.T) {
	g := NewGoogle(ProviderConfig{
		ClientID:    "id",
		RedirectURL: "http://original",
		Scopes:      []string{"openid"},
	})

	url := g.AuthURL("state",
		oidc.WithRedirectURI("http://override"),
		oidc.WithScopes("openid", "email"),
		oidc.WithExtraParam("login_hint", "user@example.com"),
	)

	if !strings.Contains(url, "redirect_uri=http%3A%2F%2Foverride") {
		t.Error("expected overridden redirect_uri")
	}
	if !strings.Contains(url, "login_hint=user%40example.com") {
		t.Error("expected login_hint")
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
