package session

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kbukum/gokit/auth"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/security"
)

func errorWriter(w http.ResponseWriter, r *http.Request, err error) {
	http.Error(w, apperrors.Normalize(err).Message, apperrors.Normalize(err).HTTPStatus())
}

func TestHTTPLogoutRotatedGenerationAndJSONFailures(t *testing.T) {
	m, _ := fixture(t, &memoryStore{rows: make(map[string]Record)})
	h, err := NewHandler(m, LoginVerifierFunc(func(context.Context, Login) (auth.Principal, error) { return caller(), nil }), "https://example.test", errorWriter)
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []struct{ origin, content, body string }{
		{"", "application/json", `{}`},
		{"https://evil.test", "application/json", `{}`},
		{"https://example.test", "text/plain", `{}`},
		{"https://example.test", "application/json", `{"username":"u","password":"p","extra":true}`},
		{"https://example.test", "application/json", `{"username":"u","password":"p"} {}`},
		{"https://example.test", "application/json", `{"username":"","password":"p"}`},
		{"https://example.test", "application/json", strings.Repeat("a", 4097)},
	} {
		r := httptest.NewRequest("POST", "https://example.test/auth/login", strings.NewReader(input.body))
		r.Header.Set("Origin", input.origin)
		r.Header.Set("Content-Type", input.content)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code < 400 || len(w.Result().Cookies()) != 0 {
			t.Fatal("invalid login succeeded", w.Code)
		}
	}
	issued, err := m.Create(context.Background(), caller())
	if err != nil {
		t.Fatal(err)
	}
	csrf, err := m.CSRFToken(context.Background(), issued.Principal.Reference)
	if err != nil {
		t.Fatal(err)
	}
	next, err := m.Rotate(context.Background(), issued.Principal.Reference, caller())
	if err != nil {
		t.Fatal(err)
	}
	r := request(issued.Token)
	r.Method = "POST"
	r.URL.Path = "/auth/logout"
	r.Header.Set("X-CSRF-Token", csrf)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 204 || len(w.Result().Cookies()) != 1 || w.Result().Cookies()[0].MaxAge != -1 {
		t.Fatal("logout failed", w.Code, w.Body.String())
	}
	if _, err := m.Authenticate(request(next.Token)); err == nil {
		t.Fatal("old logout failed to revoke replacement")
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 204 {
		t.Fatal("logout not idempotent")
	}
	r = httptest.NewRequest("GET", "https://example.test/auth/session", http.NoBody)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 401 || w.Header().Get("Set-Cookie") != "" {
		t.Fatal("status failure renewed cookie")
	}
	for _, origin := range []string{"http://example.test", "https://example.test/", "https://u@example.test", "https://example.test?q=x"} {
		if _, err := NewHandler(m, LoginVerifierFunc(nil), origin, errorWriter); err == nil {
			t.Fatal("invalid origin accepted")
		}
	}
}

func TestHTTPSessionResponseAndUnsafeCSRF(t *testing.T) {
	m, _ := fixture(t, &memoryStore{rows: make(map[string]Record)})
	h, _ := NewHandler(m, LoginVerifierFunc(func(context.Context, Login) (auth.Principal, error) { return caller(), nil }), "https://example.test", errorWriter)
	r := httptest.NewRequest("POST", "https://example.test/auth/login", strings.NewReader(`{"username":"u","password":"p"}`))
	r.Header.Set("Origin", "https://example.test")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	var out Response
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Status != "authenticated" || out.Identity.Subject != "u" || out.Identity.Reference != "" || out.Identity.Credential != "" || out.CSRFToken == "" {
		t.Fatal("response leak", out)
	}
	issued := w.Result().Cookies()[0].Value
	for _, method := range []string{"POST", "PUT", "DELETE", "PATCH"} {
		r = request(issued)
		r.Method = method
		if _, err := m.Authenticate(r); err == nil {
			t.Fatal("unsafe request accepted")
		}
		r.Header.Set("X-CSRF-Token", out.CSRFToken)
		if _, err := m.Authenticate(r); err != nil {
			t.Fatal(err)
		}
		r.Header.Add("X-CSRF-Token", out.CSRFToken)
		if _, err := m.Authenticate(r); err == nil {
			t.Fatal("duplicate CSRF accepted")
		}
	}
}

func TestMutationFailuresCancellationAndLostResponse(t *testing.T) {
	s := &memoryStore{rows: make(map[string]Record)}
	m, _ := fixture(t, s)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.Create(ctx, caller()); err == nil {
		t.Fatal("canceled login accepted")
	}
	issued, err := m.Create(context.Background(), caller())
	if err != nil {
		t.Fatal(err)
	}
	p := caller()
	p.Restrictions = auth.Restrictions{Mode: auth.Restricted, Resources: []string{"one"}, Scopes: []string{"read"}}
	next, err := m.Rotate(context.Background(), issued.Principal.Reference, p)
	if err != nil {
		t.Fatal(err)
	}
	// The replacement committed, but its response is lost: retrying the old generation is forbidden.
	if _, err := m.Rotate(context.Background(), issued.Principal.Reference, p); err == nil {
		t.Fatal("lost rotation resurrected old credential")
	}
	got, err := m.Authenticate(request(next.Token))
	if err != nil || got.Allows("two", "read") {
		t.Fatal(err)
	}
	p.Subject = "another"
	if _, err := m.Rotate(context.Background(), next.Principal.Reference, p); err == nil {
		t.Fatal("identity mutation accepted")
	}
	s.mu.Lock()
	s.fail = errors.New("secret SQL")
	s.mu.Unlock()
	if err := m.Logout(context.Background(), next.Principal.Reference); err == nil {
		t.Fatal("failed revocation claimed success")
	}
	if _, err := m.Rotate(context.Background(), next.Principal.Reference, caller()); err == nil {
		t.Fatal("store failure accepted")
	}
	if _, err := m.Cleanup(context.Background()); err == nil {
		t.Fatal("cleanup failure ignored")
	}
	s.mu.Lock()
	s.fail = nil
	s.mu.Unlock()
	if err := m.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Create(context.Background(), caller()); err == nil {
		t.Fatal("closed manager created session")
	}
	if _, _, err := m.Acquire(context.Background(), next.Principal.Reference); err == nil {
		t.Fatal("closed manager acquired watch")
	}
	if _, err := m.Cleanup(context.Background()); err == nil {
		t.Fatal("closed cleanup")
	}
}

func TestConcurrentRotateAndLogout(t *testing.T) {
	s := &memoryStore{rows: make(map[string]Record)}
	m, _ := fixture(t, s)
	other, _ := fixture(t, s)
	issued, err := m.Create(context.Background(), caller())
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Go(func() {
		_, err := m.Rotate(context.Background(), issued.Principal.Reference, caller())
		if err != nil && apperrors.Normalize(err).Code != apperrors.ErrCodeUnauthorized {
			t.Error(err)
		}
	})
	wg.Go(func() {
		if err := other.Logout(context.Background(), issued.Principal.Reference); err != nil {
			t.Error(err)
		}
	})
	wg.Wait()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, row := range s.rows {
		if !row.Revoked {
			t.Fatal("rotation resurrected revoked family")
		}
	}
}

func TestBoundsAndEntropyFailures(t *testing.T) {
	s := &memoryStore{rows: make(map[string]Record)}
	m, clock := fixture(t, s)
	if _, err := NewManager(Config{}); err == nil {
		t.Fatal("missing config")
	}

	csrf, _ := security.NewSignedCSRF(bytes.Repeat([]byte{1}, 32), bytes.NewReader(nil))
	if _, err := NewManager(Config{Store: s, Clock: clock, Random: bytes.NewReader(nil), Pepper: "short", CSRF: csrf}); err == nil {
		t.Fatal("weak pepper")
	}
	empty, err := NewManager(Config{Store: s, Clock: clock, Random: bytes.NewReader(nil), Pepper: strings.Repeat("p", 32), CSRF: csrf})
	if err != nil {
		t.Fatal(err)
	}
	defer empty.Close(context.Background())
	if _, err := empty.Create(context.Background(), caller()); err == nil {
		t.Fatal("entropy failure ignored")
	}
	invalid := caller()
	invalid.Kind = ""
	if _, err := m.Create(context.Background(), invalid); err == nil {
		t.Fatal("invalid identity")
	}
	for _, token := range []string{"", strings.Repeat("a", 42), strings.Repeat("!", 43), strings.Repeat("a", 42) + "B"} {
		if ValidateToken(token) == nil {
			t.Fatal("invalid token grammar")
		}
	}
	issued, err := m.Create(context.Background(), caller())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := m.Acquire(ctx, issued.Principal.Reference); err == nil {
		t.Fatal("canceled acquisition")
	}
	releases := make([]func(), 0, WatchLimit)
	for range WatchLimit {
		_, release, err := m.Acquire(context.Background(), issued.Principal.Reference)
		if err != nil {
			t.Fatal(err)
		}
		releases = append(releases, release)
	}
	if _, _, err := m.Acquire(context.Background(), issued.Principal.Reference); err == nil {
		t.Fatal("capacity unbounded")
	}
	for _, release := range releases {
		release()
		release()
	}
	m.Poll(ctx)
	clock.Advance(time.Hour + Retention)
	if n, err := m.Cleanup(context.Background()); err != nil || n > 1 {
		t.Fatal(n, err)
	}
	s.mu.Lock()
	remaining := len(s.rows)
	s.mu.Unlock()
	if remaining != 0 {
		t.Fatal("due row retained")
	}
}
