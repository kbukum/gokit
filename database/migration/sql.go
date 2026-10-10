package migration

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"time"

	migratedb "github.com/golang-migrate/migrate/v4/database"

	apperrors "github.com/kbukum/gokit/errors"
)

// SQLBackend owns backend-specific migration locking, qualified metadata discovery and destructive reset. Lock and
// Unlock operate on the same dedicated connection; Drop runs inside an owned transaction and removes only the tables of
// the version table's schema. Catalog inspection never creates metadata; TableExists reports a relation of another kind
// with the metadata name as an error, never as absent.
type SQLBackend interface {
	Lock(context.Context, *sql.Conn) error
	Unlock(context.Context, *sql.Conn) error
	Drop(ctx context.Context, tx *sql.Tx, table Table) error
	TableExists(ctx context.Context, conn *sql.Conn, table Table) (bool, error)
	ValidateTable(Table) error
}

type sqlDriver struct {
	ctx     context.Context
	conn    *sql.Conn
	backend SQLBackend
	queries versionQueries
	locked  bool
	discard bool
	// borrowed drivers run inside a Session, which owns the connection and lock.
	borrowed bool
}

// NewSQLDriver borrows one connection for a context-bound migration session that records its version in versionTable (see IsVersionTable). Close returns the connection to the pool; it never closes the pool.
func NewSQLDriver(ctx context.Context, pool *sql.DB, backend SQLBackend, versionTable Table) (migratedb.Driver, error) {
	if pool == nil || backend == nil {
		return nil, apperrors.InvalidInput("migration", "pool and SQL backend are required")
	}
	if err := validateTable(backend, versionTable); err != nil {
		return nil, err
	}
	conn, err := pool.Conn(ctx)
	if err != nil {
		return nil, err
	}
	return &sqlDriver{ctx: ctx, conn: conn, backend: backend, queries: newVersionQueries(versionTable)}, nil
}

func validateTable(backend SQLBackend, table Table) error {
	if err := table.Validate(); err != nil {
		return err
	}
	return backend.ValidateTable(table)
}

// versionQueries holds the version-table statements. The table name is validated by IsVersionTable before interpolation; SQL parameters cannot bind identifiers.
type versionQueries struct {
	table                      Table
	create, read, clear, write string
}

func newVersionQueries(table Table) versionQueries {
	name := table.SQL()
	return versionQueries{
		table:  table,
		create: "CREATE TABLE IF NOT EXISTS " + name + " (version BIGINT NOT NULL PRIMARY KEY, dirty BOOLEAN NOT NULL)",
		read:   "SELECT version, dirty FROM " + name + " LIMIT 2",
		clear:  "DELETE FROM " + name,
		write:  "INSERT INTO " + name + "(version, dirty) VALUES ($1, $2)",
	}
}

func (*sqlDriver) Open(string) (migratedb.Driver, error) {
	return nil, apperrors.InvalidInput("migration", "use an explicitly injected database pool")
}

func (d *sqlDriver) Close() error {
	if d.borrowed {
		return nil
	}
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
	if d.locked || d.borrowed {
		return migratedb.ErrLocked
	}
	if err := d.backend.Lock(d.ctx, d.conn); err != nil {
		d.discard = true
		return err
	}
	d.locked = true
	return nil
}

func (d *sqlDriver) Unlock() error {
	if !d.locked || d.borrowed {
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
	_, err := d.conn.ExecContext(d.ctx, d.queries.create)
	return err
}

// Version reads the recorded version. Without the migration lock it is a read-only inspection: only a missing version
// table or an empty one is no version, nothing is created, and more than one row is corrupt metadata.
func (d *sqlDriver) Version() (version int, dirty bool, err error) {
	exists, existsErr := d.backend.TableExists(d.ctx, d.conn, d.queries.table)
	if existsErr != nil || !exists {
		return migratedb.NilVersion, false, existsErr
	}
	rows, err := d.conn.QueryContext(d.ctx, d.queries.read)
	if err != nil {
		return migratedb.NilVersion, false, err
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	version, count := migratedb.NilVersion, 0
	for rows.Next() {
		count++
		if count > 1 {
			return migratedb.NilVersion, false, apperrors.New(apperrors.ErrCodeDatabaseError, "migration metadata holds more than one version")
		}
		if err := rows.Scan(&version, &dirty); err != nil {
			return migratedb.NilVersion, false, err
		}
	}
	if err := rows.Err(); err != nil {
		return migratedb.NilVersion, false, err
	}
	return version, dirty, nil
}

func (d *sqlDriver) SetVersion(version int, dirty bool) error {
	if !d.locked {
		return migratedb.ErrNotLocked
	}
	if err := d.ensureVersionTable(); err != nil {
		return err
	}
	return d.transaction(func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(d.ctx, d.queries.clear); err != nil {
			return err
		}
		if version >= 0 || dirty {
			_, err := tx.ExecContext(d.ctx, d.queries.write, version, dirty)
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
	if err := d.transaction(func(tx *sql.Tx) error { return d.backend.Drop(d.ctx, tx, d.queries.table) }); err != nil {
		return err
	}
	return nil
}

// DropTables drops up to 1,024 tables selected by a backend-owned query in the caller's transaction. Each name is
// quoted and qualified by schema when it is not empty, so the session search_path cannot redirect a drop; cascade
// enables PostgreSQL's dependent-object removal.
func DropTables(ctx context.Context, tx *sql.Tx, schema, query string, cascade bool, args ...any) (err error) {
	rows, err := tx.QueryContext(ctx, query, args...)
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
		statement := "DROP TABLE IF EXISTS " + Table{Schema: schema, Name: name}.SQL() //nolint:gosec // G202: backend-selected identifiers are double-quoted and escaped; SQL parameters cannot bind identifiers.
		if cascade {
			statement += " CASCADE"
		}
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}
