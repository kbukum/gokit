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

// HealthTimeout bounds one Health call, including the ping and the injected readiness check.
const HealthTimeout = 500 * time.Millisecond

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

// Component wraps DB and implements component.Component for lifecycle management. Configure it before Start; the
// lifecycle methods and DB are safe for concurrent use.
//
// Start opens the pool, runs auto-migration, the Initialize check and the Readiness check, then publishes the pool.
// A failed Start closes its pool and may be retried; a successful Start is idempotent. Stop is terminal, prevents a
// concurrent Start from publishing and records its close result. Health pings and reruns Readiness within
// [HealthTimeout], so schema or access drift after startup reports unhealthy.
type Component struct {
	mu          sync.Mutex
	db          *DB
	starting    chan struct{}
	stopped     bool
	stopDone    chan struct{}
	stopErr     error
	lateErr     error
	cfg         Config
	log         *logging.Logger
	name        string
	models      []any
	dialect     Dialect
	dialectName string
	secrets     SecretSource
	initialize  Check
	readiness   Check
}

// NewComponent creates a database component named "database" for use with the component registry.
// Dialects are opt-in: call WithDialect or WithDialectFromRegistry before Start.
// The Config.Enabled flag can be used to skip initialization at runtime.
func NewComponent(cfg Config, log *logging.Logger) *Component {
	return &Component{
		cfg:  cfg,
		log:  log.WithComponent("database"),
		name: "database",
	}
}

// WithName sets the unique registry name, so one process can own several database components.
func (c *Component) WithName(name string) *Component {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.name = name
	return c
}

// WithSecretSource injects explicit credential resolution during Start.
func (c *Component) WithSecretSource(source SecretSource) *Component {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.secrets = source
	return c
}

// WithInitialize injects a startup-only check that runs after auto-migration and before Readiness, such as
// verifying persisted installation state. Its failure fails Start.
func (c *Component) WithInitialize(check Check) *Component {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.initialize = check
	return c
}

// WithReadiness injects a read-only check, such as schema version and access readiness, that gates publication and
// every Health call. It must not take locks, run DDL or migrate.
func (c *Component) WithReadiness(check Check) *Component {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.readiness = check
	return c
}

// WithDialect sets the database backend dialect directly.
//
// Example:
//
//	import "github.com/kbukum/gokit/database/postgres"
//
//	db := database.NewComponent(cfg, log).
//		WithDialect(postgres.Dialect()).
//		WithAutoMigrate(&User{}, &Post{})
func (c *Component) WithDialect(d Dialect) *Component {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dialect = d
	c.dialectName = ""
	return c
}

// WithDialectFromRegistry selects a registered dialect by backend name.
func (c *Component) WithDialectFromRegistry(reg *DialectRegistry, name string) *Component {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dialectName = name
	c.dialect = nil
	if reg == nil {
		return c
	}
	if d, ok := reg.Get(name); ok {
		c.dialect = d
	}
	return c
}

// WithAutoMigrate registers models for auto-migration on Start.
// Models are only migrated if Config.AutoMigrate is true and the component is enabled.
func (c *Component) WithAutoMigrate(models ...any) *Component {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.models = append(c.models, models...)
	return c
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
func (c *Component) Name() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.name
}

type startPlan struct {
	cfg        Config
	dialect    Dialect
	name       string
	models     []any
	secrets    SecretSource
	initialize Check
	readiness  Check
}

