package sqlite_test

import (
	"database/sql"
	"embed"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	migratedb "github.com/golang-migrate/migrate/v4/database"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/kbukum/gokit/database/migration"
	"github.com/kbukum/gokit/database/sqlite"
)

//go:embed testdata/migrations/*.sql
var migrationsFS embed.FS

// fakeDriver is a fake golang-migrate database driver: it records versions and run counts and can
// be told to fail specific operations, so migration orchestration is exercised without a real
// migration backend.
type fakeDriver struct {
	mu       sync.Mutex
	version  int
	dirty    bool
	runs     int
	failRun  bool
	failSet  bool
	failDrop bool
}

func newFakeDriver() *fakeDriver { return &fakeDriver{version: -1} }

func (d *fakeDriver) Open(string) (migratedb.Driver, error) { return d, nil }
func (d *fakeDriver) Close() error                          { return nil }
func (d *fakeDriver) Lock() error                           { return nil }
func (d *fakeDriver) Unlock() error                         { return nil }

func (d *fakeDriver) Run(r io.Reader) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.failRun {
		return errors.New("run failed")
	}
	if _, err := io.ReadAll(r); err != nil {
		return err
	}
	d.runs++
	return nil
}

func (d *fakeDriver) SetVersion(version int, dirty bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.failSet {
		return errors.New("set version failed")
	}
	d.version = version
	d.dirty = dirty
	return nil
}

func (d *fakeDriver) Version() (version int, dirty bool, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.version, d.dirty, nil
}

func (d *fakeDriver) Drop() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.failDrop {
		return errors.New("drop failed")
	}
	d.version = -1
	d.dirty = false
	return nil
}

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

func migrationConfig(t *testing.T, driver *fakeDriver) migration.Config {
	t.Helper()
	return migration.Config{
		DB:   newMigrationDB(t),
		FS:   migrationsFS,
		Path: "testdata/migrations",
		Driver: func(*sql.DB) (migratedb.Driver, error) {
			return driver, nil
		},
	}
}

func TestMigrationRunsAndReportsVersion(t *testing.T) {
	driver := newFakeDriver()
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
	if driver.runs != 2 {
		t.Fatalf("runs = %d, want 2", driver.runs)
	}
	if err := cfg.Up(); err != nil {
		t.Fatalf("second Up should suppress no-change: %v", err)
	}
}

func TestMigrationStepsDownAndReset(t *testing.T) {
	driver := newFakeDriver()
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

func TestMigrationWrapsMigratorCreationErrors(t *testing.T) {
	db := newMigrationDB(t)
	cfg := migration.Config{DB: db, FS: migrationsFS, Path: "testdata/migrations", Driver: func(*sql.DB) (migratedb.Driver, error) {
		return nil, errors.New("driver failed")
	}}
	if err := cfg.Up(); err == nil || !strings.Contains(err.Error(), "create database driver") {
		t.Fatalf("driver error = %v", err)
	}

	cfg = migration.Config{DB: db, FS: migrationsFS, Path: "missing", Driver: func(*sql.DB) (migratedb.Driver, error) {
		return newFakeDriver(), nil
	}}
	if err := cfg.Up(); err == nil || !strings.Contains(err.Error(), "create iofs source") {
		t.Fatalf("source error = %v", err)
	}
}

func TestMigrationWrapsOperationErrors(t *testing.T) {
	driver := newFakeDriver()
	driver.failRun = true
	if err := migrationConfig(t, driver).Up(); err == nil || !strings.Contains(err.Error(), "migrate up") {
		t.Fatalf("Up run error = %v", err)
	}
	driver = newFakeDriver()
	driver.failSet = true
	if err := migrationConfig(t, driver).Steps(1); err == nil || !strings.Contains(err.Error(), "migrate steps") {
		t.Fatalf("Steps set-version error = %v", err)
	}
	driver = newFakeDriver()
	driver.failDrop = true
	if err := migrationConfig(t, driver).Reset(); err == nil || !strings.Contains(err.Error(), "migrate drop") {
		t.Fatalf("Reset drop error = %v", err)
	}
}
