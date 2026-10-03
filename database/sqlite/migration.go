package sqlite

import (
	"context"
	"database/sql"

	migratedb "github.com/golang-migrate/migrate/v4/database"

	"github.com/kbukum/gokit/database/migration"
)

// MigrateDriver creates a context-bound migration session. SQLite is single-process; one connection serializes migrations and writes on this pool.
func MigrateDriver() migration.DriverFunc {
	return func(ctx context.Context, pool *sql.DB) (migratedb.Driver, error) {
		pool.SetMaxOpenConns(1)
		return migration.NewSQLDriver(ctx, pool, migrationBackend{})
	}
}

type migrationBackend struct{}

func (migrationBackend) Lock(ctx context.Context, _ *sql.Conn) error { return ctx.Err() }

func (migrationBackend) Unlock(context.Context, *sql.Conn) error { return nil }

func (migrationBackend) Drop(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, "PRAGMA defer_foreign_keys = ON"); err != nil {
		return err
	}
	return migration.DropTables(ctx, tx, "SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name LIMIT 1025", false)
}
