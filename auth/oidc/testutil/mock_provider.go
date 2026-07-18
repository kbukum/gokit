package testutil

import (
	"context"
	"errors"
	"sync"

	"github.com/kbukum/gokit/auth/oidc"
)

// MockProvider implements oidc.Provider
// and oidc.ProviderMeta for testing code that depends on OAuth providers without needing a real HTTP server.
//
// Use it to inject known responses into code that calls Exchange/UserInfo:
//
//	mock := &testutil.MockProvider{
//	    ProviderName: "google",
//	    ExchangeResult: &oidc.TokenResult{AccessToken: "tok"},
//	    UserInfoResult: &oidc.UserInfo{Sub: "u1", Email: "a@b.com"},
//	}
//	// inject mock into code that calls Provider.Exchange / Provider.UserInfo
type MockProvider struct {
	// ProviderName is returned by Name().
	ProviderName string

	// ProviderLabel is returned by Label(). Defaults to ProviderName if empty.
	ProviderLabel string

	// ProviderTypeStr is returned by ProviderType(). Defaults to "identity" if empty.
	ProviderTypeStr string

	// AuthURLResult is returned by AuthURL. If empty, a default is generated.
	AuthURLResult string

	// ExchangeResult is returned by Exchange when ExchangeErr is nil.
	ExchangeResult *oidc.TokenResult

	// ExchangeErr is returned by Exchange when set.
	ExchangeErr error

	// UserInfoResult is returned by UserInfo when UserInfoErr is nil.
	UserInfoResult *oidc.UserInfo

	// UserInfoErr is returned by UserInfo when set.
	UserInfoErr error

	// RefreshResult is returned by Refresh when RefreshErr is nil.
	RefreshResult *oidc.TokenResult

	// RefreshErr is returned by Refresh when set.
	RefreshErr error

	// RefreshFunc, if set, is called instead of returning RefreshResult/RefreshErr.
	RefreshFunc func(ctx context.Context, token oidc.RefreshInput) (*oidc.TokenResult, error)

	// ExchangeFunc, if set, is called instead of returning ExchangeResult/ExchangeErr.
	// This allows dynamic behavior in tests.
	ExchangeFunc func(ctx context.Context, code string, opts ...oidc.ExchangeOption) (*oidc.TokenResult, error)

	// UserInfoFunc, if set, is called instead of returning UserInfoResult/UserInfoErr.
	UserInfoFunc func(ctx context.Context, accessToken string) (*oidc.UserInfo, error)

	mu            sync.Mutex
	exchangeCalls []string // codes passed to Exchange
	userInfoCalls []string // tokens passed to UserInfo
	refreshCalls  []oidc.RefreshInput
	authURLCalls  []AuthURLCall
}

// AuthURLCall records arguments passed to AuthURL.
type AuthURLCall struct {
	State string
	Opts  []oidc.AuthURLOption
}

// Compile-time interface checks.
var (
	_ oidc.Provider     = (*MockProvider)(nil)
	_ oidc.ProviderMeta = (*MockProvider)(nil)
)

func (m *MockProvider) Name() string { return m.ProviderName }

func (m *MockProvider) Label() string {
	if m.ProviderLabel != "" {
		return m.ProviderLabel
	}
	return m.ProviderName
}

func (m *MockProvider) ProviderType() string {
	if m.ProviderTypeStr != "" {
		return m.ProviderTypeStr
	}
	return "identity"
}

func (m *MockProvider) AuthURL(state string, opts ...oidc.AuthURLOption) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.authURLCalls = append(m.authURLCalls, AuthURLCall{State: state, Opts: opts})

	if m.AuthURLResult != "" {
		return m.AuthURLResult
	}
	return "https://mock.example.com/auth?state=" + state
}

func (m *MockProvider) Exchange(ctx context.Context, code string, opts ...oidc.ExchangeOption) (*oidc.TokenResult, error) {
	m.mu.Lock()
	m.exchangeCalls = append(m.exchangeCalls, code)
	m.mu.Unlock()

	if m.ExchangeFunc != nil {
		return m.ExchangeFunc(ctx, code, opts...)
	}
	if m.ExchangeErr != nil {
		return nil, m.ExchangeErr
	}
	if m.ExchangeResult != nil {
		return m.ExchangeResult, nil
	}
	return nil, errors.New("mock: no exchange result configured")
}

func (m *MockProvider) UserInfo(ctx context.Context, accessToken string) (*oidc.UserInfo, error) {
	m.mu.Lock()
	m.userInfoCalls = append(m.userInfoCalls, accessToken)
	m.mu.Unlock()

	if m.UserInfoFunc != nil {
		return m.UserInfoFunc(ctx, accessToken)
	}
	if m.UserInfoErr != nil {
		return nil, m.UserInfoErr
	}
	if m.UserInfoResult != nil {
		return m.UserInfoResult, nil
	}
	return nil, errors.New("mock: no userinfo result configured")
}

func (m *MockProvider) Refresh(ctx context.Context, token oidc.RefreshInput) (*oidc.TokenResult, error) {
	m.mu.Lock()
	m.refreshCalls = append(m.refreshCalls, token)
	m.mu.Unlock()

	if m.RefreshFunc != nil {
		return m.RefreshFunc(ctx, token)
	}
	if m.RefreshErr != nil {
		return nil, m.RefreshErr
	}
	if m.RefreshResult != nil {
		return m.RefreshResult, nil
	}
	return nil, errors.New("mock: no refresh result configured")
}

// --- Inspection ---

// ExchangeCalls returns all authorization codes passed to Exchange.
func (m *MockProvider) ExchangeCalls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]string, len(m.exchangeCalls))
	copy(cp, m.exchangeCalls)
	return cp
}

// UserInfoCalls returns all access tokens passed to UserInfo.
func (m *MockProvider) UserInfoCalls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]string, len(m.userInfoCalls))
	copy(cp, m.userInfoCalls)
	return cp
}

// RefreshCalls returns all RefreshInput values passed to Refresh.
func (m *MockProvider) RefreshCalls() []oidc.RefreshInput {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]oidc.RefreshInput, len(m.refreshCalls))
	copy(cp, m.refreshCalls)
	return cp
}

// AuthURLCalls returns all recorded AuthURL invocations.
func (m *MockProvider) AuthURLCalls() []AuthURLCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]AuthURLCall, len(m.authURLCalls))
	copy(cp, m.authURLCalls)
	return cp
}

// Reset clears all recorded calls.
func (m *MockProvider) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.exchangeCalls = nil
	m.userInfoCalls = nil
	m.refreshCalls = nil
	m.authURLCalls = nil
}
