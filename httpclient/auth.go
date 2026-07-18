package httpclient

import (
	"fmt"
	"net/http"
)

// AuthType identifies the authentication method.
type AuthType int

const (
	// AuthNone disables authentication.
	AuthNone AuthType = iota
	// AuthBearer uses Bearer token authentication.
	AuthBearer
	// AuthBasic uses HTTP Basic authentication.
	AuthBasic
	// AuthAPIKey uses API key authentication via a request header.
	AuthAPIKey
	// AuthCustom uses a custom authentication function.
	AuthCustom
)

// AuthConfig configures request authentication.
//
// Credentials are always transmitted in request headers, never in the URL query string, so tokens do not leak into server logs, proxies, or browser history. AuthConfig redacts its secret fields in String, GoString, and any fmt verb, so logging a Config that embeds it never exposes credentials.
type AuthConfig struct {
	// Type is the authentication method.
	Type AuthType
	// Token is the bearer token (AuthBearer).
	Token string
	// Username is the basic auth username (AuthBasic).
	Username string
	// Password is the basic auth password (AuthBasic).
	Password string
	// Key is the API key value (AuthAPIKey).
	Key string
	// Name is the header name for the API key (AuthAPIKey). Defaults to "X-API-Key".
	Name string
	// Apply is a custom function to modify the request (AuthCustom).
	Apply func(*http.Request)
}

// BearerAuth creates a bearer token auth config.
func BearerAuth(token string) *AuthConfig {
	return &AuthConfig{Type: AuthBearer, Token: token}
}

// BasicAuth creates a basic auth config.
func BasicAuth(username, password string) *AuthConfig {
	return &AuthConfig{Type: AuthBasic, Username: username, Password: password}
}

// APIKeyAuth creates an API key auth config sent via the "X-API-Key" header.
func APIKeyAuth(key string) *AuthConfig {
	return &AuthConfig{Type: AuthAPIKey, Key: key, Name: "X-API-Key"}
}

// APIKeyAuthHeader creates an API key auth config with a custom header name.
func APIKeyAuthHeader(key, headerName string) *AuthConfig {
	return &AuthConfig{Type: AuthAPIKey, Key: key, Name: headerName}
}

// CustomAuth creates a custom auth config with a request modifier function.
func CustomAuth(fn func(*http.Request)) *AuthConfig {
	return &AuthConfig{Type: AuthCustom, Apply: fn}
}

// apply applies authentication to an HTTP request. Credentials are always set as request headers — never in the query string.
func (a *AuthConfig) apply(req *http.Request) {
	if a == nil {
		return
	}
	switch a.Type {
	case AuthNone:
		return
	case AuthBearer:
		req.Header.Set("Authorization", "Bearer "+a.Token)
	case AuthBasic:
		req.SetBasicAuth(a.Username, a.Password)
	case AuthAPIKey:
		name := a.Name
		if name == "" {
			name = "X-API-Key"
		}
		req.Header.Set(name, a.Key)
	case AuthCustom:
		if a.Apply != nil {
			a.Apply(req)
		}
	}
}

// String returns a redacted, log-safe description of the auth config. Secret values (token, password, API key) are never included.
func (a *AuthConfig) String() string {
	if a == nil {
		return "AuthConfig(none)"
	}
	switch a.Type {
	case AuthNone:
		return "AuthConfig(none)"
	case AuthBearer:
		return "AuthConfig(bearer)"
	case AuthBasic:
		return fmt.Sprintf("AuthConfig(basic, user=%s)", a.Username)
	case AuthAPIKey:
		name := a.Name
		if name == "" {
			name = "X-API-Key"
		}
		return fmt.Sprintf("AuthConfig(apikey, header=%s)", name)
	case AuthCustom:
		return "AuthConfig(custom)"
	default:
		return "AuthConfig(unknown)"
	}
}

// GoString returns a redacted representation for the %#v verb.
func (a *AuthConfig) GoString() string {
	return a.String()
}
