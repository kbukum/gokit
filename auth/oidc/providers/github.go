package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/kbukum/gokit/auth/oidc"
)

// GitHub OAuth2 endpoint defaults.
const (
	GitHubAuthEndpoint     = "https://github.com/login/oauth/authorize"
	GitHubTokenEndpoint    = "https://github.com/login/oauth/access_token" //nolint:gosec // OAuth endpoint URL, not a credential
	GitHubUserInfoEndpoint = "https://api.github.com/user"
	GitHubEmailsEndpoint   = "https://api.github.com/user/emails"
)

// GitHubDefaultScopes are the standard scopes for GitHub login.
var GitHubDefaultScopes = []string{"read:user", "user:email"}

// NewGitHub creates a GitHub OAuth2 provider.
// Note: GitHub uses plain OAuth2, not OIDC — there is no ID token.
// All fields have sensible defaults; override any by setting them in cfg.
//
// For GitHub Enterprise, use NewGeneric() with custom endpoints:
//
//	providers.NewGeneric(providers.GenericConfig{
//	    AuthEndpoint: "https://github.example.com/login/oauth/authorize",
//	    ...
//	})
func NewGitHub(cfg ProviderConfig) *GenericProvider {
	return NewGeneric(GenericConfig{
		ProviderConfig:   withDefaultScopes(cfg, GitHubDefaultScopes...),
		ProviderName:     "github",
		Label:            "GitHub",
		Type:             "identity",
		AuthEndpoint:     GitHubAuthEndpoint,
		TokenEndpoint:    GitHubTokenEndpoint,
		UserInfoEndpoint: GitHubUserInfoEndpoint,
		TokenExtraHeaders: map[string]string{
			"Accept": "application/json",
		},
		UserInfo: UserInfoMapper{
			SubjectKey: "id",
			EmailKey:   "email",
			NameKey:    "name",
			PictureKey: "avatar_url",
		},
		PostUserInfoHook: newGitHubEmailFallback(GitHubEmailsEndpoint),
	})
}

// newGitHubEmailFallback returns a hook that fetches the user's primary email
// from the GitHub /user/emails endpoint when it's not public on /user.
// The endpoint URL is parameterized to support GitHub Enterprise.
func newGitHubEmailFallback(emailsEndpoint string) func(ctx context.Context, accessToken string, info *oidc.UserInfo) error {
	return func(ctx context.Context, accessToken string, info *oidc.UserInfo) error {
		if info.Email != "" {
			info.EmailVerified = true
			return nil
		}

		email, verified, err := fetchGitHubPrimaryEmail(ctx, emailsEndpoint, accessToken)
		if err != nil {
			return nil // non-fatal — return what we have
		}
		info.Email = email
		info.EmailVerified = verified
		return nil
	}
}

// fetchGitHubPrimaryEmail fetches the user's primary email from a GitHub emails API endpoint.
func fetchGitHubPrimaryEmail(ctx context.Context, endpoint, accessToken string) (email string, verified bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		return "", false, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := DefaultHTTPClient.Do(req)
	if err != nil {
		return "", false, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", false, err
	}
	if resp.StatusCode != http.StatusOK {
		return "", false, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := json.Unmarshal(body, &emails); err != nil {
		return "", false, err
	}

	for _, e := range emails {
		if e.Primary {
			return e.Email, e.Verified, nil
		}
	}
	if len(emails) > 0 {
		return emails[0].Email, emails[0].Verified, nil
	}
	return "", false, nil
}
