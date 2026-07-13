package oidc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestRefreshToken_JSONResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
			t.Errorf("expected form content type, got %s", ct)
		}

		if err := r.ParseForm(); err != nil {
			t.Fatalf("parsing form: %v", err)
		}
		if r.FormValue("grant_type") != "refresh_token" {
			t.Errorf("expected grant_type=refresh_token, got %s", r.FormValue("grant_type"))
		}
		if r.FormValue("client_id") != "my-client" {
			t.Errorf("expected client_id=my-client, got %s", r.FormValue("client_id"))
		}
		if r.FormValue("client_secret") != "my-secret" {
			t.Errorf("expected client_secret=my-secret, got %s", r.FormValue("client_secret"))
		}
		if r.FormValue("refresh_token") != "old-refresh-token" {
			t.Errorf("expected refresh_token=old-refresh-token, got %s", r.FormValue("refresh_token"))
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "new-access-token",
			"token_type":    "Bearer",
			"expires_in":    3600,
			"refresh_token": "new-refresh-token",
			"scope":         "openid email",
		})
	}))
	defer srv.Close()

	result, err := RefreshToken(context.Background(), RefreshConfig{
		TokenEndpoint: srv.URL,
		ClientID:      "my-client",
		ClientSecret:  "my-secret",
		RefreshToken:  "old-refresh-token",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.AccessToken != "new-access-token" {
		t.Errorf("expected new-access-token, got %s", result.AccessToken)
	}
	if result.RefreshToken != "new-refresh-token" {
		t.Errorf("expected new-refresh-token, got %s", result.RefreshToken)
	}
	if result.TokenType != "Bearer" {
		t.Errorf("expected Bearer, got %s", result.TokenType)
	}
	if result.ExpiresAt.Before(time.Now()) {
		t.Error("expected ExpiresAt to be in the future")
	}
	if len(result.Scopes) != 2 || result.Scopes[0] != "openid" || result.Scopes[1] != "email" {
		t.Errorf("expected [openid email], got %v", result.Scopes)
	}
}

func TestRefreshToken_FormEncodedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-www-form-urlencoded")
		values := url.Values{
			"access_token": {"form-access-token"},
			"token_type":   {"Bearer"},
			"expires_in":   {"7200"},
		}
		_, _ = w.Write([]byte(values.Encode()))
	}))
	defer srv.Close()

	result, err := RefreshToken(context.Background(), RefreshConfig{
		TokenEndpoint: srv.URL,
		ClientID:      "client",
		RefreshToken:  "rt",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.AccessToken != "form-access-token" {
		t.Errorf("expected form-access-token, got %s", result.AccessToken)
	}
	if result.TokenType != "Bearer" {
		t.Errorf("expected Bearer, got %s", result.TokenType)
	}
}

func TestRefreshToken_DefaultsTokenType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at",
			"expires_in":   3600,
		})
	}))
	defer srv.Close()

	result, err := RefreshToken(context.Background(), RefreshConfig{
		TokenEndpoint: srv.URL,
		ClientID:      "c",
		RefreshToken:  "rt",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.TokenType != "Bearer" {
		t.Errorf("expected default Bearer, got %s", result.TokenType)
	}
}

func TestRefreshToken_SendsScopes(t *testing.T) {
	var receivedScope string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		receivedScope = r.FormValue("scope")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at",
		})
	}))
	defer srv.Close()

	_, err := RefreshToken(context.Background(), RefreshConfig{
		TokenEndpoint: srv.URL,
		ClientID:      "c",
		RefreshToken:  "rt",
		Scopes:        []string{"openid", "profile"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedScope != "openid profile" {
		t.Errorf("expected 'openid profile', got %q", receivedScope)
	}
}

func TestRefreshToken_SendsExtraParams(t *testing.T) {
	var receivedAudience string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		receivedAudience = r.FormValue("audience")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at",
		})
	}))
	defer srv.Close()

	_, err := RefreshToken(context.Background(), RefreshConfig{
		TokenEndpoint: srv.URL,
		ClientID:      "c",
		RefreshToken:  "rt",
		ExtraParams:   map[string]string{"audience": "https://api.example.com"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedAudience != "https://api.example.com" {
		t.Errorf("expected audience param, got %q", receivedAudience)
	}
}

func TestRefreshToken_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer srv.Close()

	_, err := RefreshToken(context.Background(), RefreshConfig{
		TokenEndpoint: srv.URL,
		ClientID:      "c",
		RefreshToken:  "rt",
	})
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
	if !strings.Contains(err.Error(), "status 400") {
		t.Errorf("expected status 400 in error, got: %v", err)
	}
}

func TestRefreshToken_MissingEndpoint(t *testing.T) {
	_, err := RefreshToken(context.Background(), RefreshConfig{
		RefreshToken: "rt",
	})
	if err == nil {
		t.Fatal("expected error for missing endpoint")
	}
}

func TestRefreshToken_MissingRefreshToken(t *testing.T) {
	_, err := RefreshToken(context.Background(), RefreshConfig{
		TokenEndpoint: "https://example.com/token",
	})
	if err == nil {
		t.Fatal("expected error for missing refresh token")
	}
}

func TestRefreshToken_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"at"}`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := RefreshToken(ctx, RefreshConfig{
		TokenEndpoint: srv.URL,
		ClientID:      "c",
		RefreshToken:  "rt",
	})
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
}

func TestRefreshToken_TokenRotation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "rotated-access",
			"refresh_token": "rotated-refresh",
			"id_token":      "rotated-id",
		})
	}))
	defer srv.Close()

	result, err := RefreshToken(context.Background(), RefreshConfig{
		TokenEndpoint: srv.URL,
		ClientID:      "c",
		RefreshToken:  "old-rt",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.AccessToken != "rotated-access" {
		t.Errorf("expected rotated-access, got %s", result.AccessToken)
	}
	if result.RefreshToken != "rotated-refresh" {
		t.Errorf("expected rotated-refresh, got %s", result.RefreshToken)
	}
	if result.IDToken != "rotated-id" {
		t.Errorf("expected rotated-id, got %s", result.IDToken)
	}
}

func TestParseTokenResponse_InvalidBody(t *testing.T) {
	if _, err := parseTokenResponse([]byte("%zz")); err == nil {
		t.Fatal("expected parse error for malformed body")
	}
	if _, err := parseTokenResponse([]byte("token_type=bearer")); err == nil {
		t.Fatal("expected missing access_token error")
	}
}

func TestRefreshToken_BadEndpoint(t *testing.T) {
	_, err := RefreshToken(context.Background(), RefreshConfig{
		TokenEndpoint: "http://\x7f/bad",
		RefreshToken:  "rt",
	})
	if err == nil {
		t.Fatal("expected request-construction error")
	}
}
