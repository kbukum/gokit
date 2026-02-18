// Package migration provides database migration utilities for GORM.
// It supports both file-based migrations (via golang-migrate) and programmatic migrations.
//
// This package is driver-agnostic. Users must provide a DriverFunc that creates
// the appropriate database driver for their chosen database (PostgreSQL, MySQL, SQLite, etc.).
//
// Example usage with PostgreSQL:
//
//	import (
//	    "embed"
//	    "github.com/kbukum/gokit/database/migration"
//	    migratepg "github.com/golang-migrate/migrate/v4/database/postgres"
//	)
//
//	//go:embed migrations/*.sql
//	var migrationsFS embed.FS
//
//	driverFunc := func(db *sql.DB) (database.Driver, error) {
//	    return migratepg.WithInstance(db, &migratepg.Config{})
//	}
//
//	err := migration.MigrateUp(gormDB, migrationsFS, "migrations", driverFunc)
package migration

import (
	"database/sql"
	"embed"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"gorm.io/gorm"
)

// DriverFunc creates a migrate database driver from sql.DB.
// Users provide this function to specify their database driver.
//
// Example for PostgreSQL:
//
//	import migratepg "github.com/golang-migrate/migrate/v4/database/postgres"
//	driverFunc := func(db *sql.DB) (database.Driver, error) {
//	    return migratepg.WithInstance(db, &migratepg.Config{})
//	}
//
// Example for MySQL:
//
//	import migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"
//	driverFunc := func(db *sql.DB) (database.Driver, error) {
//	    return migratemysql.WithInstance(db, &migratemysql.Config{})
//	}
type DriverFunc func(*sql.DB) (database.Driver, error)

// MigrateUp runs all pending versioned migrations from the embedded FS.
// Migration files should follow the pattern: VERSION_name.up.sql and VERSION_name.down.sql.
// Returns nil if there are no new migrations to apply (migrate.ErrNoChange is suppressed).
func MigrateUp(gormDB *gorm.DB, migrationsFS embed.FS, path string, driverFunc DriverFunc) error {
	m, err := newMigrator(gormDB, migrationsFS, path, driverFunc)
	if err != nil {
		return err
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}

// MigrateDown rolls back all versioned migrations.
// This will undo all applied migrations. Use MigrateSteps for partial rollback.
// Returns nil if there are no migrations to roll back (migrate.ErrNoChange is suppressed).
func MigrateDown(gormDB *gorm.DB, migrationsFS embed.FS, path string, driverFunc DriverFunc) error {
	m, err := newMigrator(gormDB, migrationsFS, path, driverFunc)
	if err != nil {
		return err
	}
	if err := m.Down(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrate down: %w", err)
	}
	return nil
}

// MigrateVersion returns the current migration version and dirty flag.
func MigrateVersion(gormDB *gorm.DB, migrationsFS embed.FS, path string, driverFunc DriverFunc) (version uint, dirty bool, err error) {
	m, err := newMigrator(gormDB, migrationsFS, path, driverFunc)
	if err != nil {
		return 0, false, err
	}
	return m.Version()
}

// MigrateSteps runs n migrations (positive = up, negative = down).
// Use positive n to apply n forward migrations, negative n to roll back n migrations.
// Returns nil if the requested number of migrations cannot be applied (migrate.ErrNoChange is suppressed).
func MigrateSteps(gormDB *gorm.DB, migrationsFS embed.FS, path string, n int, driverFunc DriverFunc) error {
	m, err := newMigrator(gormDB, migrationsFS, path, driverFunc)
	if err != nil {
		return err
	}
	if err := m.Steps(n); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrate steps: %w", err)
	}
	return nil
}

// MigrateReset drops everything and re-applies all migrations.
// WARNING: This will destroy all data in the database. Use with caution.
// Typically used in development/testing environments only.
func MigrateReset(gormDB *gorm.DB, migrationsFS embed.FS, path string, driverFunc DriverFunc) error {
	m, err := newMigrator(gormDB, migrationsFS, path, driverFunc)
	if err != nil {
		return err
	}
	if err := m.Drop(); err != nil {
		return fmt.Errorf("migrate drop: %w", err)
	}

	// Re-create migrator after drop (schema_migrations was dropped)
	m, err = newMigrator(gormDB, migrationsFS, path, driverFunc)
	if err != nil {
		return err
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrate up after reset: %w", err)
	}
	return nil
}

// newMigrator creates a golang-migrate instance backed by the embedded FS.
// Callers must NOT call m.Close() — it would close the shared sql.DB.
func newMigrator(gormDB *gorm.DB, migrationsFS embed.FS, path string, driverFunc DriverFunc) (*migrate.Migrate, error) {
	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql.DB: %w", err)
	}

	driver, err := driverFunc(sqlDB)
	if err != nil {
		return nil, fmt.Errorf("create database driver: %w", err)
	}

	source, err := iofs.New(migrationsFS, path)
	if err != nil {
		return nil, fmt.Errorf("create iofs source: %w", err)
	}

	// The database name is used for the source-database pair identification
	// We use a generic name since the driver handles database-specific logic
	m, err := migrate.NewWithInstance("iofs", source, "database", driver)
	if err != nil {
		return nil, fmt.Errorf("create migrator: %w", err)
	}
	return m, nil
}
