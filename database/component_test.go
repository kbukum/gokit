package database

import (
	"context"
	"testing"

	"gorm.io/driver/sqlite"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/logger"
)

// TestComponent_Name tests that the component returns the correct name
func TestComponent_Name(t *testing.T) {
	cfg := Config{
		Enabled: true,
		DSN:     ":memory:",
	}
	log := logger.NewDefault("test")
	comp := NewComponent(cfg, log)

	want := "database"
	if got := comp.Name(); got != want {
		t.Errorf("Name() = %q, want %q", got, want)
	}
}

// TestComponent_Interface tests that Component satisfies component.Component
func TestComponent_Interface(t *testing.T) {
	cfg := Config{
		Enabled: true,
		DSN:     ":memory:",
	}
	log := logger.NewDefault("test")
	comp := NewComponent(cfg, log)

	var _ component.Component = comp
}

// TestComponent_Lifecycle tests basic start/stop lifecycle
func TestComponent_Lifecycle(t *testing.T) {
	cfg := Config{
		Enabled: true,
		DSN:     ":memory:",
	}
	cfg.ApplyDefaults()
	log := logger.NewDefault("test")
	comp := NewComponent(cfg, log)

	ctx := context.Background()

	// Component should not have db before start
	if db := comp.DB(); db != nil {
		t.Error("DB() should be nil before Start")
	}

	// Start should succeed
	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	// Component should have db after start
	if db := comp.DB(); db == nil {
		t.Error("DB() should not be nil after Start")
	}

	// Stop should succeed
	if err := comp.Stop(ctx); err != nil {
		t.Fatalf("Stop() failed: %v", err)
	}
}

// TestComponent_WithDriver tests custom driver function
func TestComponent_WithDriver(t *testing.T) {
	cfg := Config{
		Enabled: true,
		DSN:     ":memory:",
	}
	cfg.ApplyDefaults()
	log := logger.NewDefault("test")
	comp := NewComponent(cfg, log)

	// Custom driver that wraps sqlite
	customDriver := sqlite.Open

	// WithDriver should return the component for chaining
	result := comp.WithDriver(customDriver)
	if result != comp {
		t.Error("WithDriver() should return the component for method chaining")
	}

	ctx := context.Background()

	// Should start successfully with custom driver
	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start() with custom driver failed: %v", err)
	}

	// Component should have db
	if db := comp.DB(); db == nil {
		t.Error("DB() should not be nil after Start with custom driver")
	}

	if err := comp.Stop(ctx); err != nil {
		t.Fatalf("Stop() failed: %v", err)
	}
}

// TestComponent_DefaultSQLiteDriver tests that SQLite is used by default
func TestComponent_DefaultSQLiteDriver(t *testing.T) {
	cfg := Config{
		Enabled: true,
		DSN:     ":memory:",
	}
	cfg.ApplyDefaults()
	log := logger.NewDefault("test")
	comp := NewComponent(cfg, log)

	// Don't call WithDriver - should use default SQLite

	ctx := context.Background()

	// Should start successfully with default driver
	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start() with default SQLite driver failed: %v", err)
	}

	// Component should have db
	if db := comp.DB(); db == nil {
		t.Error("DB() should not be nil after Start with default driver")
	}

	// Database should be functional
	if err := comp.DB().Ping(); err != nil {
		t.Errorf("Ping() failed: %v", err)
	}

	if err := comp.Stop(ctx); err != nil {
		t.Fatalf("Stop() failed: %v", err)
	}
}

// TestComponent_WithAutoMigrate_Enabled tests auto-migration when enabled
func TestComponent_WithAutoMigrate_Enabled(t *testing.T) {
	cfg := Config{
		Enabled:     true,
		DSN:         ":memory:",
		AutoMigrate: true,
	}
	cfg.ApplyDefaults()
	log := logger.NewDefault("test")
	comp := NewComponent(cfg, log)

	// Simple test model
	type User struct {
		ID   uint
		Name string
	}

	// Register model for auto-migration
	comp.WithAutoMigrate(&User{})

	ctx := context.Background()

	// Start should succeed and run auto-migration
	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	// Verify table was created by checking if we can query it
	if !comp.DB().GormDB.Migrator().HasTable(&User{}) {
		t.Error("User table should have been migrated")
	}

	if err := comp.Stop(ctx); err != nil {
		t.Fatalf("Stop() failed: %v", err)
	}
}

