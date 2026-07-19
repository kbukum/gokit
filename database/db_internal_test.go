package database

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kbukum/gokit/logging"
)

func testLogger() *logging.Logger { return logging.NewDefault("test") }

func TestNewWithContextRejectsNonDialector(t *testing.T) {
	t.Parallel()
	cfg := Config{Enabled: true, DSN: ":memory:"}
	db, err := NewWithContext(context.Background(), "not-a-dialector", cfg, testLogger())
	if err == nil || db != nil {
		t.Fatalf("NewWithContext with non-dialector = db:%v err:%v, want failure", db, err)
	}
}

func TestContextSleepReturnsAfterDuration(t *testing.T) {
	t.Parallel()
	if err := contextSleep(context.Background(), time.Nanosecond); err != nil {
		t.Fatalf("contextSleep = %v, want nil", err)
	}
}

func TestContextSleepCanceled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := contextSleep(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("contextSleep = %v, want context.Canceled", err)
	}
}
