package sqlite_test

import (
	"context"
	"testing"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/database"
	"github.com/kbukum/gokit/database/sqlite"
	"github.com/kbukum/gokit/logger"
)

func testConfig() database.Config {
	cfg := database.Config{
		Enabled: true,
		DSN:     ":memory:",
	}
	cfg.ApplyDefaults()
	return cfg
}

func TestComponentLifecycleWithSQLiteAdapter(t *testing.T) {
	cfg := testConfig()
	log := logger.NewDefault("test")
	comp := database.NewComponent(cfg, log).WithDriver(sqlite.Open)
	ctx := context.Background()

	if db := comp.DB(); db != nil {
		t.Error("DB() should be nil before Start")
	}
	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	if db := comp.DB(); db == nil {
		t.Error("DB() should not be nil after Start")
	}
	if err := comp.Stop(ctx); err != nil {
		t.Fatalf("Stop() failed: %v", err)
	}
}

func TestComponentWithAutoMigrateEnabled(t *testing.T) {
	cfg := testConfig()
	cfg.AutoMigrate = true
	log := logger.NewDefault("test")
	comp := database.NewComponent(cfg, log).WithDriver(sqlite.Open)

	type User struct {
		ID   uint
		Name string
	}

	comp.WithAutoMigrate(&User{})
	ctx := context.Background()

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	if !comp.DB().GormDB.Migrator().HasTable(&User{}) {
		t.Error("User table should have been migrated")
	}
	if err := comp.Stop(ctx); err != nil {
		t.Fatalf("Stop() failed: %v", err)
	}
}

func TestComponentWithAutoMigrateDisabled(t *testing.T) {
	cfg := testConfig()
	cfg.AutoMigrate = false
	log := logger.NewDefault("test")
	comp := database.NewComponent(cfg, log).WithDriver(sqlite.Open)

	type User struct {
		ID   uint
		Name string
	}

	comp.WithAutoMigrate(&User{})
	ctx := context.Background()

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	if comp.DB().GormDB.Migrator().HasTable(&User{}) {
		t.Error("User table should not have been migrated when AutoMigrate is false")
	}
	if err := comp.Stop(ctx); err != nil {
		t.Fatalf("Stop() failed: %v", err)
	}
}

func TestNewWithContextSQLiteAdapter(t *testing.T) {
	cfg := testConfig()
	log := logger.NewDefault("test")

	db, err := database.NewWithContext(context.Background(), sqlite.Open(cfg.DSN), cfg, log)
	if err != nil {
		t.Fatalf("NewWithContext() failed: %v", err)
	}
	if db == nil {
		t.Error("NewWithContext() returned nil DB")
	}
	if err := db.Ping(); err != nil {
		t.Errorf("Ping() failed: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}
}

func TestComponentStartWithInvalidDSN(t *testing.T) {
	cfg := testConfig()
	cfg.DSN = "/invalid/path/to/db.db"
	log := logger.NewDefault("test")
	comp := database.NewComponent(cfg, log).WithDriver(sqlite.Open)

	err := comp.Start(context.Background())
	if err != nil && err.Error() == "" {
		t.Error("Error message should not be empty")
	}
}

func TestComponentHealthAfterStart(t *testing.T) {
	cfg := testConfig()
	log := logger.NewDefault("test")
	comp := database.NewComponent(cfg, log).WithDriver(sqlite.Open)
	ctx := context.Background()

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	health := comp.Health(ctx)
	if health.Name != "database" {
		t.Errorf("Health Name = %q, want %q", health.Name, "database")
	}
	if health.Status != component.StatusHealthy {
		t.Errorf("Health Status = %q, want %q", health.Status, component.StatusHealthy)
	}
	if err := comp.Stop(ctx); err != nil {
		t.Fatalf("Stop() failed: %v", err)
	}
}

func TestComponentDBReturnsValueAfterStart(t *testing.T) {
	cfg := testConfig()
	log := logger.NewDefault("test")
	comp := database.NewComponent(cfg, log).WithDriver(sqlite.Open)
	ctx := context.Background()

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	db := comp.DB()
	if db == nil {
		t.Error("DB() should not return nil after Start")
	}
	if err := comp.Stop(ctx); err != nil {
		t.Fatalf("Stop() failed: %v", err)
	}
}

func TestComponentContextInHealthCheck(t *testing.T) {
	cfg := testConfig()
	log := logger.NewDefault("test")
	comp := database.NewComponent(cfg, log).WithDriver(sqlite.Open)
	ctx := context.Background()
	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer comp.Stop(ctx)

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	health := comp.Health(canceledCtx)
	if health.Name != "database" {
		t.Errorf("Health Name = %q, want %q", health.Name, "database")
	}
}
