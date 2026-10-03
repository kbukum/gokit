package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/kbukum/gokit/database/migration"
)

type failingResetBackend struct {
	migrationBackend
	failure error
}

func (b failingResetBackend) Drop(ctx context.Context, tx *sql.Tx) error {
	if err := b.migrationBackend.Drop(ctx, tx); err != nil {
		return err
	}
	return b.failure
}

func TestFailedResetRestoresSchemaAndConstraintTiming(t *testing.T) {
	t.Parallel()
	pool, err := sql.Open("sqlite3", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := pool.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, err := pool.ExecContext(t.Context(), "CREATE TABLE retained(id INTEGER PRIMARY KEY); INSERT INTO retained VALUES (1)"); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("reset aborted")
	driver, err := migration.NewSQLDriver(t.Context(), pool, failingResetBackend{failure: failure})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := driver.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := driver.Lock(); err != nil {
		t.Fatal(err)
	}
	if err := driver.Drop(); !errors.Is(err, failure) {
		t.Fatalf("expected reset failure, got %v", err)
	}
	if err := driver.Close(); err != nil {
		t.Fatal(err)
	}
	var count, deferred int
	if err := pool.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM retained").Scan(&count); err != nil || count != 1 {
		t.Fatalf("reset failed to restore data: %d, %v", count, err)
	}
	if err := pool.QueryRowContext(t.Context(), "PRAGMA defer_foreign_keys").Scan(&deferred); err != nil || deferred != 0 {
		t.Fatalf("reset leaked constraint deferral: %d, %v", deferred, err)
	}
}
