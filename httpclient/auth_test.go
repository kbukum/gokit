package httpclient

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestBearerAuth(t *testing.T) {
	auth := BearerAuth("my-token")
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://example.com", http.NoBody)
	auth.apply(req)
	if got := req.Header.Get("Authorization"); got != "Bearer my-token" {
		t.Errorf("got %q, want %q", got, "Bearer my-token")
	}
}

func TestBasicAuth(t *testing.T) {
	auth := BasicAuth("user", "pass")
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://example.com", http.NoBody)
	auth.apply(req)
	u, p, ok := req.BasicAuth()
	if !ok || u != "user" || p != "pass" {
		t.Errorf("basic auth not set correctly: user=%q pass=%q ok=%v", u, p, ok)
	}
}

func TestAPIKeyAuth_Header(t *testing.T) {
	auth := APIKeyAuth("secret-key")
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://example.com", http.NoBody)
	auth.apply(req)
	if got := req.Header.Get("X-API-Key"); got != "secret-key" {
		t.Errorf("got %q, want %q", got, "secret-key")
	}
}

func TestAPIKeyAuthHeader_CustomName(t *testing.T) {
	auth := APIKeyAuthHeader("secret-key", "X-Custom-Key")
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://example.com", http.NoBody)
	auth.apply(req)
	if got := req.Header.Get("X-Custom-Key"); got != "secret-key" {
		t.Errorf("got %q, want %q", got, "secret-key")
	}
}

func TestAPIKeyAuth_UsesHeaderNotQuery(t *testing.T) {
	auth := APIKeyAuth("secret-key")
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://example.com/path", http.NoBody)
	auth.apply(req)
	if got := req.Header.Get("X-API-Key"); got != "secret-key" {
		t.Errorf("header X-API-Key = %q, want %q", got, "secret-key")
	}
	if req.URL.RawQuery != "" {
		t.Errorf("credentials must never be placed in the query string, got %q", req.URL.RawQuery)
	}
}

func TestAuthConfig_RedactsSecrets(t *testing.T) {
	for _, a := range []*AuthConfig{
		BearerAuth("super-secret-token"),
		BasicAuth("alice", "hunter2"),
		APIKeyAuth("api-secret"),
	} {
		s := a.String()
		for _, secret := range []string{"super-secret-token", "hunter2", "api-secret"} {
			if strings.Contains(s, secret) {
				t.Errorf("String() leaked secret %q: %s", secret, s)
			}
		}
		if g := fmt.Sprintf("%#v", a); strings.Contains(g, "hunter2") ||
			strings.Contains(g, "super-secret-token") || strings.Contains(g, "api-secret") {
			t.Errorf("%%#v leaked a secret: %s", g)
		}
	}
	var nilAuth *AuthConfig
	if nilAuth.String() != "AuthConfig(none)" {
		t.Errorf("nil String() = %q", nilAuth.String())
	}
}

func TestCustomAuth(t *testing.T) {
	auth := CustomAuth(func(req *http.Request) {
		req.Header.Set("X-Custom", "value")
	})
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://example.com", http.NoBody)
	auth.apply(req)
	if got := req.Header.Get("X-Custom"); got != "value" {
		t.Errorf("got %q, want %q", got, "value")
	}
}

func TestNilAuth(t *testing.T) {
	var auth *AuthConfig
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://example.com", http.NoBody)
	auth.apply(req) // should not panic
}

func TestAuthNone(t *testing.T) {
	auth := &AuthConfig{Type: AuthNone}
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://example.com", http.NoBody)
	auth.apply(req) // should not modify request
	if req.Header.Get("Authorization") != "" {
		t.Error("AuthNone should not set Authorization header")
	}
}
