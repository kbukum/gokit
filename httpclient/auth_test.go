package httpclient

import (
	"net/http"
	"testing"
)

func TestBearerAuth(t *testing.T) {
	auth := BearerAuth("my-token")
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	auth.apply(req)
	if got := req.Header.Get("Authorization"); got != "Bearer my-token" {
		t.Errorf("got %q, want %q", got, "Bearer my-token")
	}
}

func TestBasicAuth(t *testing.T) {
	auth := BasicAuth("user", "pass")
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	auth.apply(req)
	u, p, ok := req.BasicAuth()
	if !ok || u != "user" || p != "pass" {
		t.Errorf("basic auth not set correctly: user=%q pass=%q ok=%v", u, p, ok)
	}
}

func TestAPIKeyAuth_Header(t *testing.T) {
	auth := APIKeyAuth("secret-key")
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	auth.apply(req)
	if got := req.Header.Get("X-API-Key"); got != "secret-key" {
		t.Errorf("got %q, want %q", got, "secret-key")
	}
}

func TestAPIKeyAuthHeader_CustomName(t *testing.T) {
	auth := APIKeyAuthHeader("secret-key", "X-Custom-Key")
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	auth.apply(req)
	if got := req.Header.Get("X-Custom-Key"); got != "secret-key" {
		t.Errorf("got %q, want %q", got, "secret-key")
	}
}

func TestAPIKeyAuthQuery(t *testing.T) {
	auth := APIKeyAuthQuery("secret-key", "api_key")
	req, _ := http.NewRequest("GET", "http://example.com/path", nil)
	auth.apply(req)
	if got := req.URL.Query().Get("api_key"); got != "secret-key" {
		t.Errorf("got %q, want %q", got, "secret-key")
	}
}

func TestCustomAuth(t *testing.T) {
	auth := CustomAuth(func(req *http.Request) {
		req.Header.Set("X-Custom", "value")
	})
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	auth.apply(req)
	if got := req.Header.Get("X-Custom"); got != "value" {
		t.Errorf("got %q, want %q", got, "value")
	}
}

func TestNilAuth(t *testing.T) {
	var auth *AuthConfig
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	auth.apply(req) // should not panic
}

func TestAuthNone(t *testing.T) {
	auth := &AuthConfig{Type: AuthNone}
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	auth.apply(req) // should not modify request
	if req.Header.Get("Authorization") != "" {
		t.Error("AuthNone should not set Authorization header")
	}
}
