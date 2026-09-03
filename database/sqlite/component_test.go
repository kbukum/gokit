package sqlite_test

import (
	"context"
	"testing"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/database"
	"github.com/kbukum/gokit/database/sqlite"
	"github.com/kbukum/gokit/logging"
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
	log := logging.NewDefault("test")
	comp := database.NewComponent(cfg, log).WithDialect(sqlite.Dialect())
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
	log := logging.NewDefault("test")
	comp := database.NewComponent(cfg, log).WithDialect(sqlite.Dialect())

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
	log := logging.NewDefault("test")
	comp := database.NewComponent(cfg, log).WithDialect(sqlite.Dialect())

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
	log := logging.NewDefault("test")

	db, err := database.NewWithContext(context.Background(), sqlite.Open(cfg.DSN), cfg, log)
	if err != nil {
		t.Fatalf("NewWithContext() failed: %v", err)
	}
	if db == nil {
		t.Error("NewWithContext() returned nil DB")
	}
	if err := db.PingContext(context.Background()); err != nil {
		t.Errorf("PingContext() failed: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}
}

func TestComponentStartWithInvalidDSN(t *testing.T) {
	cfg := testConfig()
	cfg.DSN = "/invalid/path/to/db.db"
	log := logging.NewDefault("test")
	comp := database.NewComponent(cfg, log).WithDialect(sqlite.Dialect())

	err := comp.Start(context.Background())
	if err != nil && err.Error() == "" {
		t.Error("Error message should not be empty")
	}
}

func TestComponentHealthAfterStart(t *testing.T) {
	cfg := testConfig()
	log := logging.NewDefault("test")
	comp := database.NewComponent(cfg, log).WithDialect(sqlite.Dialect())
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
	log := logging.NewDefault("test")
	comp := database.NewComponent(cfg, log).WithDialect(sqlite.Dialect())
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
	log := logging.NewDefault("test")
	comp := database.NewComponent(cfg, log).WithDialect(sqlite.Dialect())
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

// unmigratable has no fields GORM can map, so AutoMigrate fails. It proves the connection Start
// opened is released when a post-open startup step fails — the registry does not Stop a component
// whose Start returns a non-context error, so the component must clean up after itself.
type unmigratable struct{}

func (unmigratable) TableName() string { return "" }

func TestComponentStartClosesPoolWhenAutoMigrateFails(t *testing.T) {
	cfg := testConfig()
	cfg.AutoMigrate = true
	log := logging.NewDefault("test")
	comp := database.NewComponent(cfg, log).
		WithDialect(sqlite.Dialect()).
		WithAutoMigrate(&unmigratable{})

	err := comp.Start(context.Background())
	if err == nil {
		t.Fatal("expected auto-migrate to fail")
	}
	if comp.DB() != nil {
		t.Fatal("DB() should be nil after a failed Start so the pool is not leaked")
	}
	// Stop must be a no-op (nil DB), not a panic or double-close.
	if stopErr := comp.Stop(context.Background()); stopErr != nil {
		t.Fatalf("Stop after failed Start: %v", stopErr)
	}
	// Health should report unhealthy (not initialized), confirming no live pool lingers.
	if h := comp.Health(context.Background()); h.Status != component.StatusUnhealthy {
		t.Fatalf("Health = %+v, want unhealthy", h)
	}
}
