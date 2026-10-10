package database

import (
	"embed"

	dbkit "github.com/kbukum/gokit/database"
	"github.com/kbukum/gokit/database/migration"
)

//go:embed migrations/*.sql
var migrations embed.FS

const (
	// SchemaVersion is the session schema version Ready must observe.
	SchemaVersion uint = 3
	// VersionTable records session migrations apart from the application's schema_migrations, so both sets keep independent version sequences in one database.
	VersionTable = "auth_session_schema_migrations"
)

// Migrations returns production versioned SQL migrations at SchemaVersion, tracked in VersionTable.
// Composition injects the driver and sets Config.Table.Schema for PostgreSQL.
func Migrations(db *dbkit.DB, driver migration.DriverFunc) migration.Config {
	cfg := migration.Config{Table: migration.Table{Name: VersionTable}}
	if db != nil {
		cfg.DB = db.GormDB
	}
	cfg.FS = migrations
	cfg.Path = "migrations"
	cfg.Driver = driver
	return cfg
}
