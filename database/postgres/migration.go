package postgres

import (
	"context"
	"database/sql"

	migratedb "github.com/golang-migrate/migrate/v4/database"

	"github.com/kbukum/gokit/database/migration"
)

// MigrateDriver creates a context-bound migration session with a database-scoped advisory lock shared by every migration set, so concurrent sets serialize.
func MigrateDriver() migration.DriverFunc {
	return func(ctx context.Context, pool *sql.DB, versionTable string) (migratedb.Driver, error) {
		return migration.NewSQLDriver(ctx, pool, migrationBackend{}, versionTable)
	}
}

type migrationBackend struct{}

func (migrationBackend) Lock(ctx context.Context, conn *sql.Conn) error {
	_, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock(718049637891)")
	return err
}

func (migrationBackend) Unlock(ctx context.Context, conn *sql.Conn) error {
	_, err := conn.ExecContext(ctx, "SELECT pg_advisory_unlock(718049637891)")
	return err
}

func (migrationBackend) Drop(ctx context.Context, tx *sql.Tx) error {
	return migration.DropTables(ctx, tx, "SELECT tablename FROM pg_tables WHERE schemaname = current_schema() ORDER BY tablename LIMIT 1025", true)
}
