package sqlite_test

import (
	"database/sql"
	"embed"
	"errors"
	"strings"
	"testing"

	migratedb "github.com/golang-migrate/migrate/v4/database"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/kbukum/gokit/database/migration"
	"github.com/kbukum/gokit/database/sqlite"
	dbtestutil "github.com/kbukum/gokit/database/testutil"
)

//go:embed testdata/migrations/*.sql
var migrationsFS embed.FS

func newMigrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	return db
}

func migrationConfig(t *testing.T, driver *dbtestutil.MigrationDriver) migration.Config {
	t.Helper()
	return migration.Config{
		DB:     newMigrationDB(t),
		FS:     migrationsFS,
		Path:   "testdata/migrations",
		Driver: driver.DriverFunc(),
	}
}

func TestMigrationRunsAndReportsVersion(t *testing.T) {
	driver := dbtestutil.NewMigrationDriver()
	cfg := migrationConfig(t, driver)
	if err := cfg.Up(); err != nil {
		t.Fatalf("Up: %v", err)
	}
	version, dirty, err := cfg.Version()
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if version != 2 || dirty {
		t.Fatalf("Version = %d dirty=%v, want 2 false", version, dirty)
	}
	if driver.Runs() != 2 {
		t.Fatalf("runs = %d, want 2", driver.Runs())
	}
	if err := cfg.Up(); err != nil {
		t.Fatalf("second Up should suppress no-change: %v", err)
	}
}

func TestMigrationStepsDownAndReset(t *testing.T) {
	driver := dbtestutil.NewMigrationDriver()
	cfg := migrationConfig(t, driver)
	if err := cfg.Steps(1); err != nil {
		t.Fatalf("Steps up: %v", err)
	}
	version, _, err := cfg.Version()
	if err != nil || version != 1 {
		t.Fatalf("version after one step = %d err=%v", version, err)
	}
	if err := cfg.Steps(-1); err != nil {
		t.Fatalf("Steps down: %v", err)
	}
	if err := cfg.Down(); err != nil {
		t.Fatalf("Down no-change: %v", err)
	}
	if err := cfg.Reset(); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	version, dirty, err := cfg.Version()
	if err != nil {
		t.Fatalf("Version after reset: %v", err)
	}
	if version != 2 || dirty {
		t.Fatalf("Version after reset = %d dirty=%v", version, dirty)
	}
}

// TestMigrationDownFailureIsSurfaced proves a failing rollback statement is wrapped and surfaced,
// not swallowed: Down must return a "migrate down" error rather than a success-shaped nil.
func TestMigrationDownFailureIsSurfaced(t *testing.T) {
	driver := dbtestutil.NewMigrationDriver()
	cfg := migrationConfig(t, driver)
	if err := cfg.Up(); err != nil {
		t.Fatalf("Up: %v", err)
	}
	driver.FailRun()
	err := cfg.Down()
	if err == nil || !strings.Contains(err.Error(), "migrate down") {
		t.Fatalf("Down error = %v, want wrapped 'migrate down'", err)
	}
}

// TestMigrationResetReapplyFailureIsSurfaced proves that when Drop succeeds but the re-apply after
// reset fails, Reset surfaces the "migrate up after reset" error instead of reporting success.
func TestMigrationResetReapplyFailureIsSurfaced(t *testing.T) {
	driver := dbtestutil.NewMigrationDriver()
	cfg := migrationConfig(t, driver)
	if err := cfg.Up(); err != nil {
		t.Fatalf("Up: %v", err)
	}
	driver.FailRun() // Drop still succeeds; the Up after reset must fail.
	err := cfg.Reset()
	if err == nil || !strings.Contains(err.Error(), "migrate up after reset") {
		t.Fatalf("Reset error = %v, want wrapped 'migrate up after reset'", err)
	}
}

func TestMigrationWrapsMigratorCreationErrors(t *testing.T) {
	db := newMigrationDB(t)
	cfg := migration.Config{DB: db, FS: migrationsFS, Path: "testdata/migrations", Driver: func(*sql.DB) (migratedb.Driver, error) {
		return nil, errors.New("driver failed")
	}}
	if err := cfg.Up(); err == nil || !strings.Contains(err.Error(), "create database driver") {
		t.Fatalf("driver error = %v", err)
	}

	cfg = migration.Config{DB: db, FS: migrationsFS, Path: "missing", Driver: dbtestutil.NewMigrationDriver().DriverFunc()}
	if err := cfg.Up(); err == nil || !strings.Contains(err.Error(), "create iofs source") {
		t.Fatalf("source error = %v", err)
	}
}

func TestMigrationWrapsOperationErrors(t *testing.T) {
	if err := migrationConfig(t, dbtestutil.NewMigrationDriver().FailRun()).Up(); err == nil ||
		!strings.Contains(err.Error(), "migrate up") {
		t.Fatalf("Up run error = %v", err)
	}
	if err := migrationConfig(t, dbtestutil.NewMigrationDriver().FailSetVersion()).Steps(1); err == nil ||
		!strings.Contains(err.Error(), "migrate steps") {
		t.Fatalf("Steps set-version error = %v", err)
	}
	if err := migrationConfig(t, dbtestutil.NewMigrationDriver().FailDrop()).Reset(); err == nil ||
		!strings.Contains(err.Error(), "migrate drop") {
		t.Fatalf("Reset drop error = %v", err)
	}
}
