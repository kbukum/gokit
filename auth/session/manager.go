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
	"github.com/kbukum/gokit/auth/lease"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/resilience"
	"github.com/kbukum/gokit/security"
	"github.com/kbukum/gokit/util"
)

const (
	Lifetime       = time.Hour
	LookupBudget   = 500 * time.Millisecond
	MutationBudget = 2 * time.Second
	// LoginBudget bounds a POST /auth/login, password verification included; a remote Backend's login call must
	// fit inside it.
	LoginBudget = 5 * time.Second
	Retention   = 10 * time.Minute
	WatchLimit  = 1024
)

// Config injects persistence, time, cryptographic randomness and independent secrets.
type Config struct {
	Store  Store
	Clock  util.Clock
	Random io.Reader
	Pepper string
	CSRF   *security.SignedCSRF
	// ReportError receives worker failures that have no caller: lease renewal and retained-generation cleanup.
	ReportError func(context.Context, error)
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
	leases     *lease.Set[watchKey]
	report     func(context.Context, error)
	mu         sync.Mutex
	closed     bool
	ctx        context.Context
	stop       context.CancelFunc
	done       chan struct{}
}

// NewManager starts explicitly owned stream lease and cleanup workers.
func NewManager(cfg Config) (*Manager, error) {
	if util.IsNil(cfg.Store) || util.IsNil(cfg.Clock) || util.IsNil(cfg.Random) || cfg.CSRF == nil || cfg.ReportError == nil {
		return nil, apperrors.InvalidInput("session", "Store, clock, randomness, CSRF and error reporter are required")
	}
	protection, err := apikey.NewHasher(apikey.HashingConfig{Pepper: cfg.Pepper, Domain: "session"})
	if err != nil {
		return nil, apperrors.InvalidInput("session", "Invalid credential protection configuration").WithCause(err)
	}
	gate, err := resilience.NewBulkhead(resilience.BulkheadConfig{Name: "session-revocation", MaxConcurrent: 1, MaxWait: MutationBudget + LookupBudget, MaxQueue: 256})
	if err != nil {
		return nil, err
	}
	ctx, stop := context.WithCancel(context.Background())
	m := &Manager{store: cfg.Store, clock: cfg.Clock, random: cfg.Random, protection: protection, csrf: cfg.CSRF, report: cfg.ReportError, ctx: ctx, stop: stop, done: make(chan struct{}), gate: gate}
	m.leases, err = lease.New(lease.Config[watchKey]{Checker: lease.CheckerFunc[watchKey](m.checkWatches), Lease: watchLease, Interval: watchInterval, Limit: WatchLimit, ReportError: cfg.ReportError})
	if err != nil {
		stop()
		return nil, err
	}
	go m.runCleanup()
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
	now := m.clock.Now()
	p.ExpiresAt = now.Add(Lifetime)
	if validationErr := p.Validate(); validationErr != nil {
		return Issued{}, validationErr
	}
	familyToken, err := m.newToken()
	if err != nil {
		return Issued{}, err
	}
	row := Record{
		Reference: p.Reference, Family: m.protection.Digest(familyToken), Generation: 1, Principal: p, ExpiresAt: p.ExpiresAt,
		RetainUntil: p.ExpiresAt.Add(Retention), AuthenticatedAt: now, Active: true,
	}
	if err := m.store.Create(ctx, row); err != nil {
		return Issued{}, storeFailure(ctx, err)
	}
	return Issued{Token: token, Principal: p.Clone()}, nil
}

// storeFailure classifies a store outcome. A finished ctx wins, so a caller that left or a spent budget is reported as
// CANCELED or TIMEOUT rather than as a credential decision or store outage.
func storeFailure(ctx context.Context, err error) error {
	if failure := apperrors.FromContext(ctx, "session"); failure != nil {
		return failure
	}
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
		return Record{}, storeFailure(ctx, err)
	}
	if ctx.Err() != nil {
		return Record{}, storeFailure(ctx, ctx.Err())
	}
	return m.validateRecord(ref, row)
}

func (m *Manager) validateRecord(ref string, row Record) (Record, error) {
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

// State is the authoritative record behind a protected session reference.
type State struct {
	Principal  auth.Principal
	Family     string
	Generation uint64
	ExpiresAt  time.Time
	// AuthenticatedAt is when the user last proved a credential for this family; zero means unknown.
	AuthenticatedAt time.Time
}

// Reference checks the session token grammar and returns its protected reference. It performs no lookup.
func (m *Manager) Reference(token string) (string, error) {
	if err := ValidateToken(token); err != nil {
		return "", err
	}
	return m.protection.Digest(token), nil
}

// Resolve validates a protected reference against authoritative storage and returns its current generation, family,
// absolute expiry and authentication time. It applies the same rules as Authenticate without CSRF, so only trusted callers that already
// hold a protected reference, such as an identity service checking a delegation, may use it.
func (m *Manager) Resolve(ctx context.Context, ref string) (State, error) {
	row, err := m.lookup(ctx, ref)
	if err != nil {
		return State{}, err
	}
	return State{Principal: row.Principal.Clone(), Family: row.Family, Generation: row.Generation, ExpiresAt: row.ExpiresAt, AuthenticatedAt: row.AuthenticatedAt}, nil
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
		token, csrfErr := ParseCSRFToken(r)
		if csrfErr != nil {
			return auth.Principal{}, csrfErr
		}
		if err := m.csrf.Verify(row.Reference, token); err != nil {
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
	next := Record{
		Reference: p.Reference, Family: row.Family, Generation: row.Generation + 1, Principal: p, ExpiresAt: row.ExpiresAt,
		RetainUntil: row.RetainUntil, AuthenticatedAt: row.AuthenticatedAt, Active: true,
	}
	if err := m.store.Rotate(ctx, ref, next); err != nil {
		return Issued{}, storeFailure(ctx, err)
	}
	m.endReference(ref)
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
		return storeFailure(ctx, err)
	}
	m.endFamily(family)
	return nil
}

// RevokeSubject revokes every session of one subject, for example after a password reset, and ends their streams. It
// returns how many session families it revoked; zero means the subject had none left to revoke.
func (m *Manager) RevokeSubject(ctx context.Context, kind auth.Kind, subject string) (int64, error) {
	if subject == "" || kind != auth.User && kind != auth.Service {
		return 0, apperrors.InvalidInput("subject", "Subject and kind are required")
	}
	ctx, cancel := context.WithTimeout(ctx, MutationBudget)
	defer cancel()
	return admitted(ctx, m, func() (int64, error) {
		if err := m.ready(); err != nil {
			return 0, err
		}
		count, err := m.store.RevokeSubject(ctx, kind, subject)
		if err != nil {
			return 0, storeFailure(ctx, err)
		}
		m.leases.Revoke(func(key watchKey) bool { return key.kind == kind && key.subject == subject })
		return count, nil
	})
}

// CSRFToken issues a token bound to a currently authoritative session generation.
func (m *Manager) CSRFToken(ctx context.Context, ref string) (string, error) {
	if _, err := m.lookup(ctx, ref); err != nil {
		return "", err
	}
	return m.csrf.Issue(ref)
}
