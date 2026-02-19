package httpclient

import "net/http"

// AuthType identifies the authentication method.
type AuthType int

const (
	// AuthNone disables authentication.
	AuthNone AuthType = iota
	// AuthBearer uses Bearer token authentication.
	AuthBearer
	// AuthBasic uses HTTP Basic authentication.
	AuthBasic
	// AuthAPIKey uses API key authentication (header or query parameter).
	AuthAPIKey
	// AuthCustom uses a custom authentication function.
	AuthCustom
)

// AuthConfig configures request authentication.
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
	// In specifies where to place the API key: "header" (default) or "query" (AuthAPIKey).
	In string
	// Name is the header or query parameter name (AuthAPIKey). Defaults to "X-API-Key".
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

// APIKeyAuth creates an API key auth config sent via header.
func APIKeyAuth(key string) *AuthConfig {
	return &AuthConfig{Type: AuthAPIKey, Key: key, In: "header", Name: "X-API-Key"}
}

// APIKeyAuthHeader creates an API key auth config with a custom header name.
func APIKeyAuthHeader(key, headerName string) *AuthConfig {
	return &AuthConfig{Type: AuthAPIKey, Key: key, In: "header", Name: headerName}
}

// APIKeyAuthQuery creates an API key auth config sent via query parameter.
func APIKeyAuthQuery(key, paramName string) *AuthConfig {
	return &AuthConfig{Type: AuthAPIKey, Key: key, In: "query", Name: paramName}
}

// CustomAuth creates a custom auth config with a request modifier function.
func CustomAuth(fn func(*http.Request)) *AuthConfig {
	return &AuthConfig{Type: AuthCustom, Apply: fn}
}

// apply applies authentication to an HTTP request.
func (a *AuthConfig) apply(req *http.Request) {
	if a == nil {
		return
	}
	switch a.Type {
	case AuthBearer:
		req.Header.Set("Authorization", "Bearer "+a.Token)
	case AuthBasic:
		req.SetBasicAuth(a.Username, a.Password)
	case AuthAPIKey:
		name := a.Name
		if name == "" {
			name = "X-API-Key"
		}
		if a.In == "query" {
			q := req.URL.Query()
			q.Set(name, a.Key)
			req.URL.RawQuery = q.Encode()
		} else {
			req.Header.Set(name, a.Key)
		}
	case AuthCustom:
		if a.Apply != nil {
			a.Apply(req)
		}
	}
}
