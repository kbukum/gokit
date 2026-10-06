package session

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kbukum/gokit/auth"
	apperrors "github.com/kbukum/gokit/errors"
)

func loginRequest(token string) *http.Request {
	r := httptest.NewRequest("POST", "https://example.test/auth/login", strings.NewReader(`{"username":"u","password":"p"}`))
	r.Header.Set("Origin", "https://example.test")
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: token})
	}
	return r
}

func TestLoginRotatesPresentedSessionAndOldLogoutRevokesReplacement(t *testing.T) {
	m, clock := fixture(t, &memoryStore{rows: make(map[string]Record)})
	first, err := m.Create(context.Background(), caller())
	if err != nil {
		t.Fatal(err)
	}
	lifetime, release, err := m.Acquire(context.Background(), first.Principal.Reference)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	clock.Advance(time.Minute)
	h, err := newLocalHandler(m, LoginVerifierFunc(func(context.Context, Login) (auth.Principal, error) { return caller(), nil }), "https://example.test", errorWriter)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, loginRequest(first.Token))
	if w.Code != http.StatusOK || len(w.Result().Cookies()) != 1 {
		t.Fatal("relogin failed", w.Code)
	}
	next := w.Result().Cookies()[0]
	if next.Value == first.Token || !next.Expires.Equal(first.Principal.ExpiresAt) {
		t.Fatal("relogin reused identifier or extended expiry")
	}
	if lifetime.Err() == nil {
		t.Fatal("relogin left old stream alive")
	}
	if _, err := m.Authenticate(request(first.Token)); err == nil {
		t.Fatal("old credential remained active")
	}
	if err := m.Logout(context.Background(), first.Principal.Reference); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Authenticate(request(next.Value)); err == nil {
		t.Fatal("old logout did not revoke relogin")
	}
}

func TestLoginRejectsAmbiguityBeforePasswordVerification(t *testing.T) {
	for _, credential := range []string{"duplicate", "mixed", "empty", "bearer", "key", "malformed"} {
		t.Run(credential, func(t *testing.T) {
			s := &memoryStore{rows: make(map[string]Record)}
			m, _ := fixture(t, s)
			calls := 0
			h, err := newLocalHandler(m, LoginVerifierFunc(func(context.Context, Login) (auth.Principal, error) {
				calls++
				return caller(), nil
			}), "https://example.test", errorWriter)
			if err != nil {
				t.Fatal(err)
			}
			r := loginRequest(strings.Repeat("a", 43))
			switch credential {
			case "duplicate":
				r.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: strings.Repeat("b", 43)})
			case "mixed":
				r.Header.Set("X-API-Key", "key")
			case "empty":
				r.Header.Set("Cookie", auth.SessionCookie+"=")
			case "bearer":
				r.Header.Set("Authorization", "Bearer token")
			case "key":
				r.Header.Del("Cookie")
				r.Header.Set("X-API-Key", "key")
			case "malformed":
				r.Header.Set("Cookie", auth.SessionCookie+"=bad")
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code < 400 || calls != 0 || len(s.rows) != 0 || w.Header().Get("Set-Cookie") != "" {
				t.Fatal("credential ambiguity created state")
			}
		})
	}
}

func TestLoginLogoutDuringVerificationCannotResurrect(t *testing.T) {
	s := &memoryStore{rows: make(map[string]Record)}
	m, _ := fixture(t, s)
	first, err := m.Create(context.Background(), caller())
	if err != nil {
		t.Fatal(err)
	}
	h, err := newLocalHandler(m, LoginVerifierFunc(func(ctx context.Context, _ Login) (auth.Principal, error) {
		if err := m.Logout(ctx, first.Principal.Reference); err != nil {
			t.Fatal(err)
		}
		return caller(), nil
	}), "https://example.test", errorWriter)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, loginRequest(first.Token))
	if w.Code < 400 || w.Header().Get("Set-Cookie") != "" {
		t.Fatal("relogin resurrected committed logout")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.rows) != 1 {
		t.Fatal("logout during verification created a replacement family")
	}
}

// passwordLogin runs both login phases back to back, as the HTTP handler does around password verification.
func passwordLogin(ctx context.Context, m *Manager, token string, p auth.Principal) (Issued, error) {
	attempt, err := m.BeginLogin(ctx, token)
	if err != nil {
		return Issued{}, err
	}
	return m.CompleteLogin(ctx, attempt, p)
}

