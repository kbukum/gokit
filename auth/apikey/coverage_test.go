package apikey_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kbukum/gokit/auth/apikey"
)

// errUnreachable is returned by validator stubs whose code paths the test
// asserts are not executed; keeping it sentinel keeps nilnil happy.
var errUnreachable = errors.New("unreachable")

// errNotFound is returned by store stubs when a record genuinely doesn't exist.
var errNotFound = errors.New("not found")

// ─── Generate / Hash ───────────────────────────────────────────────────────

func TestGenerate_ProducesUniqueKeysWithCorrectShape(t *testing.T) {
	t.Parallel()

	const prefix = "sk_live_"
	seen := map[string]bool{}
	for i := 0; i < 32; i++ {
		r, err := apikey.Generate(prefix)
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if !strings.HasPrefix(r.PlainKey, prefix) {
			t.Errorf("PlainKey %q lacks prefix %q", r.PlainKey, prefix)
		}
		// 16 random bytes hex-encoded → 32 hex chars after prefix.
		if got := len(r.PlainKey) - len(prefix); got != 32 {
			t.Errorf("hex body length: got %d want 32", got)
		}
		if r.KeyHash != apikey.Hash(r.PlainKey) {
			t.Errorf("KeyHash mismatch: got %q want %q", r.KeyHash, apikey.Hash(r.PlainKey))
		}
		// Display prefix is first 8 chars of plaintext (or shorter if total < 8).
		if r.Prefix != r.PlainKey[:8] {
			t.Errorf("Prefix: got %q want %q", r.Prefix, r.PlainKey[:8])
		}
		if seen[r.PlainKey] {
			t.Fatalf("duplicate key generated: %s", r.PlainKey)
		}
		seen[r.PlainKey] = true
	}
}

func TestGenerate_ShortPrefix_DisplayPrefixCappedAt8(t *testing.T) {
	t.Parallel()

	r, err := apikey.Generate("a") // 1 + 32 hex = 33 chars
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(r.Prefix) != 8 {
		t.Errorf("display prefix length: got %d want 8", len(r.Prefix))
	}
}

func TestGenerate_EmptyPrefix(t *testing.T) {
	t.Parallel()

	r, err := apikey.Generate("")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(r.PlainKey) != 32 {
		t.Errorf("PlainKey length: got %d want 32", len(r.PlainKey))
	}
}

func TestHash_Deterministic(t *testing.T) {
	t.Parallel()
	a := apikey.Hash("foo")
	b := apikey.Hash("foo")
	if a != b {
		t.Error("Hash must be deterministic")
	}
	if apikey.Hash("foo") == apikey.Hash("bar") {
		t.Error("different inputs should produce different hashes")
	}
	// SHA-256 hex is 64 chars.
	if len(apikey.Hash("anything")) != 64 {
		t.Errorf("hash length: got %d want 64", len(apikey.Hash("anything")))
	}
}

// ─── Key.IsExpiredPastGrace / Validate ────────────────────────────────────

func ptrTime(t time.Time) *time.Time { return &t }

