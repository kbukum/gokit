package database

import (
	"context"
	"crypto/x509"
	"errors"
	"runtime"
	"sync/atomic"
	"testing"

	apperrors "github.com/kbukum/gokit/errors"
)

type sqlStateError string

func (e sqlStateError) Error() string    { return "server rejected connection " + string(e) }
func (e sqlStateError) SQLState() string { return string(e) }

func TestConnectDoesNotRetryPermanentFailures(t *testing.T) {
	t.Parallel()
	for name, fail := range map[string]error{
		"invalid password": sqlStateError("28P01"),
		"unknown database": sqlStateError("3D000"),
		"untrusted server": x509.UnknownAuthorityError{},
	} {
		var connects, closes atomic.Int64
		cfg := Config{Enabled: true, DSN: "counting"}
		cfg.ApplyDefaults()
		cfg.MaxRetries = 3
		if _, err := NewWithContext(context.Background(), countingDialector{connects: &connects, closes: &closes, fail: fail}, cfg, testLogger()); err == nil {
			t.Fatalf("%s: connected", name)
		}
		if got := connects.Load(); got != 1 {
			t.Errorf("%s: %d attempts, want 1", name, got)
		}
	}
}

func TestConnectRetriesSpentAttemptBudgetWithoutReportingCancellation(t *testing.T) {
	t.Parallel()
	var connects, closes atomic.Int64
	cfg := Config{Enabled: true, DSN: "counting", ConnectTimeout: "20ms"}
	cfg.ApplyDefaults()
	cfg.MaxRetries = 2
	_, err := NewWithContext(context.Background(), countingDialector{connects: &connects, closes: &closes, block: true}, cfg, testLogger())
	if got := connects.Load(); got != 2 {
		t.Fatalf("%d attempts, want 2: a spent attempt budget is transient", got)
	}
	if got := apperrors.Normalize(err).Code; got != apperrors.ErrCodeServiceUnavailable || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("exhausted attempt budgets classified as %v (%v)", got, err)
	}
}

func TestConnectCallerCancellationIsClassified(t *testing.T) {
	t.Parallel()
	var connects, closes atomic.Int64
	cfg := Config{Enabled: true, DSN: "counting", ConnectTimeout: "0"}
	cfg.ApplyDefaults()
	ctx, cancel := context.WithCancel(context.Background())
	connector := countingDialector{connects: &connects, closes: &closes, block: true}
	go func() {
		for connects.Load() == 0 {
			runtime.Gosched()
		}
		cancel()
	}()
	_, err := NewWithContext(ctx, connector, cfg, testLogger())
	if got := apperrors.Normalize(err).Code; got != apperrors.ErrCodeCanceled {
		t.Fatalf("caller cancellation classified as %v (%v)", got, err)
	}
}
