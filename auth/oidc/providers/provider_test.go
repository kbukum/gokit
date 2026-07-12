package providers

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/kbukum/gokit/auth/oidc"
	"github.com/kbukum/gokit/auth/oidc/testutil"
)

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

func TestPostExchangeHook(t *testing.T) {
	srv := testutil.NewMockOAuthServer()
	defer srv.Close()

	hookCalled := false
	p := NewGeneric(GenericConfig{
		ProviderConfig: mockProviderConfig(),
		ProviderName:   "ig-like",
		TokenEndpoint:  srv.TokenURL(),
		PostExchangeHook: func(_ context.Context, _ *http.Client, cfg ProviderConfig, token *oidc.TokenResult) (*oidc.TokenResult, error) {
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
		PostExchangeHook: func(_ context.Context, _ *http.Client, _ ProviderConfig, _ *oidc.TokenResult) (*oidc.TokenResult, error) {
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
