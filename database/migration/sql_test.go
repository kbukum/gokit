package migration

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"
	"testing/synctest"
)

// A canceled transaction on a pinned connection must end on the caller's goroutine. If database/sql rolled it back from its own cancellation goroutine, that goroutine would also close the connection while its owner, which still has to unlock it, was using it.
func TestCanceledTransactionRollsBackBeforeReturning(t *testing.T) {
	t.Parallel()
	exec := func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "SELECT 1")
		return err
	}
	tests := []struct {
		name string
		run  func(context.Context, *sql.DB) (func() error, func() error, error)
	}{
		{"driver run", func(ctx context.Context, db *sql.DB) (func() error, func() error, error) {
			d, err := NewSQLDriver(ctx, db, nopBackend{}, Table{Name: DefaultVersionTable})
			if err != nil {
				return nil, nil, err
			}
			return func() error { return d.Run(strings.NewReader("SELECT 1")) }, d.Close, nil
		}},
		{"session transaction", func(ctx context.Context, db *sql.DB) (func() error, func() error, error) {
			s, err := Begin(ctx, db, nopBackend{})
			if err != nil {
				return nil, nil, err
			}
			return func() error { return s.Transaction(ctx, exec) }, s.Close, nil
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				gate := &gatedDriver{release: make(chan struct{})}
				db := sql.OpenDB(gate)
				defer func() { _ = db.Close() }()
				ctx, cancel := context.WithCancel(t.Context())
				run, closeFn, err := tt.run(ctx, db)
				if err != nil {
					t.Fatal(err)
				}
				done := make(chan error, 1)
				go func() { done <- run() }()
				synctest.Wait()
				cancel()
				synctest.Wait()
				select {
				case err := <-done:
					t.Errorf("returned (%v) while its rollback was still running on another goroutine", err)
				default:
				}
				close(gate.release)
				if err := <-done; !errors.Is(err, context.Canceled) {
					t.Fatalf("got %v, want context.Canceled", err)
				}
				if err := closeFn(); err != nil {
					t.Fatal(err)
				}
			})
		})
	}
}

func TestSessionTransactionRefusesMisuse(t *testing.T) {
	t.Parallel()
	db := sql.OpenDB(&gatedDriver{release: make(chan struct{})})
	t.Cleanup(func() { _ = db.Close() })
	s, err := Begin(t.Context(), db, nopBackend{})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Transaction(t.Context(), nil); err == nil {
		t.Error("nil transaction function accepted")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	called := false
	if err := s.Transaction(t.Context(), func(context.Context, *sql.Tx) error { called = true; return nil }); err == nil || called {
		t.Errorf("closed session ran a transaction: %v, called %t", err, called)
	}
}

// gatedDriver is a connection whose statements run until canceled and whose rollback waits for release.
type gatedDriver struct{ release chan struct{} }

func (g *gatedDriver) Connect(context.Context) (driver.Conn, error) { return gatedConn{g}, nil }
func (g *gatedDriver) Driver() driver.Driver                        { return g }
func (g *gatedDriver) Open(string) (driver.Conn, error)             { return gatedConn{g}, nil }

type gatedConn struct{ g *gatedDriver }

func (gatedConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("not supported") }
func (gatedConn) Close() error                        { return nil }
func (c gatedConn) Begin() (driver.Tx, error)         { return gatedTx(c), nil }
func (c gatedConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return gatedTx(c), nil
}

func (gatedConn) ExecContext(ctx context.Context, _ string, _ []driver.NamedValue) (driver.Result, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

type gatedTx gatedConn

func (gatedTx) Commit() error { return nil }
func (t gatedTx) Rollback() error {
	<-t.g.release
	return nil
}