func TestKey_IsExpiredPastGrace(t *testing.T) {
	t.Parallel()

	now := time.Now()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	cases := []struct {
		name string
		k    apikey.Key
		want bool
	}{
		{"no expiry, no grace", apikey.Key{IsActive: true}, false},
		{"future expiry, no grace", apikey.Key{ExpiresAt: &future}, false},
		{"past expiry, no grace", apikey.Key{ExpiresAt: &past}, true},
		{"past expiry, future grace", apikey.Key{ExpiresAt: &past, GraceEndsAt: &future}, false},
		{"past expiry, past grace", apikey.Key{ExpiresAt: &past, GraceEndsAt: &past}, true},
		{"no expiry, past grace", apikey.Key{GraceEndsAt: &past}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.k.IsExpiredPastGrace(); got != tc.want {
				t.Errorf("IsExpiredPastGrace = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()

	now := time.Now()
	past := now.Add(-time.Hour)

	t.Run("active and not expired", func(t *testing.T) {
		t.Parallel()
		if err := apikey.Validate(&apikey.Key{IsActive: true}); err != nil {
			t.Errorf("expected nil, got %v", err)
		}
	})
	t.Run("revoked", func(t *testing.T) {
		t.Parallel()
		err := apikey.Validate(&apikey.Key{IsActive: false})
		if err == nil || !strings.Contains(err.Error(), "revoked") {
			t.Errorf("got %v, want revoked error", err)
		}
	})
	t.Run("expired past grace", func(t *testing.T) {
		t.Parallel()
		err := apikey.Validate(&apikey.Key{IsActive: true, ExpiresAt: &past})
		if err == nil || !strings.Contains(err.Error(), "expired") {
			t.Errorf("got %v, want expired error", err)
		}
	})
}

// ─── Middleware / FromContext / Options ───────────────────────────────────

// fakeValidator is a configurable Validator.
type fakeValidator struct {
	mu      sync.Mutex
	calls   int
	gotKeys []string
	fn      func(ctx context.Context, plainKey string) (*apikey.Key, error)
}

func (f *fakeValidator) ValidateKey(ctx context.Context, plainKey string) (*apikey.Key, error) {
	f.mu.Lock()
	f.calls++
	f.gotKeys = append(f.gotKeys, plainKey)
	fn := f.fn
	f.mu.Unlock()
	return fn(ctx, plainKey)
}

// nextHandler reports whether it was called and surfaces the context key (if any).
type recordingHandler struct {
	called bool
	key    *apikey.Key
}

func (h *recordingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.called = true
	h.key = apikey.FromContext(r.Context())
	w.WriteHeader(http.StatusOK)
}

func TestMiddleware_NoHeader_PassesThrough(t *testing.T) {
	t.Parallel()
	v := &fakeValidator{fn: func(_ context.Context, _ string) (*apikey.Key, error) {
		t.Fatal("validator should not be called when header is absent")
		return nil, errUnreachable
	}}
	h := &recordingHandler{}
	m := apikey.Middleware(v)(h)

	req := httptest.NewRequest(http.MethodGet, "/x", http.NoBody)
	w := httptest.NewRecorder()
	m.ServeHTTP(w, req)

	if !h.called {
		t.Error("next handler not invoked")
	}
	if w.Code != http.StatusOK {
		t.Errorf("status: got %d want 200", w.Code)
	}
	if h.key != nil {
		t.Errorf("FromContext should be nil when no header, got %+v", h.key)
	}
}

func TestMiddleware_ValidKey_PopulatesContextAndCallsNext(t *testing.T) {
	t.Parallel()
	want := &apikey.Key{ID: "k1", OwnerID: "u1", IsActive: true}
	v := &fakeValidator{fn: func(_ context.Context, plainKey string) (*apikey.Key, error) {
		if plainKey != "secret" {
			t.Errorf("validator got %q want secret", plainKey)
		}
		return want, nil
	}}
	h := &recordingHandler{}
	m := apikey.Middleware(v)(h)

	req := httptest.NewRequest(http.MethodGet, "/x", http.NoBody)
	req.Header.Set("X-API-Key", "secret")
	w := httptest.NewRecorder()
	m.ServeHTTP(w, req)

	if !h.called {
		t.Fatal("next handler not invoked on valid key")
	}
	if h.key != want {
		t.Errorf("FromContext: got %+v want %+v", h.key, want)
	}
}

func TestMiddleware_InvalidKey_Returns401(t *testing.T) {
	t.Parallel()
	v := &fakeValidator{fn: func(_ context.Context, _ string) (*apikey.Key, error) {
		return nil, errors.New("bad")
	}}
	h := &recordingHandler{}
	m := apikey.Middleware(v)(h)

	req := httptest.NewRequest(http.MethodGet, "/x", http.NoBody)
	req.Header.Set("X-API-Key", "secret")
	w := httptest.NewRecorder()
	m.ServeHTTP(w, req)

	if h.called {
		t.Error("next handler must NOT be invoked on invalid key")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d want 401", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type: got %q want application/json", ct)
	}
	if !strings.Contains(w.Body.String(), "invalid or expired API key") {
		t.Errorf("body: got %q", w.Body.String())
	}
}

func TestMiddleware_WithHeader_OverridesDefault(t *testing.T) {
	t.Parallel()
	want := &apikey.Key{ID: "k1", IsActive: true}
	v := &fakeValidator{fn: func(_ context.Context, _ string) (*apikey.Key, error) { return want, nil }}
	h := &recordingHandler{}
	m := apikey.Middleware(v, apikey.WithHeader("X-Secret"))(h)

	// Default header ignored.
	req := httptest.NewRequest(http.MethodGet, "/x", http.NoBody)
	req.Header.Set("X-API-Key", "ignored")
	w := httptest.NewRecorder()
	m.ServeHTTP(w, req)
	if h.key != nil {
		t.Errorf("default header should be ignored when WithHeader is set; got key %+v", h.key)
	}

	// Custom header honored.
	h2 := &recordingHandler{}
	m2 := apikey.Middleware(v, apikey.WithHeader("X-Secret"))(h2)
	req2 := httptest.NewRequest(http.MethodGet, "/x", http.NoBody)
	req2.Header.Set("X-Secret", "real")
	w2 := httptest.NewRecorder()
	m2.ServeHTTP(w2, req2)
	if h2.key != want {
		t.Errorf("custom header not honored: got %+v want %+v", h2.key, want)
	}
}

func TestMiddleware_WithSkipPaths_BypassesValidator(t *testing.T) {
	t.Parallel()
	v := &fakeValidator{fn: func(_ context.Context, _ string) (*apikey.Key, error) {
		t.Fatal("validator should not run on skipped path")
		return nil, errUnreachable
	}}
	h := &recordingHandler{}
	m := apikey.Middleware(v, apikey.WithSkipPaths("/health", "/public/"))(h)

	for _, path := range []string{"/health", "/public/anything"} {
		req := httptest.NewRequest(http.MethodGet, path, http.NoBody)
		req.Header.Set("X-API-Key", "secret-but-ignored")
		w := httptest.NewRecorder()
		m.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("path %s: got status %d, want 200", path, w.Code)
		}
	}
	if v.calls != 0 {
		t.Errorf("validator should not be called for skipped paths, got %d calls", v.calls)
	}
}

// Path that does NOT match any skip prefix should still validate.
func TestMiddleware_WithSkipPaths_NonMatchingPath_StillValidates(t *testing.T) {
	t.Parallel()
	want := &apikey.Key{ID: "k1", IsActive: true}
	v := &fakeValidator{fn: func(_ context.Context, _ string) (*apikey.Key, error) { return want, nil }}
	h := &recordingHandler{}
	m := apikey.Middleware(v, apikey.WithSkipPaths("/health"))(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/x", http.NoBody)
	req.Header.Set("X-API-Key", "secret")
	w := httptest.NewRecorder()
	m.ServeHTTP(w, req)

	if v.calls != 1 {
		t.Errorf("validator calls: got %d want 1", v.calls)
	}
	if h.key != want {
		t.Errorf("context key: got %+v want %+v", h.key, want)
	}
}

func TestValidatorFunc_AdaptsToInterface(t *testing.T) {
	t.Parallel()
	called := false
	var v apikey.Validator = apikey.ValidatorFunc(func(_ context.Context, _ string) (*apikey.Key, error) {
		called = true
		return &apikey.Key{ID: "x"}, nil
	})
	got, err := v.ValidateKey(context.Background(), "k")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !called {
		t.Error("func not invoked")
	}
	if got.ID != "x" {
		t.Errorf("got %+v want id=x", got)
	}
}

func TestFromContext_NilWhenAbsent(t *testing.T) {
	t.Parallel()
	if got := apikey.FromContext(context.Background()); got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

// ─── Rotate ───────────────────────────────────────────────────────────────

// memStore is an in-memory apikey.Store for tests.
type memStore struct {
	mu      sync.Mutex
	byID    map[string]*apikey.Key
	getErr  error
	setErr  error
	getByID func(ctx context.Context, id string) (*apikey.Key, error)
}

func newMemStore() *memStore {
	return &memStore{byID: map[string]*apikey.Key{}}
}

func (s *memStore) Create(_ context.Context, k *apikey.Key) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byID[k.ID] = k
	return nil
}
func (s *memStore) GetByHash(_ context.Context, _ string) (*apikey.Key, error) {
	return nil, errNotFound
}
func (s *memStore) GetByID(ctx context.Context, id string) (*apikey.Key, error) {
	if s.getByID != nil {
		return s.getByID(ctx, id)
	}
	if s.getErr != nil {
		return nil, s.getErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	k, ok := s.byID[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return k, nil
}
func (s *memStore) UpdateLastUsed(_ context.Context, _ string) error { return nil }
func (s *memStore) SetGracePeriod(_ context.Context, id string, ends time.Time, by string) error {
	if s.setErr != nil {
		return s.setErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if k, ok := s.byID[id]; ok {
		k.GraceEndsAt = &ends
		k.RotatedByID = by
	}
	return nil
}
func (s *memStore) SetActive(_ context.Context, _ string, _ bool) error { return nil }
func (s *memStore) Delete(_ context.Context, _ string) error            { return nil }

func TestRotate_Success_AppliesDefaultGrace(t *testing.T) {
	t.Parallel()
	st := newMemStore()
	_ = st.Create(context.Background(), &apikey.Key{ID: "k1", IsActive: true})

	res, err := apikey.Rotate(context.Background(), st, "k1", apikey.RotationConfig{Prefix: "sk_"})
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	if res.OldKeyID != "k1" {
		t.Errorf("OldKeyID: got %q want k1", res.OldKeyID)
	}
	if res.NewKey.PlainKey == "" {
		t.Error("NewKey.PlainKey empty")
	}
	if !strings.HasPrefix(res.NewKey.PlainKey, "sk_") {
		t.Errorf("new key prefix: got %q", res.NewKey.PlainKey)
	}
	expectedGrace := time.Now().Add(apikey.DefaultGracePeriod)
	if delta := res.GraceEndsAt.Sub(expectedGrace); delta < -time.Minute || delta > time.Minute {
		t.Errorf("GraceEndsAt off default by %v", delta)
	}
	// store should now reflect the grace.
	old, _ := st.GetByID(context.Background(), "k1")
	if old.GraceEndsAt == nil {
		t.Error("old key GraceEndsAt not set on store")
	}
}

func TestRotate_Success_HonorsCustomGrace(t *testing.T) {
	t.Parallel()
	st := newMemStore()
	_ = st.Create(context.Background(), &apikey.Key{ID: "k1", IsActive: true})

	custom := 30 * time.Minute
	res, err := apikey.Rotate(context.Background(), st, "k1", apikey.RotationConfig{
		Prefix:      "sk_",
		GracePeriod: custom,
	})
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	delta := res.GraceEndsAt.Sub(time.Now().Add(custom))
	if delta < -time.Minute || delta > time.Minute {
		t.Errorf("GraceEndsAt off custom grace by %v", delta)
	}
}

func TestRotate_OldKeyNotFound_ErrorWraps(t *testing.T) {
	t.Parallel()
	st := newMemStore()
	st.getErr = errors.New("nope")

	_, err := apikey.Rotate(context.Background(), st, "missing", apikey.RotationConfig{})
	if err == nil || !strings.Contains(err.Error(), "old key not found") {
		t.Errorf("got %v, want old-key-not-found error", err)
	}
}

func TestRotate_OldKeyRevoked_RefusesRotation(t *testing.T) {
	t.Parallel()
	st := newMemStore()
	_ = st.Create(context.Background(), &apikey.Key{ID: "k1", IsActive: false})

	_, err := apikey.Rotate(context.Background(), st, "k1", apikey.RotationConfig{})
	if err == nil || !strings.Contains(err.Error(), "cannot rotate") {
		t.Errorf("got %v, want cannot-rotate error", err)
	}
}

func TestRotate_SetGracePeriodError_Propagates(t *testing.T) {
	t.Parallel()
	st := newMemStore()
	_ = st.Create(context.Background(), &apikey.Key{ID: "k1", IsActive: true})
	st.setErr = errors.New("db down")

	_, err := apikey.Rotate(context.Background(), st, "k1", apikey.RotationConfig{})
	if err == nil || !strings.Contains(err.Error(), "set grace period") {
		t.Errorf("got %v, want set-grace-period error", err)
	}
}
