package providers

import (
	"context"
	"strings"
	"testing"

	"github.com/kbukum/gokit/auth/oidc"
	"github.com/kbukum/gokit/auth/oidc/testutil"
)

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
	srv.SetIDTokenClaims(map[string]any{
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

func TestParseIDTokenClaims(t *testing.T) {
	idToken := testutil.BuildTestIDToken(map[string]any{
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
	idToken := testutil.BuildTestIDToken(map[string]any{
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
