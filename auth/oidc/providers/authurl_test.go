package providers

import (
	"strings"
	"testing"

	"github.com/kbukum/gokit/auth/oidc"
)

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
