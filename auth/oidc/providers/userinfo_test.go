package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kbukum/gokit/auth/oidc"
	"github.com/kbukum/gokit/auth/oidc/testutil"
)

func TestUserInfo(t *testing.T) {
	srv := testutil.NewMockOAuthServer()
	defer srv.Close()

	p := NewGeneric(GenericConfig{
		ProviderConfig:   mockProviderConfig(),
		ProviderName:     "test",
		UserInfoEndpoint: srv.UserInfoURL(),
		UserInfo:         standardMapper(),
	})

	user, err := p.UserInfo(context.Background(), "test-token")
	if err != nil {
		t.Fatal(err)
	}
	if user.Subject != "user-123" {
		t.Errorf("Subject = %q, want 'user-123'", user.Subject)
	}
	if user.Email != "user@example.com" {
		t.Errorf("Email = %q, want 'user@example.com'", user.Email)
	}
	if !user.EmailVerified {
		t.Error("expected EmailVerified=true")
	}
	if user.Name != "Test User" {
		t.Errorf("Name = %q, want 'Test User'", user.Name)
	}
	if user.GivenName != "Test" {
		t.Errorf("GivenName = %q, want 'Test'", user.GivenName)
	}
	if user.FamilyName != "User" {
		t.Errorf("FamilyName = %q, want 'User'", user.FamilyName)
	}
	if user.Picture != "https://example.com/photo.jpg" {
		t.Errorf("Picture = %q, want 'https://example.com/photo.jpg'", user.Picture)
	}
	if user.Locale != "en" {
		t.Errorf("Locale = %q, want 'en'", user.Locale)
	}
}

func TestUserInfoError(t *testing.T) {
	srv := testutil.NewMockOAuthServer()
	defer srv.Close()
	srv.FailUserInfo(true)

	p := NewGeneric(GenericConfig{
		ProviderConfig:   mockProviderConfig(),
		ProviderName:     "fail-test",
		UserInfoEndpoint: srv.UserInfoURL(),
		UserInfo:         standardMapper(),
	})

	_, err := p.UserInfo(context.Background(), "bad-token")
	if err == nil {
		t.Fatal("expected error for failed userinfo")
	}
}

func TestUserInfoNoEndpoint(t *testing.T) {
	p := NewGeneric(GenericConfig{
		ProviderConfig: ProviderConfig{ClientID: "id"},
		ProviderName:   "no-endpoint",
	})

	_, err := p.UserInfo(context.Background(), "token")
	if err == nil {
		t.Fatal("expected error when no userinfo endpoint")
	}
}

// TestUserInfoNeverSendsTokenInQueryString locks in header-only bearer auth:
// the access token must never appear in the request URL (query or path), and a
// legacy "{access_token}" placeholder in the configured endpoint must be
// stripped rather than substituted. The token travels only in the
// Authorization header.
func TestUserInfoNeverSendsTokenInQueryString(t *testing.T) {
	var receivedURL, authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedURL = r.URL.String()
		authHeader = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "u1"})
	}))
	defer server.Close()

	p := NewGeneric(GenericConfig{
		ProviderConfig:   ProviderConfig{ClientID: "id", RedirectURL: "http://test"},
		ProviderName:     "placeholder-test",
		UserInfoEndpoint: server.URL + "/me?access_token={access_token}",
		UserInfo:         UserInfoMapper{SubjectKey: "id"},
	})

	if _, err := p.UserInfo(context.Background(), "my-tok-123"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(receivedURL, "my-tok-123") {
		t.Errorf("access token leaked into request URL: %s", receivedURL)
	}
	if strings.Contains(receivedURL, "%7Baccess_token%7D") || strings.Contains(receivedURL, "{access_token}") {
		t.Errorf("literal placeholder leaked into request URL: %s", receivedURL)
	}
	if authHeader != "Bearer my-tok-123" {
		t.Errorf("Authorization header = %q, want 'Bearer my-tok-123'", authHeader)
	}
}

