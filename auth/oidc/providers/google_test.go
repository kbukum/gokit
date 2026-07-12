package providers

import (
	"strings"
	"testing"
)

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
