package database

import (
	"context"
	"testing"

	"gorm.io/gorm"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/logger"
)

func fakeDriver(string) gorm.Dialector { return nil }

// TestComponent_Name tests that the component returns the correct name.
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

// TestComponent_Interface tests that Component satisfies component.Component.
func TestComponent_Interface(t *testing.T) {
	cfg := Config{
		Enabled: true,
		DSN:     ":memory:",
	}
	log := logger.NewDefault("test")
	comp := NewComponent(cfg, log)

	var _ component.Component = comp
}

// TestComponent_WithDriver tests custom driver function wiring without importing a driver SDK.
func TestComponent_WithDriver(t *testing.T) {
	cfg := Config{
		Enabled: true,
		DSN:     ":memory:",
	}
	cfg.ApplyDefaults()
	log := logger.NewDefault("test")
	comp := NewComponent(cfg, log)

	result := comp.WithDriver(fakeDriver)
	if result != comp {
		t.Error("WithDriver() should return the component for method chaining")
	}
}

// TestComponent_RequiresExplicitDriver tests that no backend driver is selected by default.
func TestComponent_RequiresExplicitDriver(t *testing.T) {
	cfg := Config{
		Enabled: true,
		DSN:     ":memory:",
	}
	cfg.ApplyDefaults()
	log := logger.NewDefault("test")
	comp := NewComponent(cfg, log)

	ctx := context.Background()

	if err := comp.Start(ctx); err == nil {
		t.Fatal("Start() without explicit driver should fail")
	}
}

func TestDriverRegistryNoSideEffects(t *testing.T) {
	reg := NewDriverRegistry()
	if _, ok := reg.Get("sqlite"); ok {
		t.Fatal("sqlite registered without explicit adapter Register call")
	}
}

// TestComponent_WithAutoMigrate_Chaining tests that WithAutoMigrate returns component.
func TestComponent_WithAutoMigrate_Chaining(t *testing.T) {
	cfg := Config{
		Enabled: true,
		DSN:     ":memory:",
	}
	log := logger.NewDefault("test")
	comp := NewComponent(cfg, log).WithDriver(fakeDriver)

	type User struct {
		ID uint
	}

	result := comp.WithAutoMigrate(&User{})
	if result != comp {
		t.Error("WithAutoMigrate() should return the component for method chaining")
	}
}

// TestComponent_Health_BeforeStart tests health check before component starts.
func TestComponent_Health_BeforeStart(t *testing.T) {
	cfg := Config{
		Enabled: true,
		DSN:     ":memory:",
	}
	log := logger.NewDefault("test")
	comp := NewComponent(cfg, log).WithDriver(fakeDriver)

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

// TestComponent_Describe tests the Describe method.
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
	if desc.Details != "" && desc.Details[0:3] != "DSN" {
		t.Error("Describe Details should start with DSN")
	}
}

// TestNewWithContext_InvalidType tests NewWithContext with an invalid dialector type.
func TestNewWithContext_InvalidType(t *testing.T) {
	cfg := Config{
		Enabled: true,
		DSN:     ":memory:",
	}
	cfg.ApplyDefaults()
	log := logger.NewDefault("test")

	invalidDialector := "not-a-dialector"
	db, err := NewWithContext(context.Background(), invalidDialector, cfg, log)

	if err == nil {
		t.Error("NewWithContext() should return an error for invalid dialector type")
	}
	if db != nil {
		t.Error("NewWithContext() should return nil DB on error")
	}
	if errMsg := err.Error(); errMsg == "" {
		t.Error("Error message should not be empty")
	}
}

// TestComponent_Stop_BeforeStart tests Stop before Start is called.
func TestComponent_Stop_BeforeStart(t *testing.T) {
	cfg := Config{
		Enabled: true,
		DSN:     ":memory:",
	}
	log := logger.NewDefault("test")
	comp := NewComponent(cfg, log).WithDriver(fakeDriver)

	ctx := context.Background()

	if err := comp.Stop(ctx); err != nil {
		t.Fatalf("Stop() before Start() should not error: %v", err)
	}
}

// TestComponent_ChainedMethods tests that methods can be chained.
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

	comp := NewComponent(cfg, log).
		WithDriver(fakeDriver).
		WithAutoMigrate(&User{})

	if comp == nil {
		t.Error("Chained methods should return component")
	}
}

// TestComponent_DB_ReturnsNilBeforeStart tests DB() returns nil before Start.
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

// TestComponent_Disabled tests component behavior when Enabled=false.
func TestComponent_Disabled(t *testing.T) {
	cfg := Config{
		Enabled: false,
		DSN:     ":memory:",
	}
	cfg.ApplyDefaults()
	log := logger.NewDefault("test")
	comp := NewComponent(cfg, log).WithDriver(fakeDriver)

	ctx := context.Background()

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start() with Enabled=false should not error: %v", err)
	}
	if db := comp.DB(); db != nil {
		t.Error("DB() should be nil when component is disabled")
	}

	health := comp.Health(ctx)
	if health.Status != component.StatusHealthy {
		t.Errorf("Health Status = %q, want %q", health.Status, component.StatusHealthy)
	}
	if health.Message != "disabled" {
		t.Errorf("Health Message = %q, want %q", health.Message, "disabled")
	}
	if err := comp.Stop(ctx); err != nil {
		t.Fatalf("Stop() with Enabled=false should not error: %v", err)
	}
}

// TestComponent_EnabledDefaultBehavior tests that Enabled defaults to false.
func TestComponent_EnabledDefaultBehavior(t *testing.T) {
	cfg := Config{
		DSN: ":memory:",
	}
	cfg.ApplyDefaults()
	log := logger.NewDefault("test")
	comp := NewComponent(cfg, log).WithDriver(fakeDriver)

	ctx := context.Background()

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start() should not error with default Enabled=false: %v", err)
	}
	if db := comp.DB(); db != nil {
		t.Error("DB() should be nil when Enabled defaults to false")
	}
}
