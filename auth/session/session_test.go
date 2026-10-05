package session

import (
	"bytes"
	"context"
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
	"github.com/kbukum/gokit/util"
)

type counterReader struct{ n uint64 }

func (r *counterReader) Read(p []byte) (int, error) {
	r.n++
	for i := range p {
		p[i] = byte(r.n >> ((i % 8) * 8))
	}
	return len(p), nil
}

type memoryStore struct {
	mu         sync.Mutex
	rows       map[string]Record
	fail       error
	revokeFail error
}

func (s *memoryStore) family(ref string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rows[ref].Family
}

func (s *memoryStore) Create(ctx context.Context, row Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail != nil {
		return s.fail
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.rows[row.Reference] = row
	return nil
}

func (s *memoryStore) Lookup(ctx context.Context, ref string) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail != nil {
		return Record{}, s.fail
	}
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}
	row, ok := s.rows[ref]
	if !ok {
		return Record{}, apperrors.New(apperrors.ErrCodeNotFound, "Session not found")
	}
	return row, nil
}

func (s *memoryStore) Rotate(ctx context.Context, old string, next Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail != nil {
		return s.fail
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	row, ok := s.rows[old]
	if !ok || !row.Active || row.Revoked || !next.ExpiresAt.Equal(row.ExpiresAt) {
		return auth.Failure("SESSION_INVALID")
	}
	row.Active = false
	s.rows[old] = row
	s.rows[next.Reference] = next
	return nil
}

func (s *memoryStore) Relogin(ctx context.Context, old string, next Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail != nil {
		return s.fail
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	row, ok := s.rows[old]
	if !ok || !row.Active || row.Revoked || row.Family != next.Family || row.Generation+1 != next.Generation {
		return auth.Failure("SESSION_INVALID")
	}
	for ref := range s.rows {
		previous := s.rows[ref]
		if previous.Family == row.Family {
			previous.RetainUntil = next.RetainUntil
			s.rows[ref] = previous
		}
	}
	row.Active = false
	row.RetainUntil = next.RetainUntil
	s.rows[old] = row
	s.rows[next.Reference] = next
	return nil
}

func (s *memoryStore) Revoke(ctx context.Context, ref string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail != nil {
		return "", s.fail
	}
	if s.revokeFail != nil {
		return "", s.revokeFail
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	row, ok := s.rows[ref]
	if !ok {
		return "", auth.Failure("SESSION_INVALID")
	}
	for key := range s.rows {
		r := s.rows[key]
		if r.Family == row.Family {
			r.Revoked = true
			s.rows[key] = r
		}
	}
	return row.Family, nil
}

func (s *memoryStore) Cleanup(ctx context.Context, before time.Time, limit int) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail != nil {
		return 0, s.fail
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	var n int64
	for key := range s.rows {
		row := s.rows[key]
		if !row.RetainUntil.After(before) && n < int64(limit) {
			delete(s.rows, key)
			n++
		}
	}
	return n, nil
}

func fixture(t *testing.T, store *memoryStore) (*Manager, *util.FakeClock) {
	t.Helper()
	clock := util.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	csrf, err := security.NewSignedCSRF(bytes.Repeat([]byte{3}, 32), bytes.NewReader(bytes.Repeat([]byte{4}, 65536)))
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(Config{Store: store, Clock: clock, Random: &counterReader{}, Pepper: strings.Repeat("p", 32), CSRF: csrf})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := m.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	return m, clock
}

func caller() auth.Principal {
	return auth.Principal{Subject: "u", Kind: auth.User, Restrictions: auth.Restrictions{Mode: auth.Unrestricted}}
}

func request(token string) *http.Request {
	r := httptest.NewRequest("GET", "https://example.test/", http.NoBody)
	r.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: token})
	return r
}

func TestLifecycleRotationOldLogoutAndStreams(t *testing.T) {
	s := &memoryStore{rows: make(map[string]Record)}
	m, clock := fixture(t, s)
	issued, err := m.Create(context.Background(), caller())
	if err != nil || len(issued.Token) != 43 || !issued.Principal.ExpiresAt.Equal(clock.Now().Add(time.Hour)) {
		t.Fatal(err)
	}
	p, err := m.Authenticate(request(issued.Token))
	if err != nil {
		t.Fatal(err)
	}
	lifetime, release, err := m.Acquire(context.Background(), p.Reference)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	next, err := m.Rotate(context.Background(), p.Reference, caller())
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-lifetime.Done():
	default:
		t.Fatal("rotation retained stream")
	}
	if _, err := m.Authenticate(request(issued.Token)); err == nil {
		t.Fatal("old accepted")
	}
	if _, err := m.Authenticate(request(next.Token)); err != nil {
		t.Fatal(err)
	}
	if err := m.Logout(context.Background(), p.Reference); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Authenticate(request(next.Token)); err == nil {
		t.Fatal("replacement resurrected")
	}
}

func TestExpiryStoreFailureAndCrossInstance(t *testing.T) {
	s := &memoryStore{rows: make(map[string]Record)}
	m, clock := fixture(t, s)
	other, _ := fixture(t, s)
	issued, err := m.Create(context.Background(), caller())
	if err != nil {
		t.Fatal(err)
	}
	life, release, err := other.Acquire(context.Background(), issued.Principal.Reference)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if err := m.Logout(context.Background(), issued.Principal.Reference); err != nil {
		t.Fatal(err)
	}
	other.Poll(context.Background())
	select {
	case <-life.Done():
	default:
		t.Fatal("remote revocation retained stream")
	}
	issued, err = m.Create(context.Background(), caller())
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Hour)
	if _, err = m.Authenticate(request(issued.Token)); err == nil {
		t.Fatal("expiry accepted")
	}
	clock.Advance(-time.Hour)
	s.mu.Lock()
	s.fail = errors.New("private database failure")
	s.mu.Unlock()
	if _, err = m.Authenticate(request(issued.Token)); err == nil {
		t.Fatal("store failure accepted")
	}
}

func TestHTTPProtocol(t *testing.T) {
	m, _ := fixture(t, &memoryStore{rows: make(map[string]Record)})
	handler, err := newLocalHandler(m, LoginVerifierFunc(func(context.Context, Login) (auth.Principal, error) { return caller(), nil }), "https://example.test", func(w http.ResponseWriter, r *http.Request, err error) {
		http.Error(w, "failure", http.StatusUnauthorized)
	})
	if err != nil {
		t.Fatal(err)
	}
	login := httptest.NewRequest("POST", "https://example.test/auth/login", strings.NewReader(`{"username":"u","password":"p"}`))
	login.Header.Set("Origin", "https://example.test")
	login.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, login)
	if w.Code != 200 || len(w.Result().Cookies()) != 1 {
		t.Fatalf("login: %d %s", w.Code, w.Body.String())
	}
	cookie := w.Result().Cookies()[0]
	if cookie.Name != auth.SessionCookie || !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/" || cookie.Domain != "" {
		t.Fatal("cookie flags")
	}
	r := request(cookie.Value)
	r.URL.Path = "/auth/session"
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 200 || w.Header().Get("Set-Cookie") != "" {
		t.Fatal("status renewed cookie")
	}
	r.Method = "POST"
	r.URL.Path = "/auth/logout"
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("logout without CSRF")
	}
}

func FuzzSessionToken(f *testing.F) {
	f.Add(strings.Repeat("a", 43))
	f.Fuzz(func(t *testing.T, token string) { _ = ValidateToken(token) })
}
