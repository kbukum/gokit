package providers

import (
	"context"
	"fmt"
	"sync"

	"github.com/kbukum/gokit/auth/oidc"
)

// Manager manages multiple OAuth providers and provides a unified interface
// for starting OAuth flows and handling callbacks.
type Manager struct {
	mu        sync.RWMutex
	providers map[string]oidc.Provider
}

// NewManager creates a new provider manager.
func NewManager(provs ...oidc.Provider) *Manager {
	m := &Manager{
		providers: make(map[string]oidc.Provider, len(provs)),
	}
	for _, p := range provs {
		m.providers[p.Name()] = p
	}
	return m
}

// Register adds a provider to the manager.
func (m *Manager) Register(p oidc.Provider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.providers[p.Name()] = p
}

// Get returns a provider by name.
func (m *Manager) Get(name string) (oidc.Provider, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.providers[name]
	if !ok {
		return nil, fmt.Errorf("unknown OAuth provider: %s", name)
	}
	return p, nil
}

// List returns the names of all registered providers.
func (m *Manager) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.providers))
	for name := range m.providers {
		names = append(names, name)
	}
	return names
}

// AuthURL generates an authorization URL for the named provider.
func (m *Manager) AuthURL(providerName, state string, opts ...oidc.AuthURLOption) (string, error) {
	p, err := m.Get(providerName)
	if err != nil {
		return "", err
	}
	return p.AuthURL(state, opts...), nil
}

// Exchange performs a token exchange with the named provider.
func (m *Manager) Exchange(ctx context.Context, providerName, code string, opts ...oidc.ExchangeOption) (*oidc.TokenResult, error) {
	p, err := m.Get(providerName)
	if err != nil {
		return nil, err
	}
	return p.Exchange(ctx, code, opts...)
}

// UserInfo fetches user info from the named provider.
func (m *Manager) UserInfo(ctx context.Context, providerName, accessToken string) (*oidc.UserInfo, error) {
	p, err := m.Get(providerName)
	if err != nil {
		return nil, err
	}
	return p.UserInfo(ctx, accessToken)
}

// ExchangeAndUserInfo performs token exchange followed by user info fetch.
// For providers that don't support UserInfo (e.g., Apple), it parses the ID token instead.
func (m *Manager) ExchangeAndUserInfo(ctx context.Context, providerName, code string, opts ...oidc.ExchangeOption) (*oidc.TokenResult, *oidc.UserInfo, error) {
	p, err := m.Get(providerName)
	if err != nil {
		return nil, nil, err
	}

	tokens, err := p.Exchange(ctx, code, opts...)
	if err != nil {
		return nil, nil, err
	}

	user, err := p.UserInfo(ctx, tokens.AccessToken)
	if err != nil {
		// Fallback: try to parse ID token if UserInfo fails (e.g., Apple)
		if tokens.IDToken != "" {
			user, err = ParseIDTokenClaims(tokens.IDToken)
			if err != nil {
				return tokens, nil, fmt.Errorf("userinfo failed and ID token parse failed: %w", err)
			}
			return tokens, user, nil
		}
		return tokens, nil, fmt.Errorf("userinfo: %w", err)
	}

	return tokens, user, nil
}
