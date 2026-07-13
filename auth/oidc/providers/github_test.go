package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kbukum/gokit/auth/oidc"
	"github.com/kbukum/gokit/auth/oidc/testutil"
)

func TestGitHubConstructor(t *testing.T) {
	g := NewGitHub(ProviderConfig{ClientID: "id"})

	if g.Name() != "github" {
		t.Errorf("Name() = %q, want 'github'", g.Name())
	}
	if g.Label() != "GitHub" {
		t.Errorf("Label() = %q, want 'GitHub'", g.Label())
	}
	if g.ProviderType() != "identity" {
		t.Errorf("ProviderType() = %q, want 'identity'", g.ProviderType())
	}
}

func TestGitHubAuthURL(t *testing.T) {
	g := NewGitHub(ProviderConfig{
		ClientID:    "gh-id",
		RedirectURL: "http://localhost/callback",
		Scopes:      []string{"read:user", "user:email"},
	})
	u := g.AuthURL("my-state")

	if !strings.Contains(u, "github.com/login/oauth/authorize") {
		t.Error("expected github auth URL")
	}
	if !strings.Contains(u, "client_id=gh-id") {
		t.Error("expected client_id")
	}
}

func TestGitHubEmailFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/token":
			json.NewEncoder(w).Encode(map[string]any{
				"access_token": "gh-tok",
				"token_type":   "bearer",
			})
		case "/user":
			json.NewEncoder(w).Encode(map[string]any{
				"id":         42,
				"name":       "Octocat",
				"email":      nil, // email is private
				"avatar_url": "https://avatar.url",
			})
		case "/user/emails":
			json.NewEncoder(w).Encode([]map[string]any{
				{"email": "secondary@gh.com", "primary": false, "verified": true},
				{"email": "primary@gh.com", "primary": true, "verified": true},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	p := NewGeneric(GenericConfig{
		ProviderConfig:   ProviderConfig{ClientID: "id", ClientSecret: "s", RedirectURL: "http://test"},
		ProviderName:     "github-mock",
		TokenEndpoint:    server.URL + "/token",
		UserInfoEndpoint: server.URL + "/user",
		TokenExtraHeaders: map[string]string{
			"Accept": "application/json",
		},
		UserInfo: UserInfoMapper{
			SubjectKey: "id",
			EmailKey:   "email",
			NameKey:    "name",
			PictureKey: "avatar_url",
		},
		PostUserInfoHook: newGitHubEmailFallback(server.URL + "/user/emails"),
	})

	tokens, err := p.Exchange(context.Background(), "code")
	if err != nil {
		t.Fatal(err)
	}

	user, err := p.UserInfo(context.Background(), tokens.AccessToken)
	if err != nil {
		t.Fatal(err)
	}

	if user.Subject != "42" {
		t.Errorf("Subject = %q, want '42'", user.Subject)
	}
	if user.Email != "primary@gh.com" {
		t.Errorf("Email = %q, want 'primary@gh.com' (from fallback)", user.Email)
	}
	if !user.EmailVerified {
		t.Error("expected EmailVerified=true")
	}
}

func TestGitHubEmailNotNeeded(t *testing.T) {
	srv := testutil.NewMockOAuthServer()
	defer srv.Close()

	emailEndpointCalled := false
	emailServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		emailEndpointCalled = true
	}))
	defer emailServer.Close()

	hook := newGitHubEmailFallback(emailServer.URL + "/user/emails")
	info := &oidc.UserInfo{Email: "already@here.com"}
	err := hook(context.Background(), "tok", info)
	if err != nil {
		t.Fatal(err)
	}
	if emailEndpointCalled {
		t.Error("should not call email endpoint when email is already present")
	}
	if !info.EmailVerified {
		t.Error("should set EmailVerified=true when email present")
	}
}

func TestFetchGitHubPrimaryEmail_Paths(t *testing.T) {
	ctx := context.Background()

	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"email": "a@x.com", "primary": false, "verified": true},
			{"email": "b@x.com", "primary": true, "verified": true},
		})
	}))
	defer primary.Close()
	email, verified, err := fetchGitHubPrimaryEmail(ctx, primary.URL, "tok")
	if err != nil || email != "b@x.com" || !verified {
		t.Fatalf("primary: %q %v %v", email, verified, err)
	}

	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"email": "only@x.com", "primary": false, "verified": false},
		})
	}))
	defer first.Close()
	email, _, err = fetchGitHubPrimaryEmail(ctx, first.URL, "tok")
	if err != nil || email != "only@x.com" {
		t.Fatalf("first-fallback: %q %v", email, err)
	}

	empty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{})
	}))
	defer empty.Close()
	if email, _, err = fetchGitHubPrimaryEmail(ctx, empty.URL, "tok"); err != nil || email != "" {
		t.Fatalf("empty: %q %v", email, err)
	}

	if _, _, err = fetchGitHubPrimaryEmail(ctx, "http://\x7f/bad", "tok"); err == nil {
		t.Error("expected request-construction error")
	}

	fail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no", http.StatusForbidden)
	}))
	defer fail.Close()
	if _, _, err = fetchGitHubPrimaryEmail(ctx, fail.URL, "tok"); err == nil {
		t.Error("expected HTTP status error")
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer bad.Close()
	if _, _, err = fetchGitHubPrimaryEmail(ctx, bad.URL, "tok"); err == nil {
		t.Error("expected decode error")
	}
}
