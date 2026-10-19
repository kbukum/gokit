package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"gorm.io/gorm"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/resilience"
)

var errPingRefused = errors.New("ping refused")

// livePool is the shared state of a fake pool whose pings succeed until failPing is set.
type livePool struct {
	failPing atomic.Bool
	closes   atomic.Int64
}

type liveConnector struct{ pool *livePool }

func (c liveConnector) Connect(context.Context) (driver.Conn, error) {
	return liveConn(c), nil
}
func (liveConnector) Driver() driver.Driver { return countingDriver{} }
func (c liveConnector) Close() error {
	c.pool.closes.Add(1)
	return nil
}

type liveConn struct{ pool *livePool }

func (liveConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("not supported") }
func (liveConn) Close() error                        { return nil }
func (liveConn) Begin() (driver.Tx, error)           { return nil, errors.New("not supported") }
func (c liveConn) Ping(context.Context) error {
	if c.pool.failPing.Load() {
		return errPingRefused
	}
	return nil
}

// liveDialect opens a pool that connects and pings without a database server.
type liveDialect struct {
	gorm.Dialector
	pool *livePool
}

func (liveDialect) Name() string { return "live" }
func (d liveDialect) Prepare(context.Context, ConnectionInput) (Opener, error) {
	return liveOpener{dialect: d}, nil
}

func (d liveDialect) Initialize(db *gorm.DB) error {
	db.ConnPool = sql.OpenDB(liveConnector{pool: d.pool})
	return nil
}

type liveOpener struct{ dialect liveDialect }

func (o liveOpener) Open(context.Context) (gorm.Dialector, error) { return o.dialect, nil }

func liveConfig() Config { return Config{Enabled: true, DSN: "live", MaxRetries: 1} }

func TestComponentStartRunsChecksInOrderAndReportsHealth(t *testing.T) {
	t.Parallel()
	pool := &livePool{}
	var order []string
	var notReady atomic.Bool
	step := func(name string) Check {
		return func(context.Context, *DB) error {
			order = append(order, name)
			return nil
		}
	}
	readiness := func(context.Context, *DB) error {
		if notReady.Load() {
			return errors.New("schema missing")
		}
		order = append(order, "readiness")
		return nil
	}
	comp := newTestComponent(t, liveConfig(), WithDialect(liveDialect{pool: pool}),
		WithStartupCheck(step("startup")), WithInitialize(step("initialize")), WithReadiness(readiness))

	if err := comp.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if want := []string{"startup", "initialize", "readiness"}; !slices.Equal(order, want) {
		t.Fatalf("start order = %v, want %v", order, want)
	}
	if db, err := comp.DB(); err != nil || db == nil {
		t.Fatalf("DB after Start = %v, %v; want the published pool", db, err)
	}
	if err := comp.Start(t.Context()); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if h := comp.Health(t.Context()); h.Status != component.StatusHealthy {
		t.Fatalf("Health = %+v, want healthy", h)
	}

	notReady.Store(true)
	if h := comp.Health(t.Context()); h.Status != component.StatusUnhealthy || !strings.HasPrefix(h.Message, "not ready: ") || strings.Contains(h.Message, "schema missing") {
		t.Fatalf("Health with failing readiness = %+v, want a safe not-ready message", h)
	}
	pool.failPing.Store(true)
	if h := comp.Health(t.Context()); h.Status != component.StatusUnhealthy || !strings.HasPrefix(h.Message, "ping failed: ") || strings.Contains(h.Message, errPingRefused.Error()) {
		t.Fatalf("Health with failing ping = %+v, want a safe ping message", h)
	}

	if err := comp.Stop(t.Context()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if got := pool.closes.Load(); got != 1 {
		t.Fatalf("pool closes = %d, want 1", got)
	}
	if h := comp.Health(t.Context()); h.Status != component.StatusUnhealthy || h.Message != "database stopped" {
		t.Fatalf("Health after Stop = %+v", h)
	}
	if err := comp.Start(t.Context()); !errors.Is(err, ErrStopped) {
		t.Fatalf("Start after Stop = %v, want ErrStopped", err)
	}
}

func TestComponentStartFailureClosesPoolAndNamesTheStep(t *testing.T) {
	t.Parallel()
	failing := func(context.Context, *DB) error { return errors.New("boom") }
	for name, opt := range map[string]ComponentOption{
		"database startup check": WithStartupCheck(failing),
		"database initialize":    WithInitialize(failing),
		"database readiness":     WithReadiness(failing),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pool := &livePool{}
			comp := newTestComponent(t, liveConfig(), WithDialect(liveDialect{pool: pool}), opt)
			err := comp.Start(t.Context())
			if err == nil || !strings.Contains(err.Error(), name) {
				t.Fatalf("Start = %v, want an error naming %q", err, name)
			}
			if _, err := comp.DB(); !errors.Is(err, ErrNotStarted) {
				t.Fatalf("DB after a failed Start = %v, want ErrNotStarted", err)
			}
			if got := pool.closes.Load(); got != 1 {
				t.Fatalf("pool closes = %d, want 1", got)
			}
		})
	}
}