func TestUserInfoIDTokenAsUserInfo(t *testing.T) {
	p := NewGeneric(GenericConfig{
		ProviderConfig:    ProviderConfig{ClientID: "id"},
		ProviderName:      "apple-like",
		IDTokenAsUserInfo: true,
	})

	_, err := p.UserInfo(context.Background(), "token")
	if err == nil {
		t.Fatal("expected error for IDTokenAsUserInfo provider")
	}
	if !strings.Contains(err.Error(), "use ID token claims") {
		t.Errorf("error = %q, should mention ID token", err)
	}
}

func TestPostUserInfoHook(t *testing.T) {
	srv := testutil.NewMockOAuthServer()
	defer srv.Close()

	hookCalled := false
	p := NewGeneric(GenericConfig{
		ProviderConfig:   mockProviderConfig(),
		ProviderName:     "hook-test",
		TokenEndpoint:    srv.TokenURL(),
		UserInfoEndpoint: srv.UserInfoURL(),
		UserInfo:         standardMapper(),
		PostUserInfoHook: func(_ context.Context, token string, info *oidc.UserInfo) error {
			hookCalled = true
			if token == "" {
				t.Error("hook received empty token")
			}
			info.Email = "enriched@test.com"
			return nil
		},
	})

	user, err := p.UserInfo(context.Background(), "tok")
	if err != nil {
		t.Fatal(err)
	}
	if !hookCalled {
		t.Error("PostUserInfoHook was not called")
	}
	if user.Email != "enriched@test.com" {
		t.Errorf("Email = %q, want 'enriched@test.com'", user.Email)
	}
}

func TestPostUserInfoHookErrorNonFatal(t *testing.T) {
	srv := testutil.NewMockOAuthServer()
	defer srv.Close()

	p := NewGeneric(GenericConfig{
		ProviderConfig:   mockProviderConfig(),
		ProviderName:     "hook-err",
		UserInfoEndpoint: srv.UserInfoURL(),
		UserInfo:         standardMapper(),
		PostUserInfoHook: func(_ context.Context, _ string, _ *oidc.UserInfo) error {
			return fmt.Errorf("secondary API failed")
		},
	})

	// Hook error is non-fatal — should still return base user info
	user, err := p.UserInfo(context.Background(), "tok")
	if err != nil {
		t.Fatal(err)
	}
	if user.Subject != "user-123" {
		t.Errorf("should still get base userinfo, got Subject=%q", user.Subject)
	}
}

func TestNestedMap(t *testing.T) {
	raw := map[string]any{
		"data": map[string]any{
			"user": map[string]any{
				"open_id":      "tk-123",
				"display_name": "TikToker",
			},
		},
	}

	nested := NestedMap(raw, "data.user")
	if nested == nil {
		t.Fatal("NestedMap returned nil")
	}
	if StrVal(nested, "open_id") != "tk-123" {
		t.Errorf("open_id = %q, want 'tk-123'", StrVal(nested, "open_id"))
	}
}

func TestNestedMapEmptyPath(t *testing.T) {
	raw := map[string]any{"key": "val"}
	result := NestedMap(raw, "")
	if result == nil || StrVal(result, "key") != "val" {
		t.Error("empty path should return the input map")
	}
}

func TestNestedMapMissing(t *testing.T) {
	raw := map[string]any{"other": "data"}
	if NestedMap(raw, "data.user") != nil {
		t.Error("expected nil for missing path")
	}
}

func TestNestedMapNonMapSegment(t *testing.T) {
	raw := map[string]any{"data": "not-a-map"}
	if NestedMap(raw, "data.user") != nil {
		t.Error("expected nil when path segment is not a map")
	}
}

func TestResponsePath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"user": map[string]any{
					"open_id":      "tk-456",
					"display_name": "TikTok User",
					"avatar_url":   "https://photo.tiktok.com/pic",
				},
			},
		})
	}))
	defer server.Close()

	p := NewGeneric(GenericConfig{
		ProviderConfig:   ProviderConfig{ClientID: "id", RedirectURL: "http://test"},
		ProviderName:     "nested-test",
		UserInfoEndpoint: server.URL + "/userinfo",
		UserInfo: UserInfoMapper{
			SubjectKey:   "open_id",
			NameKey:      "display_name",
			PictureKey:   "avatar_url",
			ResponsePath: "data.user",
		},
	})

	user, err := p.UserInfo(context.Background(), "tok")
	if err != nil {
		t.Fatal(err)
	}
	if user.Subject != "tk-456" {
		t.Errorf("Subject = %q, want 'tk-456'", user.Subject)
	}
	if user.Name != "TikTok User" {
		t.Errorf("Name = %q, want 'TikTok User'", user.Name)
	}
}

func TestNumericSubjectFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":   42, // numeric, not string
			"name": "Octocat",
		})
	}))
	defer server.Close()

	p := NewGeneric(GenericConfig{
		ProviderConfig:   ProviderConfig{ClientID: "id", RedirectURL: "http://test"},
		ProviderName:     "num-id",
		UserInfoEndpoint: server.URL + "/user",
		UserInfo:         UserInfoMapper{SubjectKey: "id", NameKey: "name"},
	})

	user, err := p.UserInfo(context.Background(), "tok")
	if err != nil {
		t.Fatal(err)
	}
	if user.Subject != "42" {
		t.Errorf("Subject = %q, want '42' (converted from numeric)", user.Subject)
	}
}

func TestStrVal(t *testing.T) {
	m := map[string]any{"name": "Alice", "count": 42}
	if StrVal(m, "name") != "Alice" {
		t.Error("expected 'Alice'")
	}
	if StrVal(m, "count") != "" {
		t.Error("expected '' for non-string value")
	}
	if StrVal(m, "missing") != "" {
		t.Error("expected '' for missing key")
	}
}

func TestBoolVal(t *testing.T) {
	m := map[string]any{"verified": true, "active": false, "name": "test"}
	if !BoolVal(m, "verified") {
		t.Error("expected true")
	}
	if BoolVal(m, "active") {
		t.Error("expected false")
	}
	if BoolVal(m, "name") {
		t.Error("expected false for non-bool")
	}
	if BoolVal(m, "missing") {
		t.Error("expected false for missing key")
	}
}

func TestFetchJSON(t *testing.T) {
	srv := testutil.NewMockOAuthServer()
	defer srv.Close()

	var raw map[string]any
	err := FetchJSON(context.Background(), nil, srv.UserInfoURL(), "test-token", &raw)
	if err != nil {
		t.Fatal(err)
	}
	if StrVal(raw, "email") != "user@example.com" {
		t.Errorf("email = %q", StrVal(raw, "email"))
	}
}

func TestCommaSeparatedScopesInResponse(t *testing.T) {
	srv := testutil.NewMockOAuthServer()
	defer srv.Close()
	srv.SetTokenResponse(map[string]any{
		"access_token": "tok",
		"token_type":   "Bearer",
		"scope":        "user.info.basic,video.list",
	})

	p := NewGeneric(GenericConfig{
		ProviderConfig:     mockProviderConfig(),
		ProviderName:       "tiktok-like",
		TokenEndpoint:      srv.TokenURL(),
		TokenRequestFormat: "json",
		ScopeSeparator:     ",",
	})

	tokens, err := p.Exchange(context.Background(), "code")
	if err != nil {
		t.Fatal(err)
	}
	if len(tokens.Scopes) != 2 {
		t.Fatalf("Scopes = %v, want 2 items", tokens.Scopes)
	}
	if tokens.Scopes[0] != "user.info.basic" || tokens.Scopes[1] != "video.list" {
		t.Errorf("Scopes = %v, want [user.info.basic video.list]", tokens.Scopes)
	}
}

func TestFetchJSON_Errors(t *testing.T) {
	ctx := context.Background()
	var out map[string]any
	if err := FetchJSON(ctx, nil, "http://\x7f/bad", "tok", &out); err == nil {
		t.Error("expected request-construction error")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()
	if err := FetchJSON(ctx, nil, url, "tok", &out); err == nil {
		t.Error("expected connection error")
	}

	fail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no", http.StatusUnauthorized)
	}))
	defer fail.Close()
	if err := FetchJSON(ctx, nil, fail.URL, "tok", &out); err == nil {
		t.Error("expected HTTP status error")
	}
}
