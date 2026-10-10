package migration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	migratedb "github.com/golang-migrate/migrate/v4/database"
	"github.com/golang-migrate/migrate/v4/source"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	apperrors "github.com/kbukum/gokit/errors"
)

// DefaultTimeout bounds one migration set.
const DefaultTimeout = 2 * time.Minute

// Session pins one pooled connection and holds the backend's migration lock until Close, so a coordinator can
// validate several sets and run backend schema work and every migration under one lock on one connection. It never
// borrows a second connection, so it works on a pool limited to one open connection.
type Session struct {
	mu      sync.Mutex
	conn    *sql.Conn
	lock    *sqlDriver
	backend SQLBackend
	closed  bool
	result  error
}

// Begin borrows one connection from pool and acquires the backend lock with ctx.
func Begin(ctx context.Context, pool *sql.DB, backend SQLBackend) (*Session, error) {
	if pool == nil || backend == nil {
		return nil, apperrors.InvalidInput("migration", "pool and SQL backend are required")
	}
	conn, err := pool.Conn(ctx)
	if err != nil {
		return nil, err
	}
	lock := &sqlDriver{ctx: ctx, conn: conn, backend: backend}
	if err := lock.Lock(); err != nil {
		return nil, errors.Join(err, lock.Close()) //nolint:contextcheck // unlock uses a detached bounded cleanup budget
	}
	return &Session{conn: conn, lock: lock, backend: backend}, nil
}

// Conn returns the pinned connection for backend work under the lock. It is valid until Close and must not be closed
// by the caller.
func (s *Session) Conn() *sql.Conn { return s.conn }

// Plan validates every set and inspects its stored state against its source and ExpectedVersion without mutation.
func (s *Session) Plan(ctx context.Context, sets ...Set) error {
	return s.each(ctx, sets, func(src source.Driver, d *sqlDriver, set Set) error {
		_, err := preflight(src, d, &set.ExpectedVersion)
		return err
	})
}

// Apply plans every set first, then migrates each forward to its ExpectedVersion, each bounded by DefaultTimeout. A
// failure stops at that set; sets are not one atomic transaction.
func (s *Session) Apply(ctx context.Context, sets ...Set) error {
	if err := s.Plan(ctx, sets...); err != nil {
		return err
	}
	return s.each(ctx, sets, func(src source.Driver, d *sqlDriver, set Set) error {
		return migrateUp(src, d, &set.ExpectedVersion)
	})
}

func (s *Session) each(ctx context.Context, sets []Set, fn func(source.Driver, *sqlDriver, Set) error) error {
	if len(sets) == 0 {
		return apperrors.InvalidInput("sets", "at least one migration set is required")
	}
	for _, set := range sets {
		if set.FS == nil || set.Path == "" {
			return apperrors.InvalidInput("migration", "source and path are required")
		}
		if err := validateTable(s.backend, set.Table); err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return apperrors.New(apperrors.ErrCodeServiceUnavailable, "migration session is closed")
	}
	for _, set := range sets {
		if err := s.run(ctx, set, fn); err != nil {
			return fmt.Errorf("migration set %s: %w", set.Table.SQL(), err)
		}
	}
	return nil
}

func (s *Session) run(ctx context.Context, set Set, fn func(source.Driver, *sqlDriver, Set) error) (err error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()
	if expired := ctx.Err(); expired != nil {
		return expired
	}
	src, err := iofs.New(set.FS, set.Path)
	if err != nil {
		return fmt.Errorf("create iofs source: %w", err)
	}
	defer func() { err = errors.Join(err, src.Close()) }()
	d := &sqlDriver{ctx: ctx, conn: s.conn, backend: s.backend, queries: newVersionQueries(set.Table), locked: true, borrowed: true}
	return fn(src, d, set)
}

// Close releases the lock and returns the connection, using a fresh five-second unlock budget. Repeated calls return
// the recorded result.
func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		s.result = s.lock.Close()
	}
	return s.result
}

var _ migratedb.Driver = (*sqlDriver)(nil)