// Start connects to the database, runs auto-migration and the injected checks, then publishes the pool. If
// Config.Enabled is false, this method returns immediately without error. The context bounds connection retries
// and the checks. A concurrent Start waits for the in-flight attempt.
func (c *Component) Start(ctx context.Context) error {
	plan, done, err := c.beginStart(ctx)
	if err != nil || done == nil {
		return err
	}
	defer c.finishStart(done)
	db, err := c.open(ctx, plan)
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

// beginStart claims the start slot. It returns a nil channel when no work is required.
func (c *Component) beginStart(ctx context.Context) (startPlan, chan struct{}, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for {
		switch {
		case c.stopped:
			return startPlan{}, nil, ErrStopped
		case c.db != nil:
			return startPlan{}, nil, nil
		case c.starting == nil:
			if !c.cfg.Enabled {
				c.log.InfoCtx(ctx, "Database component is disabled")
				return startPlan{}, nil, nil
			}
			c.starting = make(chan struct{})
			return startPlan{
				cfg: c.cfg, dialect: c.dialect, name: c.dialectName, models: append([]any(nil), c.models...),
				secrets: c.secrets, initialize: c.initialize, readiness: c.readiness,
			}, c.starting, nil
		}
		inflight := c.starting
		c.mu.Unlock()
		select {
		case <-inflight:
		case <-ctx.Done():
			c.mu.Lock()
			return startPlan{}, nil, ctx.Err()
		}
		c.mu.Lock()
	}
}

// open performs one start attempt without holding the lifecycle lock and closes the pool on any failure.
func (c *Component) open(ctx context.Context, plan startPlan) (*DB, error) {
	// Apply defaults and validate before doing any work so a misconfigured component fails fast instead of
	// attempting to connect with a zero-value input.
	plan.cfg.ApplyDefaults()
	if err := plan.cfg.Validate(); err != nil {
		return nil, fmt.Errorf("database start: %w", err)
	}
	if plan.dialect == nil {
		if plan.name != "" {
			return nil, fmt.Errorf("database start: dialect %q is not configured; register the adapter and call WithDialectFromRegistry or call WithDialect directly", plan.name)
		}
		return nil, errors.New("database start: dialect is not configured; register an adapter and call WithDialectFromRegistry or call WithDialect directly")
	}
	db, err := NewWithContext(ctx, plan.dialect, plan.cfg, c.log, WithSecretSource(plan.secrets))
	if err != nil {
		return nil, fmt.Errorf("database start: %w", err)
	}
	if err := prepare(ctx, db, plan); err != nil {
		return nil, errors.Join(err, db.Close()) //nolint:contextcheck // Close owns a bounded cleanup without a request context
	}
	return db, nil
}

func prepare(ctx context.Context, db *DB, plan startPlan) error {
	if plan.cfg.AutoMigrate && len(plan.models) > 0 {
		if err := db.AutoMigrate(ctx, plan.models...); err != nil {
			return fmt.Errorf("database auto-migrate: %w", err)
		}
	}
	if plan.initialize != nil {
		if err := plan.initialize(ctx, db); err != nil {
			return fmt.Errorf("database initialize: %w", err)
		}
	}
	if plan.readiness != nil {
		if err := plan.readiness(ctx, db); err != nil {
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

// Health pings the published pool and reruns the readiness check within [HealthTimeout]. If Config.Enabled is false,
// it returns StatusHealthy with a "disabled" message. Messages carry safe classifications, never driver text.
func (c *Component) Health(ctx context.Context) component.Health {
	c.mu.Lock()
	name, enabled, db, stopped, readiness := c.name, c.cfg.Enabled, c.db, c.stopped, c.readiness
	c.mu.Unlock()
	switch {
	case !enabled:
		return component.Health{Name: name, Status: component.StatusHealthy, Message: "disabled"}
	case stopped:
		return component.Health{Name: name, Status: component.StatusUnhealthy, Message: "database stopped"}
	case db == nil:
		return component.Health{Name: name, Status: component.StatusUnhealthy, Message: "database not initialized"}
	}
	ctx, cancel := context.WithTimeout(ctx, HealthTimeout)
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
	c.mu.Lock()
	defer c.mu.Unlock()
	backend := c.dialectName
	if c.dialect != nil {
		backend = c.dialect.Name()
	}
	details := fmt.Sprintf("Backend: %s, MaxConns: %d", backend, c.cfg.MaxOpenConns)
	if c.cfg.AutoMigrate {
		details += ", auto-migrate=on"
	}
	return component.Description{
		Name:    "Database",
		Type:    "database",
		Details: details,
	}
}
