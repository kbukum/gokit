package providers

import (
	"strings"
	"testing"
)

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
