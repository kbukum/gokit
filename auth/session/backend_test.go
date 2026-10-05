package session

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kbukum/gokit/auth"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/util"
)

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

// remoteBackend models a gateway that delegates session ownership to another service.
type remoteBackend struct {
	signIn  func(context.Context, SignIn) (Grant, error)
	status  func(context.Context, string) (Grant, error)
	signOut func(context.Context, SignOut) error
	calls   []string
}

func (b *remoteBackend) SignIn(ctx context.Context, in SignIn) (Grant, error) {
	b.calls = append(b.calls, "signIn")
	return b.signIn(ctx, in)
}

func (b *remoteBackend) Status(ctx context.Context, token string) (Grant, error) {
	b.calls = append(b.calls, "status")
	return b.status(ctx, token)
}

func (b *remoteBackend) SignOut(ctx context.Context, in SignOut) error {
	b.calls = append(b.calls, "signOut")
	return b.signOut(ctx, in)
}

const remoteToken = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

func remoteGrant(now time.Time) Grant {
	p := caller()
	p.Credential, p.Reference, p.ExpiresAt = auth.Session, "remote", now.Add(30*time.Minute)
	return Grant{Token: remoteToken, Principal: p, CSRFToken: "csrf-remote"}
}

func statusRequest(token string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "https://example.test/auth/session", http.NoBody)
	r.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: token})
	return r
}

func remoteHandler(t *testing.T, backend Backend, clock util.Clock) http.Handler {
	t.Helper()
	h, err := NewHandler(backend, HandlerConfig{Origin: "https://example.test", Errors: errorWriter, Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestRemoteBackendServesBrowserContract(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	var signedIn SignIn
	var signedOut SignOut
	backend := &remoteBackend{
		signIn: func(_ context.Context, in SignIn) (Grant, error) {
			signedIn = in
			return remoteGrant(now), nil
		},
		status: func(_ context.Context, token string) (Grant, error) {
			if token != remoteToken {
				return Grant{}, auth.Failure("SESSION_INVALID")
			}
			grant := remoteGrant(now)
			grant.Token = ""
			return grant, nil
		},
		signOut: func(_ context.Context, in SignOut) error {
			signedOut = in
			return nil
		},
	}
	h := remoteHandler(t, backend, fixedClock{now})

	w := httptest.NewRecorder()
	h.ServeHTTP(w, loginRequest(remoteToken))
	if w.Code != http.StatusOK || signedIn.Presented != remoteToken || signedIn.Login.Username != "u" || signedIn.Login.Password != "p" {
		t.Fatalf("login = %d %+v", w.Code, signedIn.Login.Username)
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Value != remoteToken || cookies[0].MaxAge != 1800 || !cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("login cookie = %+v", cookies)
	}
	var out Response
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || out.CSRFToken != "csrf-remote" || out.Identity.Subject != caller().Subject || strings.Contains(w.Body.String(), remoteToken) {
		t.Fatalf("login body = %s", w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, statusRequest(remoteToken))
	if w.Code != http.StatusOK || w.Header().Get("Set-Cookie") != "" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status = %d %v", w.Code, w.Header())
	}

	logout := httptest.NewRequest(http.MethodPost, "https://example.test/auth/logout", http.NoBody)
	logout.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: remoteToken})
	logout.Header.Set("X-CSRF-Token", "csrf-remote")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, logout)
	if w.Code != http.StatusNoContent || signedOut != (SignOut{Token: remoteToken, CSRFToken: "csrf-remote"}) {
		t.Fatalf("logout = %d %+v", w.Code, signedOut)
	}
	if cleared := w.Result().Cookies(); len(cleared) != 1 || cleared[0].MaxAge != -1 {
		t.Fatalf("logout cookie = %+v", cleared)
	}
}

func TestRemoteBackendRejectsBeforeDelegation(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	fail := errors.New("backend must not be called")
	backend := &remoteBackend{
		signIn:  func(context.Context, SignIn) (Grant, error) { return Grant{}, fail },
		status:  func(context.Context, string) (Grant, error) { return Grant{}, fail },
		signOut: func(context.Context, SignOut) error { return fail },
	}
	h := remoteHandler(t, backend, fixedClock{now})
	for name, r := range map[string]*http.Request{
		"logout without csrf": func() *http.Request {
			r := httptest.NewRequest(http.MethodPost, "https://example.test/auth/logout", http.NoBody)
			r.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: remoteToken})
			return r
		}(),
		"logout duplicate csrf": func() *http.Request {
			r := httptest.NewRequest(http.MethodPost, "https://example.test/auth/logout", http.NoBody)
			r.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: remoteToken})
			r.Header.Add("X-CSRF-Token", "a")
			r.Header.Add("X-CSRF-Token", "b")
			return r
		}(),
		"login malformed cookie":  loginRequest("short"),
		"status malformed cookie": statusRequest("short"),
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code < 400 {
			t.Fatalf("%s accepted: %d", name, w.Code)
		}
	}
	if len(backend.calls) != 0 {
		t.Fatalf("backend called before local validation: %v", backend.calls)
	}
}

