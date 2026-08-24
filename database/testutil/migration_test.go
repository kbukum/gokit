package testutil_test

import (
	"embed"
	"strings"
	"testing"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	sqlitedriver "gorm.io/driver/sqlite"

	"github.com/kbukum/gokit/database/migration"
	dbtestutil "github.com/kbukum/gokit/database/testutil"
)

//go:embed testdata/migrations/*.sql
var migrationsFS embed.FS

func newMemoryDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlitedriver.Open(":memory:"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func migrationConfig(t *testing.T, driver *dbtestutil.MigrationDriver) migration.Config {
	t.Helper()
	return migration.Config{
		DB:     newMemoryDB(t),
		FS:     migrationsFS,
		Path:   "testdata/migrations",
		Driver: driver.DriverFunc(),
	}
}

func TestMigrationDriver_UpRecordsVersionAndRuns(t *testing.T) {
	t.Parallel()
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
		t.Fatalf("Runs = %d, want 2", driver.Runs())
	}
	if err := cfg.Up(); err != nil {
		t.Fatalf("second Up should suppress no-change: %v", err)
	}
}

func TestMigrationDriver_FailRunSurfacesWrappedError(t *testing.T) {
	t.Parallel()
	driver := dbtestutil.NewMigrationDriver().FailRun()
	if err := migrationConfig(t, driver).Up(); err == nil || !strings.Contains(err.Error(), "migrate up") {
		t.Fatalf("Up error = %v, want wrapped 'migrate up'", err)
	}
}

func TestMigrationDriver_FailDropSurfacesWrappedError(t *testing.T) {
	t.Parallel()
	driver := dbtestutil.NewMigrationDriver().FailDrop()
	err := migrationConfig(t, driver).Reset()
	if err == nil || !strings.Contains(err.Error(), "migrate drop") {
		t.Fatalf("Reset error = %v, want wrapped 'migrate drop'", err)
	}
}

func TestMigrationDriver_FailSetVersionSurfacesWrappedError(t *testing.T) {
	t.Parallel()
	driver := dbtestutil.NewMigrationDriver().FailSetVersion()
	if err := migrationConfig(t, driver).Steps(1); err == nil || !strings.Contains(err.Error(), "migrate steps") {
		t.Fatalf("Steps error = %v, want wrapped 'migrate steps'", err)
	}
}

func TestMigrationDriver_DriverFuncReturnsUsableDriver(t *testing.T) {
	t.Parallel()
	driver := dbtestutil.NewMigrationDriver()
	got, err := driver.DriverFunc()(nil)
	if err != nil {
		t.Fatalf("DriverFunc returned error: %v", err)
	}
	if got == nil {
		t.Fatal("DriverFunc returned nil driver")
	}
}
