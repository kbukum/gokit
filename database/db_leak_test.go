package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/kbukum/gokit/resilience"
)

// errConnectRefused is returned by the instrumented connector so every ping fails, driving
// connectOnce down its pool-cleanup branch.
var errConnectRefused = errors.New("connect refused")

// countingConnector instruments the connection path without a real driver: it counts Connect and
// Close calls, optionally blocks in Connect until the context is done (to exercise timeouts), and
// always fails so connectOnce reaches its pool-cleanup branch. database/sql invokes Connector.Close
// when its *sql.DB is closed, so closes records exactly how many pools connectOnce released.
type countingConnector struct {
	connects *atomic.Int64
	closes   *atomic.Int64
	block    bool
}

func (c countingConnector) Connect(ctx context.Context) (driver.Conn, error) {
	c.connects.Add(1)
	if c.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return nil, errConnectRefused
}

func (c countingConnector) Driver() driver.Driver { return countingDriver{} }

func (c countingConnector) Close() error {
	c.closes.Add(1)
	return nil
}

type countingDriver struct{}

func (countingDriver) Open(string) (driver.Conn, error) { return nil, errConnectRefused }

// countingDialector is a minimal gorm.Dialector whose pool is backed by countingConnector. Only
// Name and Initialize are exercised by the connection path; the embedded nil interface satisfies
// the rest of gorm.Dialector, none of which connectOnce calls.
type countingDialector struct {
	gorm.Dialector
	connects *atomic.Int64
	closes   *atomic.Int64
	block    bool
}

func (countingDialector) Name() string { return "counting" }

func (d countingDialector) Initialize(db *gorm.DB) error {
	db.ConnPool = sql.OpenDB(countingConnector{connects: d.connects, closes: d.closes, block: d.block})
	return nil
}

// TestConnectClosesPoolOnEachFailedAttempt is the regression guard for the pool leak: with GORM's
// automatic ping disabled, gorm.Open builds the *sql.DB lazily and succeeds even when the server is
// unreachable, so each failed attempt must close its pool at PingContext time. The instrumented
// dialector counts one close per attempt; a regression that stops closing failed pools drops the
// count and fails here.
func TestConnectClosesPoolOnEachFailedAttempt(t *testing.T) {
	t.Parallel()

	var connects, closes atomic.Int64
	cfg := Config{Enabled: true, DSN: "counting", MaxRetries: 3}
	cfg.ApplyDefaults()
	cfg.MaxRetries = 3

	db, err := NewWithContext(context.Background(), countingDialector{connects: &connects, closes: &closes}, cfg, testLogger())
	if err == nil || db != nil {
		t.Fatalf("NewWithContext = db:%v err:%v, want connection failure", db, err)
	}
	if got := closes.Load(); got != int64(cfg.MaxRetries) {
		t.Fatalf("pool closes = %d, want %d (one per failed attempt)", got, cfg.MaxRetries)
	}
}

// TestConnectUsesInjectedPolicyAttempts verifies WithConnectPolicy is honored: a policy capped at
// two attempts must produce exactly two connection attempts, independent of Config.MaxRetries.
func TestConnectUsesInjectedPolicyAttempts(t *testing.T) {
	t.Parallel()

	var connects, closes atomic.Int64
	cfg := Config{Enabled: true, DSN: "counting", MaxRetries: 9}
	cfg.ApplyDefaults()
	cfg.MaxRetries = 9

	policy := resilience.NewPolicy().WithRetry(resilience.RetryConfig{
		MaxAttempts:    2,
		InitialBackoff: time.Millisecond,
		RetryIf:        resilience.DefaultRetryIf,
	})

	db, err := NewWithContext(context.Background(), countingDialector{connects: &connects, closes: &closes}, cfg, testLogger(), WithConnectPolicy(policy))
	if err == nil || db != nil {
		t.Fatalf("NewWithContext = db:%v err:%v, want connection failure", db, err)
	}
	if got := connects.Load(); got != 2 {
		t.Fatalf("connect attempts = %d, want 2 (from injected policy)", got)
	}
}

// TestConnectInjectedPolicyTimeoutAbortsBlockedAttempt verifies a policy timeout bounds a blocked
// connection: the connector blocks until its context is canceled, and the policy's timeout must
// cancel it and surface a deadline error rather than hang.
func TestConnectInjectedPolicyTimeoutAbortsBlockedAttempt(t *testing.T) {
	t.Parallel()

	var connects, closes atomic.Int64
	// Disable the per-attempt Config bound so the policy timeout is the sole deadline under test.
	cfg := Config{Enabled: true, DSN: "counting", MaxRetries: 1, ConnectTimeout: "0"}
	cfg.ApplyDefaults()
	cfg.MaxRetries = 1
	cfg.ConnectTimeout = "0"

	policy := resilience.NewPolicy().
		WithRetry(resilience.RetryConfig{MaxAttempts: 1, RetryIf: resilience.DefaultRetryIf}).
		WithTimeout(50 * time.Millisecond)

	dialector := countingDialector{connects: &connects, closes: &closes, block: true}
	db, err := NewWithContext(context.Background(), dialector, cfg, testLogger(), WithConnectPolicy(policy))
	if err == nil || db != nil {
		t.Fatalf("NewWithContext = db:%v err:%v, want timeout failure", db, err)
	}
	if connects.Load() == 0 {
		t.Fatal("expected the blocked connector to have been dialed at least once")
	}
}

// TestConnectCanceledBeforeFirstAttempt verifies an already-canceled context aborts before any
// connection attempt is made.
func TestConnectCanceledBeforeFirstAttempt(t *testing.T) {
	t.Parallel()

	var connects, closes atomic.Int64
	cfg := Config{Enabled: true, DSN: "counting"}
	cfg.ApplyDefaults()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	db, err := NewWithContext(ctx, countingDialector{connects: &connects, closes: &closes}, cfg, testLogger())
	if err == nil || db != nil {
		t.Fatalf("NewWithContext = db:%v err:%v, want cancellation", db, err)
	}
	if connects.Load() != 0 {
		t.Fatalf("connect attempts = %d, want 0 before a canceled context", connects.Load())
	}
}
