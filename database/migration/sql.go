package migration

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"time"

	migratedb "github.com/golang-migrate/migrate/v4/database"

	apperrors "github.com/kbukum/gokit/errors"
)

// SQLBackend owns backend-specific migration locking and destructive reset. Lock and Unlock operate on the same dedicated connection; Drop runs inside an owned transaction.
type SQLBackend interface {
	Lock(context.Context, *sql.Conn) error
	Unlock(context.Context, *sql.Conn) error
	Drop(context.Context, *sql.Tx) error
}

type sqlDriver struct {
	ctx     context.Context
	conn    *sql.Conn
	backend SQLBackend
	locked  bool
	discard bool
}

// NewSQLDriver borrows one connection for a context-bound migration session. Close returns it to the pool; it never closes the pool.
func NewSQLDriver(ctx context.Context, pool *sql.DB, backend SQLBackend) (migratedb.Driver, error) {
	if pool == nil || backend == nil {
		return nil, apperrors.InvalidInput("migration", "pool and SQL backend are required")
	}
	conn, err := pool.Conn(ctx)
	if err != nil {
		return nil, err
	}
	return &sqlDriver{ctx: ctx, conn: conn, backend: backend}, nil
}

func (*sqlDriver) Open(string) (migratedb.Driver, error) {
	return nil, apperrors.InvalidInput("migration", "use an explicitly injected database pool")
}

func (d *sqlDriver) Close() error {
	var err error
	if d.locked {
		err = d.Unlock()
	}
	if d.discard {
		discardErr := d.conn.Raw(func(any) error { return driver.ErrBadConn })
		if !errors.Is(discardErr, driver.ErrBadConn) && !errors.Is(discardErr, sql.ErrConnDone) {
			err = errors.Join(err, discardErr)
		}
	}
	closeErr := d.conn.Close()
	if errors.Is(closeErr, sql.ErrConnDone) {
		closeErr = nil
	}
	return errors.Join(err, closeErr)
}

func (d *sqlDriver) Lock() error {
	if d.locked {
		return migratedb.ErrLocked
	}
	if err := d.backend.Lock(d.ctx, d.conn); err != nil {
		d.discard = true
		return err
	}
	d.locked = true
	return d.ensureVersionTable()
}

func (d *sqlDriver) Unlock() error {
	if !d.locked {
		return migratedb.ErrNotLocked
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(d.ctx), 5*time.Second)
	defer cancel()
	d.locked = false
	if err := d.backend.Unlock(ctx, d.conn); err != nil {
		d.discard = true
		return err
	}
	return nil
}

func (d *sqlDriver) ensureVersionTable() error {
	_, err := d.conn.ExecContext(d.ctx, "CREATE TABLE IF NOT EXISTS schema_migrations (version BIGINT NOT NULL PRIMARY KEY, dirty BOOLEAN NOT NULL)")
	return err
}

func (d *sqlDriver) Version() (version int, dirty bool, err error) {
	err = d.conn.QueryRowContext(d.ctx, "SELECT version, dirty FROM schema_migrations LIMIT 1").Scan(&version, &dirty)
	if errors.Is(err, sql.ErrNoRows) {
		return migratedb.NilVersion, false, nil
	}
	return version, dirty, err
}

func (d *sqlDriver) SetVersion(version int, dirty bool) error {
	return d.transaction(func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(d.ctx, "DELETE FROM schema_migrations"); err != nil {
			return err
		}
		if version >= 0 || dirty {
			_, err := tx.ExecContext(d.ctx, "INSERT INTO schema_migrations(version, dirty) VALUES ($1, $2)", version, dirty)
			return err
		}
		return nil
	})
}

func (d *sqlDriver) Run(reader io.Reader) error {
	bytes, err := io.ReadAll(io.LimitReader(reader, maxMigrationBytes+1))
	if err != nil {
		return err
	}
	if len(bytes) > maxMigrationBytes {
		return apperrors.InvalidInput("migration", "migration file exceeds 1 MiB")
	}
	return d.transaction(func(tx *sql.Tx) error {
		_, err := tx.ExecContext(d.ctx, string(bytes))
		return err
	})
}

func (d *sqlDriver) transaction(fn func(*sql.Tx) error) (err error) {
	tx, err := d.conn.BeginTx(d.ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if rbErr := tx.Rollback(); !errors.Is(rbErr, sql.ErrTxDone) {
			err = errors.Join(err, rbErr)
		}
	}()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (d *sqlDriver) Drop() error {
	if err := d.transaction(func(tx *sql.Tx) error { return d.backend.Drop(d.ctx, tx) }); err != nil {
		return err
	}
	return d.ensureVersionTable()
}

// DropTables drops up to 1,024 tables selected by a backend-owned query in the caller's transaction. Names are quoted as identifiers; cascade enables PostgreSQL's dependent-object removal.
func DropTables(ctx context.Context, tx *sql.Tx, query string, cascade bool) (err error) {
	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	names := make([]string, 0)
	for rows.Next() {
		var name string
		if scanErr := rows.Scan(&name); scanErr != nil {
			return scanErr
		}
		names = append(names, name)
		if len(names) > 1024 {
			return apperrors.InvalidInput("migration", "reset exceeds the table limit")
		}
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return err
	}
	for _, name := range names {
		statement := `DROP TABLE IF EXISTS "` + strings.ReplaceAll(name, `"`, `""`) + `"` //nolint:gosec // G202: backend-selected identifiers are double-quoted and escaped; SQL parameters cannot bind identifiers.
		if cascade {
			statement += " CASCADE"
		}
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}
