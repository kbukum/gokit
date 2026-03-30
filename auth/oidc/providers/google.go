package providers

import (
	"context"
	"fmt"

	"github.com/kbukum/gokit/auth/oidc"
)

const (
	googleAuthURL     = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL    = "https://oauth2.googleapis.com/token"
	googleUserInfoURL = "https://www.googleapis.com/oauth2/v2/userinfo"
)

// Google implements the oidc.Provider interface for Google OAuth2/OIDC.
type Google struct {
	cfg ProviderConfig
}

// NewGoogle creates a new Google provider.
// Default scopes: openid, email, profile.
func NewGoogle(cfg ProviderConfig) *Google {
	if len(cfg.Scopes) == 0 {
		cfg.Scopes = []string{"openid", "email", "profile"}
	}
	return &Google{cfg: cfg}
}

func (g *Google) Name() string { return "google" }

func (g *Google) AuthURL(state string, opts ...oidc.AuthURLOption) string {
	o := oidc.ApplyAuthURLOptions(opts)
	return buildAuthURL(googleAuthURL, g.cfg, state, o, map[string]string{
		"access_type": "offline",
		"prompt":      "consent",
	})
}

func (g *Google) Exchange(ctx context.Context, code string, opts ...oidc.ExchangeOption) (*oidc.TokenResult, error) {
	o := oidc.ApplyExchangeOptions(opts)
	tok, err := exchangeCode(ctx, googleTokenURL, g.cfg, code, o, nil)
	if err != nil {
		return nil, fmt.Errorf("google: %w", err)
	}
	return toTokenResult(tok), nil
}

func (g *Google) UserInfo(ctx context.Context, accessToken string) (*oidc.UserInfo, error) {
	var raw map[string]interface{}
	if err := fetchJSON(ctx, googleUserInfoURL, accessToken, &raw); err != nil {
		return nil, fmt.Errorf("google userinfo: %w", err)
	}

	return &oidc.UserInfo{
		Subject:       strVal(raw, "id"),
		Email:         strVal(raw, "email"),
		EmailVerified: boolVal(raw, "verified_email"),
		Name:          strVal(raw, "name"),
		GivenName:     strVal(raw, "given_name"),
		FamilyName:    strVal(raw, "family_name"),
		Picture:       strVal(raw, "picture"),
		Locale:        strVal(raw, "locale"),
		Raw:           raw,
	}, nil
}

// Compile-time check that Google implements Provider.
var _ oidc.Provider = (*Google)(nil)

func strVal(m map[string]interface{}, key string) string {
	v, _ := m[key].(string)
	return v
}

func boolVal(m map[string]interface{}, key string) bool {
	v, _ := m[key].(bool)
	return v
}
