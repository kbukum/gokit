package sqlite

import (
	"context"
	"database/sql"
	"errors"

	migratedb "github.com/golang-migrate/migrate/v4/database"

	"github.com/kbukum/gokit/database/migration"
	apperrors "github.com/kbukum/gokit/errors"
)

// MigrateDriver creates a context-bound migration session. SQLite is single-process; the dialect's writable pool has
// one connection, which serializes migrations with writes. Use it only with a pool opened by this package's dialect.
func MigrateDriver() migration.DriverFunc {
	return func(ctx context.Context, pool *sql.DB, versionTable migration.Table) (migratedb.Driver, error) {
		return migration.NewSQLDriver(ctx, pool, migrationBackend{}, versionTable)
	}
}

// MigrationBackend returns the SQLite SQL backend for coordinated sessions (see migration.Begin).
func MigrationBackend() migration.SQLBackend { return migrationBackend{} }

type migrationBackend struct{}

func (migrationBackend) Lock(ctx context.Context, _ *sql.Conn) error { return ctx.Err() }

func (migrationBackend) Unlock(context.Context, *sql.Conn) error { return nil }

func (migrationBackend) ValidateTable(table migration.Table) error {
	if table.Schema != "" && table.Schema != "main" {
		return apperrors.InvalidInput("schema", "SQLite migration metadata supports only the main schema")
	}
	return nil
}

// TableExists reports whether the metadata is an ordinary table in the main schema; another kind of object is an error.
func (migrationBackend) TableExists(ctx context.Context, conn *sql.Conn, table migration.Table) (bool, error) {
	var kind string
	err := conn.QueryRowContext(ctx, "SELECT type FROM main.sqlite_master WHERE name = ?", table.Name).Scan(&kind)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if kind != "table" {
		return false, apperrors.New(apperrors.ErrCodeDatabaseError, "migration metadata is not an ordinary table")
	}
	return true, nil
}

func (migrationBackend) Drop(ctx context.Context, tx *sql.Tx, _ migration.Table) error {
	if _, err := tx.ExecContext(ctx, "PRAGMA defer_foreign_keys = ON"); err != nil {
		return err
	}
	return migration.DropTables(ctx, tx, "", "SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name LIMIT 1025", false)
}
