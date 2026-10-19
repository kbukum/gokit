package database

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/kbukum/gokit/component"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/logging"
)

// Health budgets.
const (
	// DefaultHealthTimeout bounds one Health call, including the ping and the readiness check, unless [WithHealthTimeout] sets another budget.
	DefaultHealthTimeout = time.Second
	// MaxHealthTimeout is the largest accepted health budget.
	MaxHealthTimeout = 30 * time.Second
)

var (
	// ErrNotStarted reports access to a component that is disabled, not started or whose Start failed. It is a
	// retryable SERVICE_UNAVAILABLE application error; match it with errors.Is.
	ErrNotStarted = apperrors.New(apperrors.ErrCodeServiceUnavailable, "database component is not started")
	// ErrStopped reports Start or access after Stop. Stop is terminal, so it is a non-retryable SERVICE_UNAVAILABLE
	// application error; match it with errors.Is.
	ErrStopped = func() *apperrors.AppError {
		err := apperrors.New(apperrors.ErrCodeServiceUnavailable, "database component is stopped")
		err.Retryable = false
		return err
	}()
)

// Check inspects or prepares an opened database during startup or health. It must honor ctx.
type Check func(ctx context.Context, db *DB) error

// Component wraps DB and implements component.Component for lifecycle management. Its configuration is fixed by [NewComponent]; the lifecycle methods and DB are safe for concurrent use.
//
// Start opens the pool, runs auto-migration, the StartupCheck, the Initialize check and the Readiness check, then publishes the pool. A failed Start closes its pool and may be retried; a successful Start is idempotent. Stop is terminal, prevents a concurrent Start from publishing and records its close result. Health pings and reruns Readiness within the health budget, so schema or access drift after startup reports unhealthy.
type Component struct {
	mu       sync.Mutex
	db       *DB
	starting chan struct{}
	stopped  bool
	stopDone chan struct{}
	stopErr  error
	lateErr  error
	cfg      Config
	log      *logging.Logger
	options  componentOptions
}

type componentOptions struct {
	name          string
	models        []any
	dialect       Dialect
	dialectName   string
	connect       []Option
	startup       Check
	initialize    Check
	readiness     Check
	healthTimeout time.Duration
}

// ComponentOption configures a [Component] at construction.
type ComponentOption func(*componentOptions) error

// NewComponent creates a database component for use with the component registry. It is named "database" unless [WithComponentName] names it. Dialects are opt-in through [WithDialect] or [WithDialectFromRegistry]; a disabled config (Config.Enabled false) needs none.
func NewComponent(cfg Config, log *logging.Logger, opts ...ComponentOption) (*Component, error) {
	if log == nil {
		return nil, apperrors.InvalidInput("logger", "A database component requires an injected logger")
	}
	options := componentOptions{name: "database", healthTimeout: DefaultHealthTimeout}
	for _, opt := range opts {
		if opt == nil {
			return nil, apperrors.InvalidInput("option", "Database component options must not be nil")
		}
		if err := opt(&options); err != nil {
			return nil, err
		}
	}
	return &Component{cfg: cfg, log: log.WithComponent(options.name), options: options}, nil
}

// WithComponentName sets the unique registry name, so one process can own several database components. The name also scopes the component's logger and its bootstrap display name, so several databases in one process stay distinguishable in logs and the summary.
func WithComponentName(name string) ComponentOption {
	return func(o *componentOptions) error {
		if name == "" {
			return apperrors.InvalidInput("name", "A database component name must not be empty")
		}
		o.name = name
		return nil
	}
}

// WithDialect sets the database backend dialect directly.
//
//	db, err := database.NewComponent(cfg, log, database.WithDialect(postgres.Dialect()), database.WithAutoMigrate(&User{}, &Post{}))
func WithDialect(d Dialect) ComponentOption {
	return func(o *componentOptions) error {
		if d == nil {
			return apperrors.InvalidInput("dialect", "A database dialect is required")
		}
		o.dialect, o.dialectName = d, ""
		return nil
	}
}

// WithDialectFromRegistry selects a registered dialect by backend name. A name the registry lacks fails Start, not construction, so a disabled component may name an adapter the binary does not register.
func WithDialectFromRegistry(reg *DialectRegistry, name string) ComponentOption {
	return func(o *componentOptions) error {
		if reg == nil || name == "" {
			return apperrors.InvalidInput("dialect", "A dialect registry and backend name are required")
		}
		o.dialect, o.dialectName = nil, name
		if d, ok := reg.Get(name); ok {
			o.dialect = d
		}
		return nil
	}
}

// WithAutoMigrate registers models for auto-migration on Start. Models are only migrated if Config.AutoMigrate is true and the component is enabled.
func WithAutoMigrate(models ...any) ComponentOption {
	return func(o *componentOptions) error {
		o.models = append(o.models, models...)
		return nil
	}
}