func TestLoginAttemptFencesStateCapturedBeforeVerification(t *testing.T) {
	s := &memoryStore{rows: make(map[string]Record)}
	m, _ := fixture(t, s)
	other, _ := fixture(t, s)
	ctx := context.Background()
	first, err := m.Create(ctx, caller())
	if err != nil {
		t.Fatal(err)
	}
	active, err := m.BeginLogin(ctx, first.Token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.CompleteLogin(ctx, active, caller()); err == nil {
		t.Fatal("attempt completed on a foreign manager")
	}
	if _, err := m.CompleteLogin(ctx, LoginAttempt{}, caller()); err == nil {
		t.Fatal("zero attempt completed")
	}
	if err := m.Logout(ctx, first.Principal.Reference); err != nil {
		t.Fatal(err)
	}
	if _, err := m.CompleteLogin(ctx, active, caller()); apperrors.Normalize(err).Code != apperrors.ErrCodeUnauthorized {
		t.Fatal("logout committed during verification lost to an active attempt", err)
	}
	terminal, err := m.BeginLogin(ctx, first.Token)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := m.CompleteLogin(ctx, terminal, caller())
	if err != nil || s.family(recovered.Principal.Reference) == s.family(first.Principal.Reference) {
		t.Fatal("already-terminal cookie did not recover into a fresh family", err)
	}
	s.mu.Lock()
	s.fail = errors.New("unavailable")
	s.mu.Unlock()
	if _, err := m.BeginLogin(ctx, recovered.Token); apperrors.Normalize(err).Code != apperrors.ErrCodeServiceUnavailable {
		t.Fatal("store failure at admission was not fail-closed", err)
	}
	s.mu.Lock()
	s.fail = nil
	s.mu.Unlock()
	if err := m.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := m.BeginLogin(ctx, ""); err == nil {
		t.Fatal("closed manager began a login")
	}
}

func TestLoginRecoveryAndLostResponse(t *testing.T) {
	s := &memoryStore{rows: make(map[string]Record)}
	m, clock := fixture(t, s)
	first, err := passwordLogin(context.Background(), m, "", caller())
	if err != nil {
		t.Fatal(err)
	}
	next, err := passwordLogin(context.Background(), m, first.Token, caller())
	if err != nil {
		t.Fatal(err)
	}
	retried, err := passwordLogin(context.Background(), m, first.Token, caller())
	if err != nil {
		t.Fatal("lost-response retry could not reauthenticate", err)
	}
	if retried.Token == next.Token || s.family(retried.Principal.Reference) == s.family(next.Principal.Reference) {
		t.Fatal("superseded credential reused its family")
	}
	for _, stale := range []string{first.Token, next.Token} {
		if _, err := m.Authenticate(request(stale)); err == nil {
			t.Fatal("lost-response replacement survived terminal relogin")
		}
	}
	if err := m.Logout(context.Background(), retried.Principal.Reference); err != nil {
		t.Fatal(err)
	}
	recoveredAfterLogout, err := passwordLogin(context.Background(), m, retried.Token, caller())
	if err != nil {
		t.Fatal("revoked cookie blocked reauthentication", err)
	}
	if _, err := m.Authenticate(request(retried.Token)); err == nil {
		t.Fatal("revoked credential recovered")
	}
	if _, err := m.Authenticate(request(recoveredAfterLogout.Token)); err != nil {
		t.Fatal(err)
	}
	fresh, err := passwordLogin(context.Background(), m, "", caller())
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(Lifetime)
	recovered, err := passwordLogin(context.Background(), m, fresh.Token, caller())
	if err != nil || !recovered.Principal.ExpiresAt.Equal(clock.Now().Add(Lifetime)) {
		t.Fatal("strong-auth expiry recovery failed", err)
	}
	if _, err := m.Authenticate(request(fresh.Token)); err == nil {
		t.Fatal("expired credential became active again")
	}
	if err := m.Logout(context.Background(), fresh.Principal.Reference); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Authenticate(request(recovered.Token)); err == nil {
		t.Fatal("expired-generation logout failed")
	}
	missing, err := passwordLogin(context.Background(), m, strings.Repeat("A", 43), caller())
	if err != nil || missing.Token == "" {
		t.Fatal("explicit missing-credential recovery failed", err)
	}
	s.mu.Lock()
	s.fail = errors.New("unavailable")
	s.mu.Unlock()
	if _, err := passwordLogin(context.Background(), m, missing.Token, caller()); err == nil {
		t.Fatal("store failure recovered as fresh login")
	}
	s.mu.Lock()
	s.fail = nil
	s.mu.Unlock()
	if _, err := passwordLogin(context.Background(), m, "malformed", caller()); err == nil {
		t.Fatal("malformed credential recovered")
	}
	invalid := caller()
	invalid.Subject = ""
	if _, err := passwordLogin(context.Background(), m, missing.Token, invalid); err == nil {
		t.Fatal("invalid identity replaced session")
	}
	m.random = bytes.NewReader(nil)
	if _, err := passwordLogin(context.Background(), m, missing.Token, caller()); err == nil {
		t.Fatal("entropy failure replaced session")
	}
}

func TestConcurrentLoginAndLogoutAcrossManagers(t *testing.T) {
	s := &memoryStore{rows: make(map[string]Record)}
	m, _ := fixture(t, s)
	other, _ := fixture(t, s)
	other.random = &counterReader{n: 100}
	first, err := m.Create(context.Background(), caller())
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Go(func() {
		if _, err := passwordLogin(context.Background(), other, first.Token, caller()); err != nil && apperrors.Normalize(err).Code != apperrors.ErrCodeUnauthorized {
			t.Error(err)
		}
	})
	wg.Go(func() {
		if err := m.Logout(context.Background(), first.Principal.Reference); err != nil {
			t.Error(err)
		}
	})
	wg.Wait()
	original := s.family(first.Principal.Reference)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, row := range s.rows {
		if row.Family == original && !row.Revoked {
			t.Fatal("concurrent relogin resurrected family")
		}
	}
}

func TestTerminalReloginFailsClosed(t *testing.T) {
	s := &memoryStore{rows: make(map[string]Record)}
	m, _ := fixture(t, s)
	first, err := m.Create(context.Background(), caller())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Logout(context.Background(), first.Principal.Reference); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.revokeFail = errors.New("unavailable")
	s.mu.Unlock()
	if _, err := passwordLogin(context.Background(), m, first.Token, caller()); apperrors.Normalize(err).Code != apperrors.ErrCodeServiceUnavailable {
		t.Fatal("terminal relogin ignored a failed family revocation", err)
	}
	s.mu.Lock()
	s.revokeFail = nil
	row := s.rows[first.Principal.Reference]
	row.Family = ""
	s.rows[first.Principal.Reference] = row
	s.mu.Unlock()
	if _, err := passwordLogin(context.Background(), m, first.Token, caller()); err == nil {
		t.Fatal("corrupt terminal record recovered")
	}
}

// tlsBrowser dials base over TLS while presenting the browser Origin the handler was configured with.
type tlsBrowser struct {
	t      *testing.T
	base   string
	client *http.Client
}

const tlsOrigin = "https://example.test"

func (b tlsBrowser) do(method, path, body, csrf string) (int, []http.Cookie, Response) {
	b.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r, err := http.NewRequestWithContext(ctx, method, b.base+path, strings.NewReader(body))
	if err != nil {
		b.t.Fatal(err)
	}
	r.Header.Set("Origin", tlsOrigin)
	r.Header.Set("Content-Type", "application/json")
	if csrf != "" {
		r.Header.Set("X-CSRF-Token", csrf)
	}
	w, err := b.client.Do(r)
	if err != nil {
		b.t.Fatal(err)
	}
	defer func() {
		if err := w.Body.Close(); err != nil {
			b.t.Error(err)
		}
	}()
	var document Response
	if w.StatusCode == http.StatusOK {
		if err := json.NewDecoder(io.LimitReader(w.Body, 4096)).Decode(&document); err != nil {
			b.t.Fatal(err)
		}
	}
	cookies := make([]http.Cookie, 0, 1)
	for _, c := range w.Cookies() {
		cookies = append(cookies, *c)
	}
	return w.StatusCode, cookies, document
}

func (b tlsBrowser) withJar(cookies []*http.Cookie) tlsBrowser {
	b.t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		b.t.Fatal(err)
	}
	u, err := url.Parse(b.base)
	if err != nil {
		b.t.Fatal(err)
	}
	jar.SetCookies(u, cookies)
	return tlsBrowser{t: b.t, base: b.base, client: &http.Client{Transport: b.client.Transport, Jar: jar, Timeout: 5 * time.Second}}
}

