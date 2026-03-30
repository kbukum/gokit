package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/kbukum/gokit/auth/oidc"
)

const (
	githubAuthURL     = "https://github.com/login/oauth/authorize"
	githubTokenURL    = "https://github.com/login/oauth/access_token"
	githubUserInfoURL = "https://api.github.com/user"
	githubEmailsURL   = "https://api.github.com/user/emails"
)

// GitHub implements the oidc.Provider interface for GitHub OAuth2.
// Note: GitHub uses plain OAuth2, not OIDC — there is no ID token.
type GitHub struct {
	cfg ProviderConfig
}

// NewGitHub creates a new GitHub provider.
// Default scopes: read:user, user:email.
func NewGitHub(cfg ProviderConfig) *GitHub {
	if len(cfg.Scopes) == 0 {
		cfg.Scopes = []string{"read:user", "user:email"}
	}
	return &GitHub{cfg: cfg}
}

func (g *GitHub) Name() string { return "github" }

func (g *GitHub) AuthURL(state string, opts ...oidc.AuthURLOption) string {
	o := oidc.ApplyAuthURLOptions(opts)
	return buildAuthURL(githubAuthURL, g.cfg, state, o, nil)
}

func (g *GitHub) Exchange(ctx context.Context, code string, opts ...oidc.ExchangeOption) (*oidc.TokenResult, error) {
	o := oidc.ApplyExchangeOptions(opts)
	tok, err := exchangeCode(ctx, githubTokenURL, g.cfg, code, o, map[string]string{
		"Accept": "application/json",
	})
	if err != nil {
		return nil, fmt.Errorf("github: %w", err)
	}
	return toTokenResult(tok), nil
}

func (g *GitHub) UserInfo(ctx context.Context, accessToken string) (*oidc.UserInfo, error) {
	var raw map[string]interface{}
	if err := fetchJSON(ctx, githubUserInfoURL, accessToken, &raw); err != nil {
		return nil, fmt.Errorf("github user: %w", err)
	}

	email := strVal(raw, "email")
	emailVerified := email != ""

	// If email is not public, fetch from /user/emails endpoint
	if email == "" {
		e, verified, err := g.fetchPrimaryEmail(ctx, accessToken)
		if err == nil && e != "" {
			email = e
			emailVerified = verified
		}
	}

	// GitHub uses numeric IDs — convert to string
	subject := strVal(raw, "login")
	if id, ok := raw["id"]; ok {
		subject = fmt.Sprintf("%v", id)
	}

	return &oidc.UserInfo{
		Subject:       subject,
		Email:         email,
		EmailVerified: emailVerified,
		Name:          strVal(raw, "name"),
		Picture:       strVal(raw, "avatar_url"),
		Raw:           raw,
	}, nil
}

// fetchPrimaryEmail fetches the user's primary email from the GitHub emails API.
func (g *GitHub) fetchPrimaryEmail(ctx context.Context, accessToken string) (string, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubEmailsURL, nil)
	if err != nil {
		return "", false, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()

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

var _ oidc.Provider = (*GitHub)(nil)
