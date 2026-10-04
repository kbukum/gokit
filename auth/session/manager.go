package session

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/kbukum/gokit/auth"
	"github.com/kbukum/gokit/auth/apikey"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/resilience"
	"github.com/kbukum/gokit/security"
	"github.com/kbukum/gokit/util"
)

const (
	Lifetime       = time.Hour
	LookupBudget   = 500 * time.Millisecond
	MutationBudget = 2 * time.Second
	Retention      = 10 * time.Minute
	WatchLimit     = 1024
)

// Config injects persistence, time, cryptographic randomness and independent secrets.
type Config struct {
	Store  Store
	Clock  util.Clock
	Random io.Reader
	Pepper string
	CSRF   *security.SignedCSRF
}

// Manager owns its watch/cleanup workers. Composition must Close it before closing its store.
type Manager struct {
	store      Store
	clock      util.Clock
	timing     util.MonotonicClock
	random     io.Reader
	randomMu   sync.Mutex
	protection *apikey.Hasher
	csrf       *security.SignedCSRF
	gate       *resilience.Bulkhead
	mu         sync.Mutex
	watches    map[uint64]*watch
	sequence   uint64
	closed     bool
	ctx        context.Context
	stop       context.CancelFunc
	done       chan struct{}
	wake       chan struct{}
	started    chan struct{}
}

// NewManager starts explicitly owned fail-closed watch and cleanup workers.
func NewManager(cfg Config) (*Manager, error) {
	if util.IsNil(cfg.Store) || util.IsNil(cfg.Clock) || util.IsNil(cfg.Random) || cfg.CSRF == nil {
		return nil, apperrors.InvalidInput("session", "Store, clock, randomness and CSRF are required")
	}
	protection, err := apikey.NewHasher(apikey.HashingConfig{Pepper: cfg.Pepper, Domain: "session"})
	if err != nil {
		return nil, apperrors.InvalidInput("session", "Invalid credential protection configuration").WithCause(err)
	}
	ctx, stop := context.WithCancel(context.Background())
	m := &Manager{store: cfg.Store, clock: cfg.Clock, random: cfg.Random, protection: protection, csrf: cfg.CSRF, watches: make(map[uint64]*watch), ctx: ctx, stop: stop, done: make(chan struct{}), wake: make(chan struct{}, 1), started: make(chan struct{}), gate: resilience.NewBulkhead(resilience.BulkheadConfig{Name: "session-revocation", MaxConcurrent: 1, MaxWait: MutationBudget + LookupBudget})}
	go m.run()
	<-m.started
	return m, nil
}

// ValidateToken checks the exact canonical 32-byte base64url session grammar.
func ValidateToken(token string) error {
	if len(token) != 43 {
		return auth.Failure("SESSION_INVALID")
	}
	data, err := base64.RawURLEncoding.Strict().DecodeString(token)
	if err != nil || len(data) != 32 {
		return auth.Failure("SESSION_INVALID")
	}
	return nil
}

func (m *Manager) newToken() (string, error) {
	var data [32]byte
	m.randomMu.Lock()
	_, err := io.ReadFull(m.random, data[:])
	m.randomMu.Unlock()
	if err != nil {
		return "", apperrors.Internal(err)
	}
	return base64.RawURLEncoding.EncodeToString(data[:]), nil
}

func (m *Manager) ready() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return auth.Failure("SESSION_CLOSED")
	}
	return nil
}

// Create establishes a fresh one-hour family, without binding it to an application resource.
func (m *Manager) Create(ctx context.Context, p auth.Principal) (Issued, error) {
	ctx, cancel := context.WithTimeout(ctx, MutationBudget)
	defer cancel()
	return admitted(ctx, m, func() (Issued, error) { return m.create(ctx, p) })
}

func (m *Manager) create(ctx context.Context, p auth.Principal) (Issued, error) {
	if err := m.ready(); err != nil {
		return Issued{}, err
	}
	token, err := m.newToken()
	if err != nil {
		return Issued{}, err
	}
	p = p.Clone()
	p.Credential = auth.Session
	p.Reference = m.protection.Digest(token)
	p.ExpiresAt = m.clock.Now().Add(Lifetime)
	if validationErr := p.Validate(); validationErr != nil {
		return Issued{}, validationErr
	}
	familyToken, err := m.newToken()
	if err != nil {
		return Issued{}, err
	}
	row := Record{Reference: p.Reference, Family: m.protection.Digest(familyToken), Generation: 1, Principal: p, ExpiresAt: p.ExpiresAt, RetainUntil: p.ExpiresAt.Add(Retention), Active: true}
	if err := m.store.Create(ctx, row); err != nil {
		return Issued{}, storeFailure(err)
	}
	return Issued{Token: token, Principal: p.Clone()}, nil
}

