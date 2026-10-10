package database

import (
	"context"
	"database/sql"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/kbukum/gokit/resilience"
)

type failingPreparedDialect struct {
	connects, closes *atomic.Int64
}

func (f failingPreparedDialect) Name() string { return "failed-apply" }
func (f failingPreparedDialect) Prepare(context.Context, ConnectionInput) (Opener, error) {
	return f, nil
}

func (f failingPreparedDialect) Open(context.Context) (gorm.Dialector, error) {
	return &failingOwnedDialector{pool: sql.OpenDB(countingConnector{connects: f.connects, closes: f.closes})}, nil
}

var errApplyFailure = errors.New("synthetic private initialization failure")

type failingOwnedDialector struct {
	gorm.Dialector
	pool *sql.DB
}

func (*failingOwnedDialector) Name() string             { return "failed-apply" }
func (*failingOwnedDialector) Apply(*gorm.Config) error { return errApplyFailure }
func (f *failingOwnedDialector) Close() error           { return f.pool.Close() }

func TestPreparedRetryClosesPoolsBeforeGORMInitialization(t *testing.T) {
	t.Parallel()
	var connects, closes atomic.Int64
	policy := resilience.NewPolicy().WithRetry(resilience.RetryConfig{
		MaxAttempts: 3, InitialBackoff: time.Millisecond, RetryIf: resilience.DefaultRetryIf,
	})
	db, err := NewWithContext(t.Context(), failingPreparedDialect{connects: &connects, closes: &closes},
		Config{DSN: "synthetic"}, testLogger(), WithConnectPolicy(policy))
	if db != nil || !errors.Is(err, errApplyFailure) || closes.Load() != 3 || connects.Load() != 0 {
		t.Fatalf("pre-initialization ownership lost: closes=%d connects=%d error=%v", closes.Load(), connects.Load(), err)
	}
}