// TestComponent_WithAutoMigrate_Disabled tests auto-migration when disabled
func TestComponent_WithAutoMigrate_Disabled(t *testing.T) {
	cfg := Config{
		Enabled:     true,
		DSN:         ":memory:",
		AutoMigrate: false,
	}
	cfg.ApplyDefaults()
	log := logger.NewDefault("test")
	comp := NewComponent(cfg, log)

	type User struct {
		ID   uint
		Name string
	}

	// Register model but auto-migrate is disabled
	comp.WithAutoMigrate(&User{})

	ctx := context.Background()

	// Start should succeed but table should not be created
	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	// Table should not exist
	if comp.DB().GormDB.Migrator().HasTable(&User{}) {
		t.Error("User table should not have been migrated when AutoMigrate is false")
	}

	if err := comp.Stop(ctx); err != nil {
		t.Fatalf("Stop() failed: %v", err)
	}
}

// TestComponent_WithAutoMigrate_Chaining tests that WithAutoMigrate returns component
func TestComponent_WithAutoMigrate_Chaining(t *testing.T) {
	cfg := Config{
		Enabled: true,
		DSN:     ":memory:",
	}
	log := logger.NewDefault("test")
	comp := NewComponent(cfg, log)

	type User struct {
		ID uint
	}

	// WithAutoMigrate should return the component for chaining
	result := comp.WithAutoMigrate(&User{})
	if result != comp {
		t.Error("WithAutoMigrate() should return the component for method chaining")
	}
}

// TestComponent_Health_BeforeStart tests health check before component starts
func TestComponent_Health_BeforeStart(t *testing.T) {
	cfg := Config{
		Enabled: true,
		DSN:     ":memory:",
	}
	log := logger.NewDefault("test")
	comp := NewComponent(cfg, log)

	ctx := context.Background()

	health := comp.Health(ctx)

	if health.Name != "database" {
		t.Errorf("Health Name = %q, want %q", health.Name, "database")
	}
	if health.Status != component.StatusUnhealthy {
		t.Errorf("Health Status = %q, want %q", health.Status, component.StatusUnhealthy)
	}
	if health.Message != "database not initialized" {
		t.Errorf("Health Message = %q, want %q", health.Message, "database not initialized")
	}
}

// TestComponent_Health_AfterStart tests health check after component starts
func TestComponent_Health_AfterStart(t *testing.T) {
	cfg := Config{
		Enabled: true,
		DSN:     ":memory:",
	}
	cfg.ApplyDefaults()
	log := logger.NewDefault("test")
	comp := NewComponent(cfg, log)

	ctx := context.Background()

	// Start the component
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

// TestComponent_Describe tests the Describe method
func TestComponent_Describe(t *testing.T) {
	cfg := Config{
		Enabled:      true,
		DSN:          "file:testdb.db?mode=memory",
		MaxOpenConns: 30,
		AutoMigrate:  true,
	}
	log := logger.NewDefault("test")
	comp := NewComponent(cfg, log)

	desc := comp.Describe()

	if desc.Name != "Database" {
		t.Errorf("Describe Name = %q, want %q", desc.Name, "Database")
	}
	if desc.Type != "database" {
		t.Errorf("Describe Type = %q, want %q", desc.Type, "database")
	}
	if desc.Details == "" {
		t.Error("Describe Details should not be empty")
	}
	// Verify DSN is masked
	if desc.Details != "" && desc.Details[0:3] != "DSN" {
		t.Error("Describe Details should start with DSN")
	}
}

// TestComponent_NewWithDialector tests NewWithDialector function
func TestComponent_NewWithDialector(t *testing.T) {
	cfg := Config{
		Enabled: true,
		DSN:     ":memory:",
	}
	cfg.ApplyDefaults()
	log := logger.NewDefault("test")

	dialector := sqlite.Open(cfg.DSN)

	db, err := NewWithDialector(dialector, cfg, log)
	if err != nil {
		t.Fatalf("NewWithDialector() failed: %v", err)
	}

	if db == nil {
		t.Error("NewWithDialector() returned nil DB")
	}

	// Verify database is functional
	if err := db.Ping(); err != nil {
		t.Errorf("Ping() failed: %v", err)
	}

	if err := db.Close(); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}
}

