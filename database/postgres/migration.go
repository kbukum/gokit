package postgres

import (
	"context"
	"database/sql"
	"errors"

	migratedb "github.com/golang-migrate/migrate/v4/database"

	"github.com/kbukum/gokit/database/migration"
	apperrors "github.com/kbukum/gokit/errors"
)

// MigrateDriver creates a context-bound migration session with a database-scoped advisory lock shared by every migration set, so concurrent sets serialize.
func MigrateDriver() migration.DriverFunc {
	return func(ctx context.Context, pool *sql.DB, versionTable migration.Table) (migratedb.Driver, error) {
		return migration.NewSQLDriver(ctx, pool, migrationBackend{}, versionTable)
	}
}

// MigrationBackend returns the PostgreSQL SQL backend for coordinated sessions (see migration.Begin).
func MigrationBackend() migration.SQLBackend { return migrationBackend{} }

type migrationBackend struct{}

func (migrationBackend) Lock(ctx context.Context, conn *sql.Conn) error {
	_, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock(718049637891)")
	return err
}

func (migrationBackend) Unlock(ctx context.Context, conn *sql.Conn) error {
	_, err := conn.ExecContext(ctx, "SELECT pg_advisory_unlock(718049637891)")
	return err
}

func (migrationBackend) ValidateTable(table migration.Table) error {
	if !validSchema(table.Schema) {
		return apperrors.InvalidInput("schema", "PostgreSQL migration metadata requires an explicit application schema")
	}
	return nil
}

// TableExists reads the catalog by exact schema and name, independent of search_path. Only an ordinary table is metadata.
func (migrationBackend) TableExists(ctx context.Context, conn *sql.Conn, table migration.Table) (bool, error) {
	var kind string
	err := conn.QueryRowContext(ctx, `SELECT c.relkind::text FROM pg_catalog.pg_class c
		JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace WHERE n.nspname = $1 AND c.relname = $2`,
		table.Schema, table.Name).Scan(&kind)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if kind != "r" {
		return false, apperrors.New(apperrors.ErrCodeDatabaseError, "migration metadata is not an ordinary table")
	}
	return true, nil
}

// Drop removes every table of the metadata schema by qualified name, whatever the session search_path.
func (migrationBackend) Drop(ctx context.Context, tx *sql.Tx, table migration.Table) error {
	return migration.DropTables(ctx, tx, table.Schema,
		"SELECT tablename FROM pg_catalog.pg_tables WHERE schemaname = $1 ORDER BY tablename LIMIT 1025", true, table.Schema)
}
