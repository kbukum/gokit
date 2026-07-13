package providers

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
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

func TestNewApple_WithPrivateKeySetsSecretFunc(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey: %v", err)
	}
	pemKey := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))

	p := NewApple(AppleConfig{
		ProviderConfig: ProviderConfig{ClientID: "cid", RedirectURL: "http://x"},
		TeamID:         "team",
		KeyID:          "kid",
		PrivateKey:     pemKey,
	})
	if p.Name() != "apple" {
		t.Errorf("Name = %q", p.Name())
	}
}

func TestNewApple_WithoutPrivateKey(t *testing.T) {
	p := NewApple(AppleConfig{ProviderConfig: ProviderConfig{ClientID: "cid"}})
	if p.Name() != "apple" {
		t.Errorf("Name = %q", p.Name())
	}
}