// WithConnectOptions passes connection options, such as [WithSecretSource] or [WithConnectPolicy], to the pool Start opens.
func WithConnectOptions(opts ...Option) ComponentOption {
	return func(o *componentOptions) error {
		for _, opt := range opts {
			if opt == nil {
				return apperrors.InvalidInput("option", "Connection options must not be nil")
			}
		}
		o.connect = append(o.connect, opts...)
		return nil
	}
}

// WithStartupCheck injects a read-only check that runs once at Start, after auto-migration and before Initialize, such as verifying the schema version and access an initializer relies on. Its failure fails Start and closes the pool. It does not run on Health; use [WithReadiness] for checks that must keep holding.
func WithStartupCheck(check Check) ComponentOption {
	return func(o *componentOptions) error {
		if check == nil {
			return apperrors.InvalidInput("startup_check", "A startup check is required")
		}
		o.startup = check
		return nil
	}
}

// WithInitialize injects a startup-only step that runs after the startup check and before Readiness, such as verifying persisted installation state. Its failure fails Start.
func WithInitialize(check Check) ComponentOption {
	return func(o *componentOptions) error {
		if check == nil {
			return apperrors.InvalidInput("initialize", "An initialize check is required")
		}
		o.initialize = check
		return nil
	}
}

// WithReadiness injects a read-only check, such as schema version and access readiness, that gates publication and every Health call. It must not take locks, run DDL or migrate, and must finish within the health budget.
func WithReadiness(check Check) ComponentOption {
	return func(o *componentOptions) error {
		if check == nil {
			return apperrors.InvalidInput("readiness", "A readiness check is required")
		}
		o.readiness = check
		return nil
	}
}

// WithHealthTimeout bounds one Health call, including the ping and the readiness check. It must be positive and at most [MaxHealthTimeout]; the default is [DefaultHealthTimeout].
func WithHealthTimeout(budget time.Duration) ComponentOption {
	return func(o *componentOptions) error {
		if budget <= 0 || budget > MaxHealthTimeout {
			return apperrors.InvalidInput("health_timeout", "The health budget must be positive and at most 30s")
		}
		o.healthTimeout = budget
		return nil
	}
}

// DB returns the published pool. It returns [ErrNotStarted] before a successful Start or when disabled, and
// [ErrStopped] after Stop. Callers own no part of the pool and must drain their work before Stop.
func (c *Component) DB() (*DB, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch {
	case c.stopped:
		return nil, ErrStopped
	case c.db == nil:
		return nil, ErrNotStarted
	default:
		return c.db, nil
	}
}

// ensure Component satisfies component.Component
var _ component.Component = (*Component)(nil)

// Name returns the component name.
func (c *Component) Name() string { return c.options.name }

