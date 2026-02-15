package migration

import (
	"embed"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	migratepg "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"gorm.io/gorm"
)

// MigrateUp runs all pending versioned migrations from the embedded FS.
func MigrateUp(gormDB *gorm.DB, migrationsFS embed.FS, path string) error {
	m, err := newMigrator(gormDB, migrationsFS, path)
	if err != nil {
		return err
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}

// MigrateDown rolls back all versioned migrations.
func MigrateDown(gormDB *gorm.DB, migrationsFS embed.FS, path string) error {
	m, err := newMigrator(gormDB, migrationsFS, path)
	if err != nil {
		return err
	}
	if err := m.Down(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrate down: %w", err)
	}
	return nil
}

// MigrateVersion returns the current migration version and dirty flag.
func MigrateVersion(gormDB *gorm.DB, migrationsFS embed.FS, path string) (version uint, dirty bool, err error) {
	m, err := newMigrator(gormDB, migrationsFS, path)
	if err != nil {
		return 0, false, err
	}
	return m.Version()
}

// MigrateSteps runs n migrations (positive = up, negative = down).
func MigrateSteps(gormDB *gorm.DB, migrationsFS embed.FS, path string, n int) error {
	m, err := newMigrator(gormDB, migrationsFS, path)
	if err != nil {
		return err
	}
	if err := m.Steps(n); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrate steps: %w", err)
	}
	return nil
}

// MigrateReset drops everything and re-applies all migrations.
func MigrateReset(gormDB *gorm.DB, migrationsFS embed.FS, path string) error {
	m, err := newMigrator(gormDB, migrationsFS, path)
	if err != nil {
		return err
	}
	if err := m.Drop(); err != nil {
		return fmt.Errorf("migrate drop: %w", err)
	}

	// Re-create migrator after drop (schema_migrations was dropped)
	m, err = newMigrator(gormDB, migrationsFS, path)
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
func newMigrator(gormDB *gorm.DB, migrationsFS embed.FS, path string) (*migrate.Migrate, error) {
	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql.DB: %w", err)
	}

	driver, err := migratepg.WithInstance(sqlDB, &migratepg.Config{})
	if err != nil {
		return nil, fmt.Errorf("create postgres driver: %w", err)
	}

	source, err := iofs.New(migrationsFS, path)
	if err != nil {
		return nil, fmt.Errorf("create iofs source: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", source, "postgres", driver)
	if err != nil {
		return nil, fmt.Errorf("create migrator: %w", err)
	}
	return m, nil
}
