package testutil

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
)

// MockOAuthServer simulates an OAuth2 provider's endpoints for testing.
// It handles token exchange, userinfo, and optionally JSON token exchange.
//
// The server is fully configurable: set custom response data, simulate errors,
// and inspect received requests.
type MockOAuthServer struct {
	server *httptest.Server

	mu            sync.Mutex
	tokenResponse map[string]interface{}
	userResponse  map[string]interface{}
	tokenRequests []map[string]string
	failToken     bool
	failUserInfo  bool
	idTokenClaims map[string]interface{}
}

// NewMockOAuthServer creates and starts a mock OAuth server with sensible defaults.
// Call Close() when done.
func NewMockOAuthServer() *MockOAuthServer {
	m := &MockOAuthServer{
		tokenResponse: map[string]interface{}{
			"access_token":  "mock-access-token",
			"refresh_token": "mock-refresh-token",
			"token_type":    "Bearer",
			"expires_in":    3600,
			"scope":         "openid email profile",
		},
		userResponse: map[string]interface{}{
			"sub":            "user-123",
			"id":             "user-123",
			"email":          "user@example.com",
			"email_verified": true,
			"name":           "Test User",
			"given_name":     "Test",
			"family_name":    "User",
			"picture":        "https://example.com/photo.jpg",
			"locale":         "en",
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/authorize", m.handleAuth)
	mux.HandleFunc("/token", m.handleToken)
	mux.HandleFunc("/userinfo", m.handleUserInfo)
	m.server = httptest.NewServer(mux)
	return m
}

// Close shuts down the mock server.
func (m *MockOAuthServer) Close() {
	m.server.Close()
}

// --- URL accessors ---

// BaseURL returns the mock server's base URL.
func (m *MockOAuthServer) BaseURL() string { return m.server.URL }

// AuthURL returns the mock authorization endpoint URL.
func (m *MockOAuthServer) AuthURL() string { return m.server.URL + "/authorize" }

// TokenURL returns the mock token exchange endpoint URL.
func (m *MockOAuthServer) TokenURL() string { return m.server.URL + "/token" }

// UserInfoURL returns the mock userinfo endpoint URL.
func (m *MockOAuthServer) UserInfoURL() string { return m.server.URL + "/userinfo" }

// --- Configuration ---

// SetTokenResponse overrides the token endpoint response.
func (m *MockOAuthServer) SetTokenResponse(resp map[string]interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tokenResponse = resp
}

// SetUserResponse overrides the userinfo endpoint response.
func (m *MockOAuthServer) SetUserResponse(resp map[string]interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.userResponse = resp
}

// SetIDTokenClaims sets claims that will be encoded as an ID token in the token response.
// When set, the token response will include an "id_token" field containing a JWT
// with these claims (unsigned, for testing only).
func (m *MockOAuthServer) SetIDTokenClaims(claims map[string]interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.idTokenClaims = claims
}

// FailToken makes the token endpoint return an error response.
func (m *MockOAuthServer) FailToken(fail bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failToken = fail
}

// FailUserInfo makes the userinfo endpoint return an error response.
func (m *MockOAuthServer) FailUserInfo(fail bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failUserInfo = fail
}

// --- Inspection ---

// TokenRequests returns all token exchange requests received.
// Each entry is a map of form field name → value.
func (m *MockOAuthServer) TokenRequests() []map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]map[string]string, len(m.tokenRequests))
	copy(cp, m.tokenRequests)
	return cp
}

// LastTokenRequest returns the most recent token exchange request, or nil.
func (m *MockOAuthServer) LastTokenRequest() map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.tokenRequests) == 0 {
		return nil
	}
	return m.tokenRequests[len(m.tokenRequests)-1]
}

// Reset clears all recorded requests and restores default responses.
func (m *MockOAuthServer) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tokenRequests = nil
	m.failToken = false
	m.failUserInfo = false
	m.idTokenClaims = nil
}

// --- HTTP handlers ---

func (m *MockOAuthServer) handleAuth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"redirect": true,
		"params":   r.URL.Query(),
	})
}

func (m *MockOAuthServer) handleToken(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Record the request (works for both form and JSON)
	params := make(map[string]string)
	contentType := r.Header.Get("Content-Type")

	if contentType == "application/json" {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			params = body
		}
	} else {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		for k, v := range r.PostForm {
			if len(v) > 0 {
				params[k] = v[0]
			}
		}
	}
	m.tokenRequests = append(m.tokenRequests, params)

	if m.failToken {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
		return
	}

	resp := make(map[string]interface{})
	for k, v := range m.tokenResponse {
		resp[k] = v
	}

	// Generate ID token if claims are set
	if m.idTokenClaims != nil {
		resp["id_token"] = BuildTestIDToken(m.idTokenClaims)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (m *MockOAuthServer) handleUserInfo(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.failUserInfo {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_token"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(m.userResponse)
}

// BuildTestIDToken creates an unsigned JWT with the given claims.
// Exported for use in tests that need to construct ID tokens directly.
func BuildTestIDToken(claims map[string]interface{}) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload, _ := json.Marshal(claims)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payload)
	sig := base64.RawURLEncoding.EncodeToString([]byte("test-signature"))
	return fmt.Sprintf("%s.%s.%s", header, payloadB64, sig)
}
