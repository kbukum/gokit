// Package testutil provides test utilities for OAuth/OIDC provider testing.
//
// It includes a mock OAuth server that simulates the standard OAuth2 flow (authorization, token exchange, userinfo)
// and a MockProvider for unit testing code that depends on oidc.Provider without needing HTTP.
//
// Usage with MockOAuthServer (integration testing):
//
//	func TestMyProvider(t *testing.T) {
//	    srv := testutil.NewMockOAuthServer()
//	    defer srv.Close()
//
//	    p := providers.NewGeneric(providers.GenericConfig{
//	        ProviderConfig: providers.ProviderConfig{
//	            ClientID:     "test-client",
//	            ClientSecret: "test-secret",
//	            RedirectURL:  "http://localhost/callback",
//	        },
//	        ProviderName:     "test",
//	        AuthEndpoint:     srv.AuthURL(),
//	        TokenEndpoint:    srv.TokenURL(),
//	        UserInfoEndpoint: srv.UserInfoURL(),
//	        UserInfo:         testutil.StandardUserInfoMapper(),
//	    })
//
//	    tokens, err := p.Exchange(context.Background(), "valid-code")
//	    // ... assert tokens
//	}
//
// Usage with MockProvider (unit testing):
//
//	mock := &testutil.MockProvider{
//	    ProviderName: "google",
//	    ExchangeResult: &oidc.TokenResult{AccessToken: "tok"},
//	    UserInfoResult: &oidc.UserInfo{Sub: "u1", Email: "a@b.com"},
//	}
//	// inject mock into code that calls Provider.Exchange / Provider.UserInfo
package testutil