func (b tlsBrowser) cookies() []*http.Cookie {
	u, err := url.Parse(b.base)
	if err != nil {
		b.t.Fatal(err)
	}
	return b.client.Jar.Cookies(u)
}

// tlsHost serves the real handler over TLS. hold, when non-nil, pauses password verification until it is closed.
func tlsHost(t *testing.T, hold func() <-chan struct{}) tlsBrowser {
	t.Helper()
	m, _ := fixture(t, &memoryStore{rows: make(map[string]Record)})
	h, err := newLocalHandler(m, LoginVerifierFunc(func(ctx context.Context, _ Login) (auth.Principal, error) {
		if release := hold(); release != nil {
			select {
			case <-release:
			case <-ctx.Done():
				return auth.Principal{}, ctx.Err()
			}
		}
		return caller(), nil
	}), tlsOrigin, errorWriter)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(h)
	t.Cleanup(server.Close)
	return tlsBrowser{t: t, base: server.URL, client: server.Client()}.withJar(nil)
}

func TestHTTPSStaleCookieRecoversButLogoutDuringVerificationWins(t *testing.T) {
	t.Run("stale_cookie_recovers", func(t *testing.T) {
		browser := tlsHost(t, func() <-chan struct{} { return nil })
		code, _, first := browser.do(http.MethodPost, "/auth/login", `{"username":"u","password":"p"}`, "")
		if code != http.StatusOK {
			t.Fatal(code)
		}
		stale := browser.cookies()
		lost := browser.withJar(stale)
		if code, _, _ := lost.do(http.MethodPost, "/auth/logout", "", first.CSRFToken); code != http.StatusNoContent {
			t.Fatal("logout failed", code)
		}
		if code, cookies, _ := browser.do(http.MethodGet, "/auth/session", "", ""); code != http.StatusUnauthorized || len(cookies) != 0 {
			t.Fatal("revoked cookie stayed authenticated or status set a cookie")
		}
		if code, _, _ := browser.do(http.MethodPost, "/auth/login", `{"username":"u","password":"p"}`, ""); code != http.StatusOK {
			t.Fatal("stale cookie blocked reauthentication", code)
		}
		if fresh := browser.cookies(); len(fresh) != 1 || fresh[0].Value == stale[0].Value {
			t.Fatal("login did not replace the stale cookie")
		}
		if code, _, _ := browser.do(http.MethodGet, "/auth/session", "", ""); code != http.StatusOK {
			t.Fatal("recovered browser is not authenticated", code)
		}
		if code, _, _ := lost.do(http.MethodGet, "/auth/session", "", ""); code != http.StatusUnauthorized {
			t.Fatal("stale cookie regained access")
		}
	})
	t.Run("logout_during_verification_wins", func(t *testing.T) {
		var mu sync.Mutex
		var release chan struct{}
		entered := make(chan struct{}, 1)
		browser := tlsHost(t, func() <-chan struct{} {
			mu.Lock()
			defer mu.Unlock()
			if release != nil {
				entered <- struct{}{}
			}
			return release
		})
		code, _, first := browser.do(http.MethodPost, "/auth/login", `{"username":"u","password":"p"}`, "")
		if code != http.StatusOK {
			t.Fatal(code)
		}
		mu.Lock()
		release = make(chan struct{})
		mu.Unlock()
		type result struct {
			code    int
			cookies []http.Cookie
		}
		late := make(chan result, 1)
		go func() {
			code, cookies, _ := browser.do(http.MethodPost, "/auth/login", `{"username":"u","password":"p"}`, "")
			late <- result{code, cookies}
		}()
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("login never reached password verification")
		}
		if code, _, _ := browser.do(http.MethodPost, "/auth/logout", "", first.CSRFToken); code != http.StatusNoContent {
			t.Fatal("logout failed", code)
		}
		close(release)
		got := <-late
		if got.code != http.StatusUnauthorized || len(got.cookies) != 0 {
			t.Fatalf("late login installed a credential after logout: %d %d", got.code, len(got.cookies))
		}
		if code, _, _ := browser.do(http.MethodGet, "/auth/session", "", ""); code != http.StatusUnauthorized {
			t.Fatal("browser was signed back in after logout")
		}
	})
}
