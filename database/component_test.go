package database

import (
	"context"
	"testing"

	"gorm.io/gorm"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/logging"
)

type fakeDialect struct{}

func (fakeDialect) Name() string               { return "fake" }
func (fakeDialect) Open(string) gorm.Dialector { return nil }

// TestComponent_Name tests that the component returns the correct name.
func TestComponent_Name(t *testing.T) {
	cfg := Config{
		Enabled: true,
		DSN:     ":memory:",
	}
	log := logging.NewDefault("test")
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
	log := logging.NewDefault("test")
	comp := NewComponent(cfg, log)

	var _ component.Component = comp
}

// TestComponent_WithDialect tests custom dialect wiring without importing a driver SDK.
func TestComponent_WithDialect(t *testing.T) {
	cfg := Config{
		Enabled: true,
		DSN:     ":memory:",
	}
	cfg.ApplyDefaults()
	log := logging.NewDefault("test")
	comp := NewComponent(cfg, log)

	result := comp.WithDialect(fakeDialect{})
	if result != comp {
		t.Error("WithDialect() should return the component for method chaining")
	}
}

// TestComponent_RequiresExplicitDriver tests that no backend dialect is selected by default.
func TestComponent_RequiresExplicitDialect(t *testing.T) {
	cfg := Config{
		Enabled: true,
		DSN:     ":memory:",
	}
	cfg.ApplyDefaults()
	log := logging.NewDefault("test")
	comp := NewComponent(cfg, log)

	ctx := context.Background()

	if err := comp.Start(ctx); err == nil {
		t.Fatal("Start() without explicit dialect should fail")
	}
}

func TestDialectRegistryNoSideEffects(t *testing.T) {
	reg := NewDialectRegistry()
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
	log := logging.NewDefault("test")
	comp := NewComponent(cfg, log).WithDialect(fakeDialect{})

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
	log := logging.NewDefault("test")
	comp := NewComponent(cfg, log).WithDialect(fakeDialect{})

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
	log := logging.NewDefault("test")
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
	log := logging.NewDefault("test")

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
	log := logging.NewDefault("test")
	comp := NewComponent(cfg, log).WithDialect(fakeDialect{})

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
	log := logging.NewDefault("test")

	type User struct {
		ID uint
	}

	comp := NewComponent(cfg, log).
		WithDialect(fakeDialect{}).
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
	log := logging.NewDefault("test")
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
	log := logging.NewDefault("test")
	comp := NewComponent(cfg, log).WithDialect(fakeDialect{})

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
	log := logging.NewDefault("test")
	comp := NewComponent(cfg, log).WithDialect(fakeDialect{})

	ctx := context.Background()

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start() should not error with default Enabled=false: %v", err)
	}
	if db := comp.DB(); db != nil {
		t.Error("DB() should be nil when Enabled defaults to false")
	}
}

// structuredDialect is a fake dialect that records the DSN it was asked to open and builds one
// from ConnParams, exercising the Component's structured-DSN path without a real driver SDK.
type structuredDialect struct{ gotDSN string }

func (*structuredDialect) Name() string { return "structured" }

func (d *structuredDialect) Open(dsn string) gorm.Dialector {
	d.gotDSN = dsn
	return nil
}

func (*structuredDialect) DSN(p ConnParams) (string, error) {
	return "built://" + p.Host + "/" + p.Database, nil
}

// TestComponent_ResolveDSN_ExplicitWins verifies an explicit Config.DSN is used verbatim even when
// the dialect could build one from Params.
func TestComponent_ResolveDSN_ExplicitWins(t *testing.T) {
	cfg := Config{Enabled: true, DSN: "explicit://dsn", Params: ConnParams{Host: "ignored"}}
	comp := NewComponent(cfg, logging.NewDefault("test")).WithDialect(&structuredDialect{})
	got, err := comp.resolveDSN()
	if err != nil {
		t.Fatalf("resolveDSN() error: %v", err)
	}
	if got != "explicit://dsn" {
		t.Errorf("resolveDSN() = %q, want explicit DSN", got)
	}
}

// TestComponent_ResolveDSN_FromParams verifies a StructuredDialect builds the DSN from Params when
// no explicit DSN is set.
func TestComponent_ResolveDSN_FromParams(t *testing.T) {
	cfg := Config{Enabled: true, Params: ConnParams{Host: "db.example", Database: "app"}}
	comp := NewComponent(cfg, logging.NewDefault("test")).WithDialect(&structuredDialect{})
	got, err := comp.resolveDSN()
	if err != nil {
		t.Fatalf("resolveDSN() error: %v", err)
	}
	if got != "built://db.example/app" {
		t.Errorf("resolveDSN() = %q, want built DSN from params", got)
	}
}

// TestComponent_ResolveDSN_NonStructuredRequiresDSN verifies that a dialect without structured
// support fails clearly when only Params (no DSN) are provided.
func TestComponent_ResolveDSN_NonStructuredRequiresDSN(t *testing.T) {
	cfg := Config{Enabled: true, Params: ConnParams{Host: "db.example"}}
	comp := NewComponent(cfg, logging.NewDefault("test")).WithDialect(fakeDialect{})
	if _, err := comp.resolveDSN(); err == nil {
		t.Fatal("resolveDSN() should fail for a non-structured dialect without an explicit DSN")
	}
}
