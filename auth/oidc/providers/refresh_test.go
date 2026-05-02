package providers

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kbukum/gokit/auth/oidc"
	oidctestutil "github.com/kbukum/gokit/auth/oidc/testutil"
)

func TestGenericProvider_Refresh_DefaultGrant(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if r.FormValue("refresh_token") != "refresh-token" {
			t.Fatalf("unexpected refresh token: %q", r.FormValue("refresh_token"))
		}
		_, _ = w.Write([]byte(`{"access_token":"new-access","refresh_token":"new-refresh"}`))
	}))
	defer srv.Close()

	p := NewGeneric(GenericConfig{
		ProviderConfig: ProviderConfig{
			ClientID:     "client-id",
			ClientSecret: "client-secret",
		},
		ProviderName:  "test",
		TokenEndpoint: srv.URL,
	})

	result, err := p.Refresh(context.Background(), oidc.RefreshInput{RefreshToken: "refresh-token"})
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if result.AccessToken != "new-access" || result.RefreshToken != "new-refresh" {
		t.Fatalf("unexpected refresh result: %+v", result)
	}
}

func TestManager_Refresh(t *testing.T) {
	t.Parallel()

	provider := &oidctestutil.MockProvider{
		ProviderName: "mock",
		RefreshResult: &oidc.TokenResult{
			AccessToken: "rotated",
		},
	}
	manager := NewManager(provider)
	result, err := manager.Refresh(context.Background(), "mock", oidc.RefreshInput{RefreshToken: "rt"})
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if result.AccessToken != "rotated" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestNewAppleSecretFunc_InvalidAndValidPEM(t *testing.T) {
	t.Parallel()

	invalid := newAppleSecretFunc(AppleConfig{
		ProviderConfig: ProviderConfig{ClientID: "client-id"},
		TeamID:         "team-id",
		KeyID:          "key-id",
		PrivateKey:     "not-a-pem",
	})
	if _, err := invalid(); err == nil {
		t.Fatal("expected invalid PEM to fail")
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey: %v", err)
	}
	valid := newAppleSecretFunc(AppleConfig{
		ProviderConfig: ProviderConfig{ClientID: "client-id"},
		TeamID:         "team-id",
		KeyID:          "key-id",
		PrivateKey: string(pem.EncodeToMemory(&pem.Block{
			Type:  "PRIVATE KEY",
			Bytes: der,
		})),
	})
	secret, err := valid()
	if err != nil {
		t.Fatalf("expected valid PEM to succeed: %v", err)
	}
	if secret == "" {
		t.Fatal("expected generated secret")
	}
}