func TestComponentStartValidatesConfigBeforeConnecting(t *testing.T) {
	t.Parallel()
	pool := &livePool{}
	cfg := liveConfig()
	cfg.ConnMaxLifetime = "forever"
	comp := newTestComponent(t, cfg, WithDialect(liveDialect{pool: pool}))
	if err := comp.Start(t.Context()); err == nil || !strings.Contains(err.Error(), "database start") {
		t.Fatalf("Start = %v, want a validation error", err)
	}
	if got := pool.closes.Load(); got != 0 {
		t.Fatalf("an invalid config opened %d pools", got)
	}
}

func TestComponentConcurrentStartWaitsForTheInflightAttempt(t *testing.T) {
	t.Parallel()
	pool := &livePool{}
	entered, release := make(chan struct{}), make(chan struct{})
	var runs atomic.Int64
	startup := func(context.Context, *DB) error {
		if runs.Add(1) == 1 {
			close(entered)
		}
		<-release
		return nil
	}
	comp := newTestComponent(t, liveConfig(), WithDialect(liveDialect{pool: pool}), WithStartupCheck(startup))

	first := make(chan error, 1)
	go func() { first <- comp.Start(t.Context()) }()
	<-entered

	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if err := comp.Start(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("waiting Start with a canceled context = %v, want context.Canceled", err)
	}

	second := make(chan error, 1)
	go func() { second <- comp.Start(t.Context()) }()
	close(release)
	if err := <-first; err != nil {
		t.Fatalf("first Start: %v", err)
	}
	if err := <-second; err != nil {
		t.Fatalf("waiting Start: %v", err)
	}
	if got := runs.Load(); got != 1 {
		t.Fatalf("startup check ran %d times, want 1", got)
	}
	if err := comp.Stop(t.Context()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestComponentDescribeNamesAnUnresolvedRegistryBackend(t *testing.T) {
	t.Parallel()
	comp := newTestComponent(t, liveConfig(), WithDialectFromRegistry(NewDialectRegistry(), "postgres"))
	if details := comp.Describe().Details; !strings.HasPrefix(details, "Backend: postgres,") {
		t.Fatalf("Describe details = %q, want the configured backend name", details)
	}
}

func TestComponentStartsARegisteredDialectWithConnectOptions(t *testing.T) {
	t.Parallel()
	pool := &livePool{}
	reg := NewDialectRegistry()
	if err := reg.Register(liveDialect{pool: pool}); err != nil {
		t.Fatal(err)
	}
	policy := resilience.NewPolicy().WithRetry(resilience.RetryConfig{MaxAttempts: 1, RetryIf: resilience.DefaultRetryIf})
	comp := newTestComponent(t, liveConfig(), WithDialectFromRegistry(reg, "live"), WithConnectOptions(WithConnectPolicy(policy)))
	if err := comp.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if details := comp.Describe().Details; !strings.HasPrefix(details, "Backend: live,") {
		t.Fatalf("Describe details = %q, want the resolved dialect", details)
	}
	if err := comp.Stop(t.Context()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if got := pool.closes.Load(); got != 1 {
		t.Fatalf("pool closes = %d, want 1", got)
	}
}

func TestComponentDescribeReportsAutoMigrate(t *testing.T) {
	t.Parallel()
	cfg := liveConfig()
	cfg.AutoMigrate = true
	comp := newTestComponent(t, cfg, WithDialect(liveDialect{pool: &livePool{}}), WithAutoMigrate(&struct{ ID int }{}))
	if details := comp.Describe().Details; !strings.HasSuffix(details, ", auto-migrate=on") {
		t.Fatalf("Describe details = %q, want auto-migrate reported", details)
	}
}

type failingOpenDialect struct{}

func (failingOpenDialect) Name() string { return "failing-open" }
func (failingOpenDialect) Prepare(context.Context, ConnectionInput) (Opener, error) {
	return failingOpener{}, nil
}

type failingOpener struct{}

func (failingOpener) Open(context.Context) (gorm.Dialector, error) {
	return nil, errors.New("credentials unavailable")
}

func TestComponentStartReportsAnOpenerFailure(t *testing.T) {
	t.Parallel()
	comp := newTestComponent(t, liveConfig(), WithDialect(failingOpenDialect{}))
	err := comp.Start(t.Context())
	if err == nil || !strings.Contains(err.Error(), "database start") {
		t.Fatalf("Start = %v, want the opener failure", err)
	}
	if _, dbErr := comp.DB(); !errors.Is(dbErr, ErrNotStarted) {
		t.Fatalf("DB after a failed open = %v, want ErrNotStarted", dbErr)
	}
}
