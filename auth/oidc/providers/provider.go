package providers

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/kbukum/gokit/auth/oidc"
)

// GenericProvider implements oidc.Provider and oidc.ProviderMeta for any OAuth2/OIDC provider using configuration only. This is THE single provider implementation — all built-in providers (Google, GitHub, Apple) and custom providers (YouTube, TikTok, Instagram) are constructed via NewGeneric.
//
// Adding a new provider is typically a single function that returns NewGeneric(GenericConfig{...}) with the right endpoints, field mappings, and optional hooks for provider-specific quirks.
type GenericProvider struct {
	cfg GenericConfig
}

// NewGeneric creates a provider from configuration.
func NewGeneric(cfg GenericConfig) *GenericProvider {
	if cfg.Label == "" {
		cfg.Label = cfg.ProviderName
	}
	if cfg.Type == "" {
		cfg.Type = "identity"
	}
	return &GenericProvider{cfg: cfg}
}

// --- oidc.Provider implementation ---

func (p *GenericProvider) Name() string { return p.cfg.ProviderName }

func (p *GenericProvider) AuthURL(state string, opts ...oidc.AuthURLOption) string {
	o := oidc.ApplyAuthURLOptions(opts)
	return BuildAuthURL(
		p.cfg.AuthEndpoint,
		p.cfg.ProviderConfig,
		state, o,
		p.cfg.AuthExtraParams,
		p.cfg.ClientIDParam,
		p.cfg.ScopeSeparator,
	)
}

func (p *GenericProvider) Exchange(ctx context.Context, code string, opts ...oidc.ExchangeOption) (*oidc.TokenResult, error) {
	o := oidc.ApplyExchangeOptions(opts)

	// Override client secret if dynamic generation is configured (e.g., Apple JWT)
	cfg := p.cfg.ProviderConfig
	if p.cfg.ClientSecretFunc != nil {
		secret, err := p.cfg.ClientSecretFunc()
		if err != nil {
			return nil, fmt.Errorf("%s: generate client secret: %w", p.cfg.ProviderName, err)
		}
		cfg.ClientSecret = secret
	}

	var result *oidc.TokenResult

	if p.cfg.TokenRequestFormat == "json" {
		tok, err := ExchangeJSON(ctx, p.cfg.HTTPClient, p.cfg.TokenEndpoint, cfg, code, o, p.cfg.ClientIDParam, p.cfg.TokenExtraHeaders)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p.cfg.ProviderName, err)
		}
		result = ToTokenResult(tok)
		// Handle non-standard scope separators in response
		if p.cfg.ScopeSeparator == "," && tok.Scope != "" {
			result.Scopes = strings.Split(tok.Scope, ",")
		}
	} else {
		tok, err := ExchangeCode(ctx, p.cfg.HTTPClient, p.cfg.TokenEndpoint, cfg, code, o, p.cfg.TokenExtraHeaders)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p.cfg.ProviderName, err)
		}
		result = ToTokenResult(tok)
	}

	// Run post-exchange hook if configured (e.g., Instagram long-lived token exchange)
	if p.cfg.PostExchangeHook != nil {
		hooked, err := p.cfg.PostExchangeHook(ctx, resolveClient(p.cfg.HTTPClient), cfg, result)
		if err != nil {
			// Fall back to the original token if the hook fails — the hook is an optional enrichment step (e.g., Instagram long-lived token exchange).
			return result, nil //nolint:nilerr // intentional fallback
		}
		return hooked, nil
	}

	return result, nil
}

func (p *GenericProvider) Refresh(ctx context.Context, token oidc.RefreshInput) (*oidc.TokenResult, error) {
	// Custom refresh (TikTok JSON, Facebook fb_exchange_token, Instagram ig_refresh_token)
	if p.cfg.RefreshFunc != nil {
		result, err := p.cfg.RefreshFunc(ctx, resolveClient(p.cfg.HTTPClient), p.cfg.ProviderConfig, token)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p.cfg.ProviderName, err)
		}
		return result, nil
	}

	// Standard OAuth2 refresh_token grant (RFC 6749 §6)
	endpoint := p.cfg.RefreshEndpoint
	if endpoint == "" {
		endpoint = p.cfg.TokenEndpoint
	}

	result, err := oidc.RefreshToken(ctx, oidc.RefreshConfig{
		TokenEndpoint: endpoint,
		ClientID:      p.cfg.ClientID,
		ClientSecret:  p.cfg.ClientSecret,
		RefreshToken:  token.RefreshToken,
		HTTPClient:    resolveClient(p.cfg.HTTPClient),
	})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", p.cfg.ProviderName, err)
	}
	return result, nil
}

