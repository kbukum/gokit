package testutil

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/kbukum/gokit/auth/oidc"
)

func TestMockProvider_BehaviorAndInspection(t *testing.T) {
	t.Parallel()

	mock := &MockProvider{
		ProviderName: "google",
		ExchangeResult: &oidc.TokenResult{
			AccessToken: "access",
		},
		UserInfoResult: &oidc.UserInfo{
			Subject: "user-1",
		},
		RefreshResult: &oidc.TokenResult{
			AccessToken: "refresh-access",
		},
	}

	if mock.Label() != "google" || mock.ProviderType() != "identity" {
		t.Fatalf("unexpected metadata: %q %q", mock.Label(), mock.ProviderType())
	}
	if mock.Name() != "google" {
		t.Fatalf("unexpected name: %q", mock.Name())
	}
	if got := mock.AuthURL("state-1"); got == "" {
		t.Fatal("expected auth URL")
	}
	if _, err := mock.Exchange(context.Background(), "code-1"); err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if _, err := mock.UserInfo(context.Background(), "access"); err != nil {
		t.Fatalf("UserInfo: %v", err)
	}
	if _, err := mock.Refresh(context.Background(), oidc.RefreshInput{RefreshToken: "rt"}); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	if len(mock.ExchangeCalls()) != 1 || len(mock.UserInfoCalls()) != 1 || len(mock.RefreshCalls()) != 1 || len(mock.AuthURLCalls()) != 1 {
		t.Fatal("expected calls to be recorded")
	}

	mock.Reset()
	if len(mock.ExchangeCalls()) != 0 || len(mock.AuthURLCalls()) != 0 {
		t.Fatal("expected Reset to clear recorded calls")
	}
}

func TestMockOAuthServer_Flows(t *testing.T) {
	t.Parallel()

	srv := NewMockOAuthServer()
	defer srv.Close()
	if srv.BaseURL() == "" || !strings.Contains(srv.AuthURL(), "/authorize") {
		t.Fatal("expected URL helpers to be populated")
	}

	srv.SetIDTokenClaims(map[string]any{
		"sub": "user-1",
	})
	srv.SetTokenResponse(map[string]any{"access_token": "custom-token"})
	srv.SetUserResponse(map[string]any{"sub": "override-user"})

	resp, err := http.PostForm(srv.TokenURL(), url.Values{
		"grant_type": {"authorization_code"},
		"code":       {"abc"},
	})
	if err != nil {
		t.Fatalf("token request: %v", err)
	}
	defer resp.Body.Close()

	var body map[string]any
	if decodeErr := json.NewDecoder(resp.Body).Decode(&body); decodeErr != nil {
		t.Fatalf("decode token response: %v", decodeErr)
	}
	if body["access_token"] != "custom-token" || body["id_token"] == "" {
		t.Fatalf("unexpected token response: %+v", body)
	}
	if len(srv.TokenRequests()) != 1 {
		t.Fatalf("expected recorded token requests")
	}
	if last := srv.LastTokenRequest(); last["code"] != "abc" {
		t.Fatalf("unexpected recorded request: %+v", last)
	}

	userResp, err := http.Get(srv.UserInfoURL())
	if err != nil {
		t.Fatalf("userinfo request: %v", err)
	}
	defer userResp.Body.Close()
	if userResp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected userinfo status: %d", userResp.StatusCode)
	}
	var userBody map[string]any
	if decodeErr := json.NewDecoder(userResp.Body).Decode(&userBody); decodeErr != nil {
		t.Fatalf("decode userinfo: %v", decodeErr)
	}
	if userBody["sub"] != "override-user" {
		t.Fatalf("unexpected userinfo body: %+v", userBody)
	}

	srv.FailUserInfo(true)
	userResp, err = http.Get(srv.UserInfoURL())
	if err != nil {
		t.Fatalf("failing userinfo request: %v", err)
	}
	userResp.Body.Close()
	if userResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized, got %d", userResp.StatusCode)
	}

	srv.FailToken(true)
	resp, err = http.PostForm(srv.TokenURL(), url.Values{"grant_type": {"authorization_code"}})
	if err != nil {
		t.Fatalf("failing token request: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected bad request, got %d", resp.StatusCode)
	}

	srv.Reset()
	if len(srv.TokenRequests()) != 0 || srv.LastTokenRequest() != nil {
		t.Fatal("expected Reset to clear token requests")
	}
}