func storeFailure(err error) error {
	if app, ok := apperrors.AsAppError(err); ok && app != nil && app.Code == apperrors.ErrCodeNotFound {
		return auth.Failure("SESSION_INVALID")
	}
	if app, ok := apperrors.AsAppError(err); ok && app != nil && app.Code == apperrors.ErrCodeUnauthorized && app.Reason == "SESSION_INVALID" {
		return auth.Failure("SESSION_INVALID")
	}
	return apperrors.New(apperrors.ErrCodeServiceUnavailable, "Authentication store unavailable").WithReason("AUTH_STORE_UNAVAILABLE").WithCause(err)
}

func (m *Manager) lookup(ctx context.Context, ref string) (Record, error) {
	if err := m.ready(); err != nil {
		return Record{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, LookupBudget)
	defer cancel()
	row, err := m.store.Lookup(ctx, ref)
	if err != nil {
		return Record{}, storeFailure(err)
	}
	if ctx.Err() != nil {
		return Record{}, storeFailure(ctx.Err())
	}
	if row.Reference != ref || row.Family == "" || row.Generation == 0 || !row.Active || row.Revoked || !m.clock.Now().Before(row.ExpiresAt) {
		return Record{}, auth.Failure("SESSION_INVALID")
	}
	p := row.Principal.Clone()
	p.Reference = row.Reference
	p.Credential = auth.Session
	p.ExpiresAt = row.ExpiresAt
	if err := p.Validate(); err != nil {
		return Record{}, err
	}
	row.Principal = p
	return row, nil
}

// Authenticate resolves a cookie and enforces synchronizer CSRF on every unsafe cookie request.
func (m *Manager) Authenticate(r *http.Request) (auth.Principal, error) {
	credential, err := auth.ParseCredentials(r)
	if err != nil {
		return auth.Principal{}, err
	}
	if credential.Kind != auth.Session {
		return auth.Principal{}, auth.Failure("MISSING_CREDENTIAL")
	}
	if tokenErr := ValidateToken(credential.Value); tokenErr != nil {
		return auth.Principal{}, tokenErr
	}
	row, err := m.lookup(r.Context(), m.protection.Digest(credential.Value))
	if err != nil {
		return auth.Principal{}, err
	}
	if unsafe(r.Method) {
		values := r.Header.Values("X-CSRF-Token")
		if len(values) != 1 {
			return auth.Principal{}, apperrors.New(apperrors.ErrCodeForbidden, "CSRF verification failed").WithReason("CSRF_INVALID")
		}
		if err := m.csrf.Verify(row.Reference, values[0]); err != nil {
			return auth.Principal{}, err
		}
	}
	return row.Principal.Clone(), nil
}

func unsafe(method string) bool { return method != "GET" && method != "HEAD" && method != "OPTIONS" }

// Rotate atomically replaces the active generation; no grace, retry or expiry extension is allowed.
func (m *Manager) Rotate(ctx context.Context, ref string, p auth.Principal) (Issued, error) {
	ctx, cancel := context.WithTimeout(ctx, MutationBudget)
	defer cancel()
	return admitted(ctx, m, func() (Issued, error) { return m.rotate(ctx, ref, p) })
}

func (m *Manager) rotate(ctx context.Context, ref string, p auth.Principal) (Issued, error) {
	row, err := m.lookup(ctx, ref)
	if err != nil {
		return Issued{}, err
	}
	if p.Subject != row.Principal.Subject || p.Kind != row.Principal.Kind {
		return Issued{}, auth.Failure("IDENTITY_CHANGED")
	}
	token, err := m.newToken()
	if err != nil {
		return Issued{}, err
	}
	p = p.Clone()
	p.Reference = m.protection.Digest(token)
	p.Credential = auth.Session
	p.ExpiresAt = row.ExpiresAt
	if err := p.Validate(); err != nil {
		return Issued{}, err
	}
	next := Record{Reference: p.Reference, Family: row.Family, Generation: row.Generation + 1, Principal: p, ExpiresAt: row.ExpiresAt, RetainUntil: row.RetainUntil, Active: true}
	if err := m.store.Rotate(ctx, ref, next); err != nil {
		return Issued{}, storeFailure(err)
	}
	m.cancelReference(ref)
	return Issued{Token: token, Principal: p.Clone()}, nil
}

// Logout revokes the family even when ref belongs to a rotated-away generation.
func (m *Manager) Logout(ctx context.Context, ref string) error {
	ctx, cancel := context.WithTimeout(ctx, MutationBudget)
	defer cancel()
	_, err := admitted(ctx, m, func() (struct{}, error) { return struct{}{}, m.logout(ctx, ref) })
	return err
}

func (m *Manager) logout(ctx context.Context, ref string) error {
	if err := m.ready(); err != nil {
		return err
	}
	family, err := m.store.Revoke(ctx, ref)
	if err != nil {
		return storeFailure(err)
	}
	m.cancelFamily(family)
	return nil
}

// CSRFToken issues a token bound to a currently authoritative session generation.
func (m *Manager) CSRFToken(ctx context.Context, ref string) (string, error) {
	if _, err := m.lookup(ctx, ref); err != nil {
		return "", err
	}
	return m.csrf.Issue(ref)
}