// Start connects to the database, runs auto-migration and the injected checks, then publishes the pool. If
// Config.Enabled is false, this method returns immediately without error. The context bounds connection retries
// and the checks. A concurrent Start waits for the in-flight attempt.
func (c *Component) Start(ctx context.Context) error {
	done, claimed, err := c.beginStart(ctx)
	if err != nil || !claimed {
		return err
	}
	defer c.finishStart(done)
	db, err := c.open(ctx)
	if err != nil {
		return err
	}
	c.mu.Lock()
	if !c.stopped {
		c.db = db
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()
	// Stop waits for finishStart, so recording the result here lets Stop report this cleanup.
	closeErr := db.Close() //nolint:contextcheck // Close owns a bounded cleanup without a request context
	c.mu.Lock()
	c.lateErr = closeErr
	c.mu.Unlock()
	return errors.Join(ErrStopped, closeErr)
}

// finishStart releases the start slot after any late-pool cleanup, so a waiting Stop observes a closed pool.
func (c *Component) finishStart(done chan struct{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.starting = nil
	close(done)
}

// beginStart claims the start slot. It reports claimed=false when the component is already started or disabled.
func (c *Component) beginStart(ctx context.Context) (done chan struct{}, claimed bool, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for {
		switch {
		case c.stopped:
			return nil, false, ErrStopped
		case c.db != nil:
			return nil, false, nil
		case c.starting == nil:
			if !c.cfg.Enabled {
				c.log.InfoCtx(ctx, "Database component is disabled")
				return nil, false, nil
			}
			c.starting = make(chan struct{})
			return c.starting, true, nil
		}
		inflight := c.starting
		c.mu.Unlock()
		select {
		case <-inflight:
		case <-ctx.Done():
			c.mu.Lock()
			return nil, false, ctx.Err()
		}
		c.mu.Lock()
	}
}

// open performs one start attempt without holding the lifecycle lock and closes the pool on any failure.
func (c *Component) open(ctx context.Context) (*DB, error) {
	// Apply defaults and validate before doing any work so a misconfigured component fails fast instead of
	// attempting to connect with a zero-value input.
	cfg := c.cfg
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("database start: %w", err)
	}
	o := c.options
	if o.dialect == nil {
		if o.dialectName != "" {
			return nil, fmt.Errorf("database start: dialect %q is not registered; register the adapter before WithDialectFromRegistry or use WithDialect", o.dialectName)
		}
		return nil, errors.New("database start: dialect is not configured; use WithDialectFromRegistry or WithDialect")
	}
	db, err := NewWithContext(ctx, o.dialect, cfg, c.log, o.connect...)
	if err != nil {
		return nil, fmt.Errorf("database start: %w", err)
	}
	if err := prepare(ctx, db, cfg, o); err != nil {
		return nil, errors.Join(err, db.Close()) //nolint:contextcheck // Close owns a bounded cleanup without a request context
	}
	return db, nil
}

func prepare(ctx context.Context, db *DB, cfg Config, o componentOptions) error {
	if cfg.AutoMigrate && len(o.models) > 0 {
		if err := db.AutoMigrate(ctx, o.models...); err != nil {
			return fmt.Errorf("database auto-migrate: %w", err)
		}
	}
	if o.startup != nil {
		if err := o.startup(ctx, db); err != nil {
			return fmt.Errorf("database startup check: %w", err)
		}
	}
	if o.initialize != nil {
		if err := o.initialize(ctx, db); err != nil {
			return fmt.Errorf("database initialize: %w", err)
		}
	}
	if o.readiness != nil {
		if err := o.readiness(ctx, db); err != nil {
			return fmt.Errorf("database readiness: %w", err)
		}
	}
	return nil
}

// Stop prevents further starts and access, waits within ctx for an in-flight Start to observe it, then closes the
// published pool once. Repeated calls return the recorded close result, including a failure to close the pool of a
// Start that Stop prevented from publishing.
func (c *Component) Stop(ctx context.Context) error {
	c.mu.Lock()
	if c.stopped {
		done := c.stopDone
		c.mu.Unlock()
		select {
		case <-done:
			return c.recordedStop()
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	c.stopped = true
	c.stopDone = make(chan struct{})
	inflight := c.starting
	c.mu.Unlock()

	var waitErr error
	if inflight != nil {
		select {
		case <-inflight:
		case <-ctx.Done():
			waitErr = ctx.Err()
		}
	}
	c.mu.Lock()
	db, lateErr := c.db, c.lateErr
	c.db = nil
	c.mu.Unlock()
	closeErr := lateErr
	if db != nil {
		closeErr = errors.Join(closeErr, db.Close()) //nolint:contextcheck // Close is invoked from lifecycle Stop without a request context
	}
	c.mu.Lock()
	c.stopErr = errors.Join(waitErr, closeErr)
	close(c.stopDone)
	c.mu.Unlock()
	return c.recordedStop()
}

func (c *Component) recordedStop() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stopErr
}

// Health pings the published pool and reruns the readiness check within the health budget. If Config.Enabled is false, it returns StatusHealthy with a "disabled" message. Messages carry safe classifications, never driver text.
func (c *Component) Health(ctx context.Context) component.Health {
	name, enabled, readiness := c.options.name, c.cfg.Enabled, c.options.readiness
	c.mu.Lock()
	db, stopped := c.db, c.stopped
	c.mu.Unlock()
	switch {
	case !enabled:
		return component.Health{Name: name, Status: component.StatusHealthy, Message: "disabled"}
	case stopped:
		return component.Health{Name: name, Status: component.StatusUnhealthy, Message: "database stopped"}
	case db == nil:
		return component.Health{Name: name, Status: component.StatusUnhealthy, Message: "database not initialized"}
	}
	ctx, cancel := context.WithTimeout(ctx, c.options.healthTimeout)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return component.Health{Name: name, Status: component.StatusUnhealthy, Message: "ping failed: " + safeMessage(err)}
	}
	if readiness != nil {
		if err := readiness(ctx, db); err != nil {
			return component.Health{Name: name, Status: component.StatusUnhealthy, Message: "not ready: " + safeMessage(err)}
		}
	}
	return component.Health{Name: name, Status: component.StatusHealthy}
}

// safeMessage reports a normalized classification without its cause, which may hold driver diagnostics.
func safeMessage(err error) string {
	app := apperrors.Normalize(err)
	return fmt.Sprintf("%s: %s", app.Code, app.Message)
}

// Describe returns infrastructure summary info for the bootstrap display.
func (c *Component) Describe() component.Description {
	backend := c.options.dialectName
	if c.options.dialect != nil {
		backend = c.options.dialect.Name()
	}
	details := fmt.Sprintf("Backend: %s, MaxConns: %d", backend, c.cfg.MaxOpenConns)
	if c.cfg.AutoMigrate {
		details += ", auto-migrate=on"
	}
	// A process with several databases names them distinctly (for example
	// "tenancy-database"); the unnamed default shows as a friendly "Database".
	displayName := "Database"
	if c.options.name != "database" {
		displayName = c.options.name
	}
	return component.Description{
		Name:    displayName,
		Type:    "database",
		Details: details,
	}
}
