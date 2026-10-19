package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/logging"
)

type fakeDialect struct{}

func (fakeDialect) Name() string { return "fake" }
func (fakeDialect) Prepare(context.Context, ConnectionInput) (Opener, error) {
	return nil, fmt.Errorf("fake dialect has no connections")
}

func newTestComponent(t *testing.T, cfg Config, opts ...ComponentOption) *Component {
	t.Helper()
	comp, err := NewComponent(cfg, logging.NewDefault("test"), opts...)
	if err != nil {
		t.Fatalf("NewComponent: %v", err)
	}
	return comp
}

// TestComponent_Name tests that the component returns the correct name.
func TestComponent_Name(t *testing.T) {
	cfg := Config{
		Enabled: true,
		DSN:     ":memory:",
	}
	comp := newTestComponent(t, cfg)

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
	comp := newTestComponent(t, cfg)

	var _ component.Component = comp
}

// TestComponent_RequiresExplicitDriver tests that no backend dialect is selected by default.
func TestComponent_RequiresExplicitDialect(t *testing.T) {
	cfg := Config{
		Enabled: true,
		DSN:     ":memory:",
	}
	cfg.ApplyDefaults()
	comp := newTestComponent(t, cfg)

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

// TestComponent_Health_BeforeStart tests health check before component starts.
func TestComponent_Health_BeforeStart(t *testing.T) {
	cfg := Config{
		Enabled: true,
		DSN:     ":memory:",
	}
	comp := newTestComponent(t, cfg, WithDialect(fakeDialect{}))

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
	comp := newTestComponent(t, cfg)

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
	if desc.Details != "" && desc.Details[0:7] != "Backend" {
		t.Error("Describe Details should identify the backend without exposing a DSN")
	}
}

// TestNewWithContext_InvalidType tests NewWithContext with an invalid dialector type.
func TestNewWithContext_InvalidType(t *testing.T) {
	cfg := Config{
		Enabled: true,
		DSN:     ":memory:",
	}
	cfg.ApplyDefaults()

	db, err := NewWithContext(context.Background(), nil, cfg, logging.NewDefault("test"))

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
	comp := newTestComponent(t, cfg, WithDialect(fakeDialect{}))

	ctx := context.Background()

	if err := comp.Stop(ctx); err != nil {
		t.Fatalf("Stop() before Start() should not error: %v", err)
	}
}

// TestComponent_DB_ReturnsNilBeforeStart tests DB() reports ErrNotStarted before Start.
func TestComponent_DB_ReturnsNilBeforeStart(t *testing.T) {
	cfg := Config{
		Enabled: true,
		DSN:     ":memory:",
	}
	comp := newTestComponent(t, cfg)

	if db, err := comp.DB(); db != nil || !errors.Is(err, ErrNotStarted) {
		t.Errorf("DB() before Start = %v, %v", db, err)
	}
}

// TestComponent_Disabled tests component behavior when Enabled=false.
func TestComponent_Disabled(t *testing.T) {
	cfg := Config{
		Enabled: false,
		DSN:     ":memory:",
	}
	cfg.ApplyDefaults()
	comp := newTestComponent(t, cfg, WithDialect(fakeDialect{}))

	ctx := context.Background()

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start() with Enabled=false should not error: %v", err)
	}
	if db, err := comp.DB(); db != nil || !errors.Is(err, ErrNotStarted) {
		t.Errorf("DB() when disabled = %v, %v", db, err)
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
	comp := newTestComponent(t, cfg, WithDialect(fakeDialect{}))

	ctx := context.Background()

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start() should not error with default Enabled=false: %v", err)
	}
	if db, err := comp.DB(); db != nil || !errors.Is(err, ErrNotStarted) {
		t.Errorf("DB() when Enabled defaults to false = %v, %v", db, err)
	}
}

type structuredDialect struct{ gotInput ConnectionInput }

func (*structuredDialect) Name() string { return "structured" }

func (d *structuredDialect) Prepare(_ context.Context, input ConnectionInput) (Opener, error) {
	d.gotInput = input
	return nil, fmt.Errorf("fixture does not allocate connections")
}

func TestComponentRejectsAmbiguousConnectionInput(t *testing.T) {
	cfg := Config{Enabled: true, DSN: "explicit://dsn", Params: ConnParams{Host: "ignored"}}
	dialect := &structuredDialect{}
	comp := newTestComponent(t, cfg, WithDialect(dialect))
	if err := comp.Start(t.Context()); err == nil || dialect.gotInput.DSN != "" {
		t.Fatal("ambiguous input reached backend preparation")
	}
}

func TestComponentPassesStructuredInputWithoutSerialization(t *testing.T) {
	cfg := Config{Enabled: true, Params: ConnParams{Host: "db.example", Database: "app"}}
	dialect := &structuredDialect{}
	comp := newTestComponent(t, cfg, WithDialect(dialect))
	if err := comp.Start(t.Context()); err == nil {
		t.Fatal("fixture falsely opened a connection")
	}
	if dialect.gotInput.Params.Host != "db.example" || dialect.gotInput.DSN != "" {
		t.Fatal("structured configuration was not passed directly")
	}
}

func TestComponentPassesOpaqueInputWithoutParsing(t *testing.T) {
	dialect := &structuredDialect{}
	comp := newTestComponent(t, Config{Enabled: true, DSN: "opaque"}, WithDialect(dialect))
	if err := comp.Start(t.Context()); err == nil || dialect.gotInput.DSN != "opaque" {
		t.Fatal("opaque configuration was not passed directly")
	}
}

func TestNewComponentValidatesOptions(t *testing.T) {
	log := logging.NewDefault("test")
	check := func(context.Context, *DB) error { return nil }
	for name, opt := range map[string]ComponentOption{
		"nil option":         nil,
		"empty name":         WithComponentName(""),
		"nil dialect":        WithDialect(nil),
		"nil registry":       WithDialectFromRegistry(nil, "sqlite"),
		"empty backend":      WithDialectFromRegistry(NewDialectRegistry(), ""),
		"nil connect option": WithConnectOptions(nil),
		"nil initialize":     WithInitialize(nil),
		"nil startup check":  WithStartupCheck(nil),
		"nil readiness":      WithReadiness(nil),
		"zero health":        WithHealthTimeout(0),
		"huge health":        WithHealthTimeout(MaxHealthTimeout + 1),
	} {
		if _, err := NewComponent(Config{}, log, opt); err == nil {
			t.Errorf("%s: NewComponent accepted an invalid option", name)
		}
	}
	if _, err := NewComponent(Config{}, nil); err == nil {
		t.Error("NewComponent accepted a nil logger")
	}
	comp, err := NewComponent(Config{}, log, WithComponentName("orders-db"), WithReadiness(check), WithInitialize(check), WithHealthTimeout(MaxHealthTimeout))
	if err != nil || comp.Name() != "orders-db" || comp.Describe().Name != "orders-db" {
		t.Fatalf("valid options: %v", err)
	}
}

func TestComponentReportsUnregisteredDialectAtStart(t *testing.T) {
	comp := newTestComponent(t, Config{Enabled: true, DSN: "x"}, WithDialectFromRegistry(NewDialectRegistry(), "missing"))
	if err := comp.Start(t.Context()); err == nil || !strings.Contains(err.Error(), `"missing"`) {
		t.Fatalf("Start = %v, want an unregistered dialect error", err)
	}
}
