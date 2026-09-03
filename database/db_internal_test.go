package database

import (
	"context"
	"testing"

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