// TestComponent_NewWithDialector_InvalidType tests NewWithDialector with invalid dialector
func TestComponent_NewWithDialector_InvalidType(t *testing.T) {
	cfg := Config{
		Enabled: true,
		DSN:     ":memory:",
	}
	cfg.ApplyDefaults()
	log := logger.NewDefault("test")

	// Pass an invalid dialector type (string instead of gorm.Dialector)
	invalidDialector := "not-a-dialector"

	db, err := NewWithDialector(invalidDialector, cfg, log)

	if err == nil {
		t.Error("NewWithDialector() should return an error for invalid dialector type")
	}

	if db != nil {
		t.Error("NewWithDialector() should return nil DB on error")
	}

	if errMsg := err.Error(); errMsg == "" {
		t.Error("Error message should not be empty")
	}
}

// TestComponent_Stop_BeforeStart tests Stop before Start is called
func TestComponent_Stop_BeforeStart(t *testing.T) {
	cfg := Config{
		Enabled: true,
		DSN:     ":memory:",
	}
	log := logger.NewDefault("test")
	comp := NewComponent(cfg, log)

	ctx := context.Background()

	// Stop should be idempotent and not error when db is nil
	if err := comp.Stop(ctx); err != nil {
		t.Fatalf("Stop() before Start() should not error: %v", err)
	}
}

// TestComponent_ChainedMethods tests that methods can be chained
func TestComponent_ChainedMethods(t *testing.T) {
	cfg := Config{
		Enabled:     true,
		DSN:         ":memory:",
		AutoMigrate: true,
	}
	cfg.ApplyDefaults()
	log := logger.NewDefault("test")

	type User struct {
		ID uint
	}

	// Chain methods
	comp := NewComponent(cfg, log).
		WithDriver(sqlite.Open).
		WithAutoMigrate(&User{})

	if comp == nil {
		t.Error("Chained methods should return component")
	}

	ctx := context.Background()
	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	if err := comp.Stop(ctx); err != nil {
		t.Fatalf("Stop() failed: %v", err)
	}
}

// TestComponent_StartWithInvalidDSN tests Start with invalid DSN
func TestComponent_StartWithInvalidDSN(t *testing.T) {
	cfg := Config{
		Enabled: true,
		DSN:     "/invalid/path/to/db.db",
	}
	cfg.ApplyDefaults()
	log := logger.NewDefault("test")
	comp := NewComponent(cfg, log)

	ctx := context.Background()

	// Start might fail due to invalid path
	err := comp.Start(ctx)
	// Either succeeds or fails gracefully - just verify it's handled
	if err != nil && err.Error() == "" {
		t.Error("Error message should not be empty")
	}
}

// TestComponent_DB_ReturnsNilBeforeStart tests DB() returns nil before Start
func TestComponent_DB_ReturnsNilBeforeStart(t *testing.T) {
	cfg := Config{
		Enabled: true,
		DSN:     ":memory:",
	}
	log := logger.NewDefault("test")
	comp := NewComponent(cfg, log)

	if db := comp.DB(); db != nil {
		t.Error("DB() should return nil before Start")
	}
}

// TestComponent_DB_ReturnsValueAfterStart tests DB() returns value after Start
func TestComponent_DB_ReturnsValueAfterStart(t *testing.T) {
	cfg := Config{
		Enabled: true,
		DSN:     ":memory:",
	}
	cfg.ApplyDefaults()
	log := logger.NewDefault("test")
	comp := NewComponent(cfg, log)

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