func (p *GenericProvider) UserInfo(ctx context.Context, accessToken string) (*oidc.UserInfo, error) {
	// ID-token-based user info (e.g., Apple)
	if p.cfg.IDTokenAsUserInfo {
		return nil, fmt.Errorf("%s: UserInfo endpoint not supported; use ID token claims via Manager.ExchangeAndUserInfo", p.cfg.ProviderName)
	}

	if p.cfg.UserInfoEndpoint == "" {
		return nil, fmt.Errorf("%s: userinfo endpoint not configured", p.cfg.ProviderName)
	}

	// Build the request URL. The access token is sent only in the Authorization header (see FetchJSON); it is never placed in the query string. Any legacy "{access_token}" query parameter in the configured endpoint is dropped so it neither substitutes the token nor leaks as a literal query value.
	reqURL := stripAccessTokenPlaceholder(p.cfg.UserInfoEndpoint)

	var raw map[string]any
	if err := FetchJSON(ctx, p.cfg.HTTPClient, reqURL, accessToken, &raw); err != nil {
		return nil, fmt.Errorf("%s userinfo: %w", p.cfg.ProviderName, err)
	}

	info := p.mapUserInfo(raw)

	// Run post-userinfo hook if configured (e.g., GitHub email fallback)
	if p.cfg.PostUserInfoHook != nil {
		if err := p.cfg.PostUserInfoHook(ctx, accessToken, info); err != nil {
			// Return what we have if the hook fails — the hook is an optional enrichment step (e.g., GitHub email fallback).
			return info, nil //nolint:nilerr // intentional fallback
		}
	}

	return info, nil
}

// stripAccessTokenPlaceholder removes any query parameter whose value carries a legacy "{access_token}" placeholder. Bearer tokens are header-only; a stale placeholder must not substitute the token or leak into the query string.
func stripAccessTokenPlaceholder(endpoint string) string {
	if !strings.Contains(endpoint, "{access_token}") {
		return endpoint
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return endpoint
	}
	q := u.Query()
	for k, vs := range q {
		for _, v := range vs {
			if strings.Contains(v, "{access_token}") {
				q.Del(k)
				break
			}
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// --- oidc.ProviderMeta implementation ---

func (p *GenericProvider) Label() string        { return p.cfg.Label }
func (p *GenericProvider) ProviderType() string { return p.cfg.Type }

// --- Internal helpers ---

// mapUserInfo extracts standard fields from a raw JSON response using the configured mapping.
func (p *GenericProvider) mapUserInfo(raw map[string]any) *oidc.UserInfo {
	m := p.cfg.UserInfo
	data := raw

	// Navigate to nested path if configured (e.g., TikTok "data.user")
	if m.ResponsePath != "" {
		if nested := NestedMap(raw, m.ResponsePath); nested != nil {
			data = nested
		}
	}

	info := &oidc.UserInfo{Raw: raw}

	if m.SubjectKey != "" {
		info.Subject = StrVal(data, m.SubjectKey)
		// Fall back to numeric/non-string ID conversion
		if info.Subject == "" {
			if v, ok := data[m.SubjectKey]; ok {
				info.Subject = fmt.Sprintf("%v", v)
			}
		}
	}
	if m.EmailKey != "" {
		info.Email = StrVal(data, m.EmailKey)
	}
	if m.EmailVerifiedKey != "" {
		info.EmailVerified = BoolVal(data, m.EmailVerifiedKey)
	}
	if m.NameKey != "" {
		info.Name = StrVal(data, m.NameKey)
	}
	if m.GivenNameKey != "" {
		info.GivenName = StrVal(data, m.GivenNameKey)
	}
	if m.FamilyNameKey != "" {
		info.FamilyName = StrVal(data, m.FamilyNameKey)
	}
	if m.PictureKey != "" {
		info.Picture = StrVal(data, m.PictureKey)
	}
	if m.LocaleKey != "" {
		info.Locale = StrVal(data, m.LocaleKey)
	}

	return info
}

// Compile-time checks.
var (
	_ oidc.Provider     = (*GenericProvider)(nil)
	_ oidc.ProviderMeta = (*GenericProvider)(nil)
)