func TestRemoteBackendFailuresAndExpiredGrants(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	expired := remoteGrant(now)
	expired.Principal.ExpiresAt = now
	backend := &remoteBackend{
		signIn: func(context.Context, SignIn) (Grant, error) { return expired, nil },
		status: func(context.Context, string) (Grant, error) {
			return Grant{}, apperrors.ServiceUnavailable("access")
		},
		signOut: func(context.Context, SignOut) error { return auth.Failure("SESSION_INVALID") },
	}
	h := remoteHandler(t, backend, fixedClock{now})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, loginRequest(""))
	if w.Code != http.StatusUnauthorized || w.Header().Get("Set-Cookie") != "" {
		t.Fatalf("expired grant = %d", w.Code)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, statusRequest(remoteToken))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("unavailable status = %d", w.Code)
	}
	logout := httptest.NewRequest(http.MethodPost, "https://example.test/auth/logout", http.NoBody)
	logout.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: remoteToken})
	logout.Header.Set("X-CSRF-Token", "csrf")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, logout)
	if w.Code != http.StatusUnauthorized || w.Header().Get("Set-Cookie") != "" {
		t.Fatalf("failed logout = %d", w.Code)
	}
}

func TestNewHandlerRequiresBackendConfig(t *testing.T) {
	t.Parallel()
	backend := &remoteBackend{}
	clock := fixedClock{}
	for name, cfg := range map[string]HandlerConfig{
		"origin": {Errors: errorWriter, Clock: clock},
		"errors": {Origin: "https://example.test", Clock: clock},
		"clock":  {Origin: "https://example.test", Errors: errorWriter},
	} {
		if _, err := NewHandler(backend, cfg); err == nil {
			t.Fatalf("missing %s accepted", name)
		}
	}
	if _, err := NewHandler(nil, HandlerConfig{Origin: "https://example.test", Errors: errorWriter, Clock: clock}); err == nil {
		t.Fatal("nil backend accepted")
	}
}

func TestRequestCSRF(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		method  string
		values  []string
		token   string
		wantErr bool
	}{
		{http.MethodGet, nil, "", false},
		{http.MethodHead, []string{"ignored"}, "", false},
		{http.MethodPost, []string{"t"}, "t", false},
		{http.MethodPost, nil, "", true},
		{http.MethodDelete, []string{"a", "b"}, "", true},
		{http.MethodPut, []string{""}, "", true},
	} {
		r := httptest.NewRequest(tt.method, "https://example.test/", http.NoBody)
		for _, value := range tt.values {
			r.Header.Add("X-CSRF-Token", value)
		}
		token, err := RequestCSRF(r)
		if (err != nil) != tt.wantErr || token != tt.token {
			t.Fatalf("%s %v = %q, %v", tt.method, tt.values, token, err)
		}
		if err != nil && apperrors.Normalize(err).Reason != "CSRF_INVALID" {
			t.Fatalf("csrf reason = %v", err)
		}
	}
}

func newLocalHandler(m *Manager, verifier LoginVerifier, origin string, writer ErrorWriter) (http.Handler, error) {
	backend, err := NewLocalBackend(m, verifier)
	if err != nil {
		return nil, err
	}
	return NewHandler(backend, HandlerConfig{Origin: origin, Errors: writer, Clock: m.clock})
}

func TestRemoteBackendRejectsInvalidPrincipals(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for name, mutate := range map[string]func(*auth.Principal){
		"empty subject":        func(p *auth.Principal) { p.Subject = "" },
		"missing reference":    func(p *auth.Principal) { p.Reference = "" },
		"api key credential":   func(p *auth.Principal) { p.Credential = auth.APIKey },
		"missing credential":   func(p *auth.Principal) { p.Credential = "" },
		"invalid restrictions": func(p *auth.Principal) { p.Restrictions = auth.Restrictions{} },
	} {
		grant := remoteGrant(now)
		mutate(&grant.Principal)
		backend := &remoteBackend{
			signIn: func(context.Context, SignIn) (Grant, error) { return grant, nil },
			status: func(context.Context, string) (Grant, error) { return grant, nil },
		}
		h := remoteHandler(t, backend, fixedClock{now})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, loginRequest(""))
		if w.Code < 400 || w.Header().Get("Set-Cookie") != "" {
			t.Fatalf("%s: login = %d %v", name, w.Code, w.Header())
		}
		w = httptest.NewRecorder()
		h.ServeHTTP(w, statusRequest(remoteToken))
		if w.Code < 400 {
			t.Fatalf("%s: status = %d", name, w.Code)
		}
	}
}

func TestRemoteBackendUnencodableGrantSetsNoCookie(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	grant := remoteGrant(now)
	grant.Principal.ExpiresAt = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)
	backend := &remoteBackend{signIn: func(context.Context, SignIn) (Grant, error) { return grant, nil }}
	w := httptest.NewRecorder()
	remoteHandler(t, backend, fixedClock{now}).ServeHTTP(w, loginRequest(""))
	if w.Code < 400 || w.Header().Get("Set-Cookie") != "" {
		t.Fatalf("login = %d %v", w.Code, w.Header())
	}
}
