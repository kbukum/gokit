package apikey

import (
	"context"
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

var epoch = time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)

// faultStore injects failures in front of the real memory store.
type faultStore struct {
	Store
	getErr, createErr, rotateErr, revokeErr error
}

func (s *faultStore) GetByDigest(ctx context.Context, digest string) (*Key, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	return s.Store.GetByDigest(ctx, digest)
}

func (s *faultStore) GetByID(ctx context.Context, id string) (*Key, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	return s.Store.GetByID(ctx, id)
}

func (s *faultStore) Create(ctx context.Context, key *Key) error {
	if s.createErr != nil {
		return s.createErr
	}
	return s.Store.Create(ctx, key)
}

func (s *faultStore) Rotate(ctx context.Context, r Rotation) error {
	if s.rotateErr != nil {
		return s.rotateErr
	}
	return s.Store.Rotate(ctx, r)
}

func (s *faultStore) Revoke(ctx context.Context, id string, at time.Time) error {
	if s.revokeErr != nil {
		return s.revokeErr
	}
	return s.Store.Revoke(ctx, id, at)
}

func newMemory(t *testing.T) Store {
	t.Helper()
	store, err := NewMemoryStore(64)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func newTestManager(t *testing.T, store Store, clock util.Clock) *Manager {
	t.Helper()
	if store == nil {
		store = newMemory(t)
	}
	if clock == nil {
		clock = util.NewFakeClock(epoch)
	}
	m, err := NewManager(Config{Store: store, Hasher: testHasher(t), Clock: clock})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return m
}

func issue(t *testing.T, m *Manager, id string, expires *time.Time) *GenerateResult {
	t.Helper()
	issued, _, err := m.IssueKey(context.Background(), IssueRequest{KeyID: id, OwnerID: "owner", Name: "name", Prefix: "key", Scopes: []string{"read"}, ExpiresAt: expires})
	if err != nil {
		t.Fatalf("IssueKey(%s): %v", id, err)
	}
	return issued
}

func at(t time.Time) *time.Time { return &t }

func reason(err error) string {
	if app, ok := apperrors.AsAppError(err); ok {
		return app.Reason
	}
	return ""
}

func TestNewManagerRequiresEveryDependency(t *testing.T) {
	t.Parallel()
	hasher, store, clock := testHasher(t), newMemory(t), util.NewFakeClock(epoch)
	var nilStore *faultStore
	for name, cfg := range map[string]Config{
		"store":       {Hasher: hasher, Clock: clock},
		"typed store": {Store: nilStore, Hasher: hasher, Clock: clock},
		"hasher":      {Store: store, Clock: clock},
		"clock":       {Store: store, Hasher: hasher},
	} {
		if _, err := NewManager(cfg); apperrors.Normalize(err).Code != apperrors.ErrCodeInvalidInput {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
}

func TestValidateKeyAndKeyIDShareRules(t *testing.T) {
	t.Parallel()
	clock := util.NewFakeClock(epoch)
	m := newTestManager(t, nil, clock)
	issued := issue(t, m, "k1", at(epoch.Add(time.Minute)))

	byKey, err := m.ValidateKey(context.Background(), issued.PlainKey, "read")
	if err != nil {
		t.Fatalf("ValidateKey: %v", err)
	}
	byID, err := m.ValidateKeyID(context.Background(), "k1", "read")
	if err != nil || byID.KeyDigest != byKey.KeyDigest {
		t.Fatalf("ValidateKeyID = %+v, %v", byID, err)
	}
	for name, validate := range map[string]func(...string) error{
		"key": func(s ...string) error {
			_, err := m.ValidateKey(context.Background(), issued.PlainKey, s...)
			return err
		},
		"id": func(s ...string) error { _, err := m.ValidateKeyID(context.Background(), "k1", s...); return err },
	} {
		if err := validate("write"); reason(err) != "CREDENTIAL_CEILING" {
			t.Fatalf("%s scope escalation: %v", name, err)
		}
	}
	clock.Set(epoch.Add(time.Minute - time.Nanosecond))
	if _, err := m.ValidateKeyID(context.Background(), "k1"); err != nil {
		t.Fatalf("just before expiry: %v", err)
	}
	clock.Set(epoch.Add(time.Minute))
	for name, err := range map[string]error{
		"key": func() error { _, err := m.ValidateKey(context.Background(), issued.PlainKey); return err }(),
		"id":  func() error { _, err := m.ValidateKeyID(context.Background(), "k1"); return err }(),
	} {
		if reason(err) != "INVALID_CREDENTIAL" {
			t.Fatalf("%s at expiry: %v", name, err)
		}
	}
}

func TestValidateKeyIDRejectsBadInputAndSeparatesStoreFailure(t *testing.T) {
	t.Parallel()
	store := &faultStore{Store: newMemory(t)}
	m := newTestManager(t, store, nil)
	for _, id := range []string{"", "absent", strings.Repeat("x", 513)} {
		if _, err := m.ValidateKeyID(context.Background(), id); reason(err) != "INVALID_CREDENTIAL" {
			t.Fatalf("ValidateKeyID(%.10q): %v", id, err)
		}
	}
	store.getErr = errors.New("down")
	for name, err := range map[string]error{
		"key": func() error { _, err := m.ValidateKey(context.Background(), "key.secret"); return err }(),
		"id":  func() error { _, err := m.ValidateKeyID(context.Background(), "k1"); return err }(),
	} {
		if reason(err) != "AUTH_STORE_UNAVAILABLE" || !errors.Is(err, store.getErr) {
			t.Fatalf("%s store failure: %v", name, err)
		}
	}
}

func TestRevokeKeyIsImmediateAndOneWay(t *testing.T) {
	t.Parallel()
	m := newTestManager(t, nil, nil)
	issued := issue(t, m, "k1", nil)
	if err := m.RevokeKey(context.Background(), "k1"); err != nil {
		t.Fatalf("RevokeKey: %v", err)
	}
	if err := m.RevokeKey(context.Background(), "k1"); err != nil {
		t.Fatalf("repeated RevokeKey: %v", err)
	}
	if _, err := m.ValidateKey(context.Background(), issued.PlainKey); reason(err) != "INVALID_CREDENTIAL" {
		t.Fatalf("revoked key: %v", err)
	}
	if _, err := m.RotateKey(context.Background(), "k1", RotateRequest{NewKeyID: "k2"}); apperrors.Normalize(err).Code != apperrors.ErrCodeConflict {
		t.Fatalf("rotating a revoked key: %v", err)
	}
	if err := m.RevokeKey(context.Background(), "absent"); apperrors.Normalize(err).Code != apperrors.ErrCodeNotFound {
		t.Fatalf("revoking a missing key: %v", err)
	}
}

func TestIssueKeyValidation(t *testing.T) {
	t.Parallel()
	store := &faultStore{Store: newMemory(t)}
	m := newTestManager(t, store, nil)
	bad := map[string]IssueRequest{
		"prefix":       {KeyID: "k", OwnerID: "o", Prefix: "ab"},
		"key id":       {OwnerID: "o", Prefix: "key"},
		"owner":        {KeyID: "k", Prefix: "key"},
		"kind":         {KeyID: "k", OwnerID: "o", Prefix: "key", Kind: "robot"},
		"past expiry":  {KeyID: "k", OwnerID: "o", Prefix: "key", ExpiresAt: at(epoch)},
		"restrictions": {KeyID: "k", OwnerID: "o", Prefix: "key", RestrictionMode: auth.Restricted, Scopes: []string{""}},
	}
	for name, req := range bad {
		if _, _, err := m.IssueKey(context.Background(), req); err == nil {
			t.Fatalf("invalid %s accepted", name)
		}
	}
	store.createErr = errors.New("create failed")
	if _, _, err := m.IssueKey(context.Background(), IssueRequest{KeyID: "k", OwnerID: "o", Prefix: "key"}); !errors.Is(err, store.createErr) {
		t.Fatalf("create failure: %v", err)
	}
}

func TestValidationAbandonedByCallerIsNotCredentialDenial(t *testing.T) {
	t.Parallel()
	store := &faultStore{Store: newMemory(t)}
	m := newTestManager(t, store, nil)
	issued := issue(t, m, "k1", at(epoch.Add(time.Minute)))
	store.getErr = apperrors.NotFound("key", "k1")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for name, call := range map[string]func() error{
		"key": func() error { _, err := m.ValidateKey(ctx, issued.PlainKey); return err },
		"id":  func() error { _, err := m.ValidateKeyID(ctx, "k1"); return err },
	} {
		if got := apperrors.Normalize(call()); got.Code != apperrors.ErrCodeCanceled {
			t.Fatalf("%s: canceled validation classified as %v/%s", name, got.Code, got.Reason)
		}
	}
}

// misdirectedStore answers every lookup with one fixed key, as a store with a broken index would.
type misdirectedStore struct {
	Store
	key *Key
}

func (s *misdirectedStore) GetByDigest(context.Context, string) (*Key, error) {
	return s.key.Clone(), nil
}

func (s *misdirectedStore) GetByID(context.Context, string) (*Key, error) {
	return s.key.Clone(), nil
}

func TestValidateKeyRejectsMalformedKeysBeforeTheStore(t *testing.T) {
	t.Parallel()
	store := &faultStore{Store: newMemory(t), getErr: errors.New("store must not be asked")}
	m := newTestManager(t, store, nil)
	for _, plain := range []string{strings.Repeat("k", maxPlainBytes+1), "no-separator", "key.", ".secret", "k!.secret"} {
		if _, err := m.ValidateKey(context.Background(), plain); reason(err) != "INVALID_CREDENTIAL" {
			t.Fatalf("ValidateKey(%.12q): %v", plain, err)
		}
	}
}

func TestValidateRejectsAKeyTheStoreMisdirects(t *testing.T) {
	t.Parallel()
	m := newTestManager(t, nil, nil)
	issue(t, m, "k1", nil)
	stored, err := m.store.GetByID(context.Background(), "k1")
	if err != nil {
		t.Fatal(err)
	}
	misdirected := newTestManager(t, &misdirectedStore{Store: newMemory(t), key: stored}, nil)
	other := issue(t, m, "k2", nil)
	if _, err := misdirected.ValidateKey(context.Background(), other.PlainKey); reason(err) != "INVALID_CREDENTIAL" {
		t.Fatalf("a key whose digest differs was accepted: %v", err)
	}
	if _, err := misdirected.ValidateKeyID(context.Background(), "k2"); reason(err) != "INVALID_CREDENTIAL" {
		t.Fatalf("a key with another id was accepted: %v", err)
	}
}

func TestAuthenticateNeedsAValidAPIKey(t *testing.T) {
	t.Parallel()
	m := newTestManager(t, nil, nil)
	cookie := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	cookie.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: "session"})
	if _, err := m.Authenticate(cookie); reason(err) != "MISSING_CREDENTIAL" {
		t.Fatalf("a session cookie authenticated as an API key: %v", err)
	}
	unknown := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	unknown.Header.Set("X-API-Key", "key.unknown-secret")
	if _, err := m.Authenticate(unknown); reason(err) != "INVALID_CREDENTIAL" {
		t.Fatalf("an unknown key authenticated: %v", err)
	}
}
