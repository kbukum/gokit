package database

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/resilience"
)

// DB wraps a GORM database with gokit logging.
type DB struct {
	GormDB *gorm.DB
	log    *logging.Logger
	cfg    Config
	closed bool
	mu     sync.Mutex
}

// Option customizes how a database connection is opened.
type Option func(*connectOptions)

type connectOptions struct {
	policy *resilience.Policy
}

// WithConnectPolicy injects the resilience policy that governs connection attempts (retry,
// backoff, timeout, and circuit-breaking). When unset, a default retry policy derived from
// Config.MaxRetries is used. Passing a policy lets callers share one canonical policy across
// remote calls instead of the component maintaining its own retry loop.
func WithConnectPolicy(p *resilience.Policy) Option {
	return func(o *connectOptions) { o.policy = p }
}

// New opens a database connection with retry logic and connection pooling. For most use cases,
// use Component instead which provides backend flexibility via WithDialect().
func New(cfg Config, log *logging.Logger, dialector gorm.Dialector, opts ...Option) (*DB, error) {
	return NewWithContext(context.Background(), dialector, cfg, log, opts...)
}

// NewWithContext creates a database connection with context-aware retry logic. Connection attempts
// run through a resilience.Policy (canonical retry/backoff/timeout owner) rather than a bespoke
// loop; the context cancels attempts and their backoff waits.
func NewWithContext(ctx context.Context, dialector any, cfg Config, log *logging.Logger, opts ...Option) (*DB, error) {
	cfg.ApplyDefaults()

	slowThreshold, _ := time.ParseDuration(cfg.SlowQueryThreshold)
	logLevel := parseLogLevel(cfg.LogLevel)

	gormCfg := &gorm.Config{
		Logger: newGormLogger(log, slowThreshold, logLevel),
		// connectOnce owns the sole, context-cancellable liveness check via PingContext. Disabling
		// GORM's own context-free Ping keeps a stalled server from blocking uncancellably inside
		// gorm.Open and ensures the pool-cleanup branch here is the one that closes failed pools.
		DisableAutomaticPing: true,
	}

	d, ok := dialector.(gorm.Dialector)
	if !ok {
		return nil, fmt.Errorf("invalid dialector type: expected gorm.Dialector, got %T", dialector)
	}

	options := connectOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	policy := options.policy
	if policy == nil {
		policy = defaultConnectPolicy(ctx, cfg, log)
	}

	// A per-attempt deadline bounds each connection attempt so a blackholed endpoint cannot leave
	// New(context.Background(), …) blocked for the driver's unbounded duration; the retry budget
	// then advances between attempts. Zero disables the per-attempt bound.
	connectTimeout, _ := time.ParseDuration(cfg.ConnectTimeout)

	attempt := 0
	db, err := resilience.Execute(ctx, policy, func(attemptCtx context.Context) (*gorm.DB, error) {
		attempt++
		if connectTimeout > 0 {
			var cancel context.CancelFunc
			attemptCtx, cancel = context.WithTimeout(attemptCtx, connectTimeout)
			defer cancel()
		}
		return connectOnce(attemptCtx, d, gormCfg, cfg, log, attempt)
	})
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("database connection canceled: %w", ctxErr)
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("database connection canceled: %w", err)
		}
		return nil, fmt.Errorf("failed to connect to database after %d attempts: %w", cfg.MaxRetries, err)
	}

	log.InfoCtx(ctx, "Database connection established", map[string]any{"attempt": attempt})
	return &DB{GormDB: db, log: log, cfg: cfg}, nil
}

// defaultConnectPolicy builds the connection retry policy from Config.MaxRetries. It reuses the
// canonical jittered exponential backoff defaults and logs each retry through the injected logger.
func defaultConnectPolicy(ctx context.Context, cfg Config, log *logging.Logger) *resilience.Policy {
	retry := resilience.DefaultRetryConfig()
	retry.MaxAttempts = cfg.MaxRetries
	retry.OnRetry = func(attempt int, err error, backoff time.Duration) {
		log.WarnCtx(ctx, "Database connection attempt failed, retrying", map[string]any{
			"attempt": attempt,
			"error":   err.Error(),
			"backoff": backoff.String(),
		})
	}
	return resilience.NewPolicy().WithRetry(retry)
}

// connectOnce performs a single connection attempt: open the dialector, verify it with a ping,
// and configure the pool. gorm.Open builds the *sql.DB pool lazily and returns nil even when the
// server is unreachable, so the failure only surfaces at ping time; on any failure after the pool
// exists it is closed so a failed attempt never leaks connections across retries.
func connectOnce(
	ctx context.Context,
	d gorm.Dialector,
	gormCfg *gorm.Config,
	cfg Config,
	log *logging.Logger,
	attempt int,
) (*gorm.DB, error) {
	db, err := gorm.Open(d, gormCfg)
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		log.WarnCtx(ctx, "Failed to get underlying sql.DB", map[string]any{
			"error":   err.Error(),
			"attempt": attempt,
		})
		return nil, err
	}

	if err := sqlDB.PingContext(ctx); err != nil {
		if closeErr := sqlDB.Close(); closeErr != nil {
			log.WarnCtx(ctx, "Failed to close pool after ping failure", map[string]any{
				"error":   closeErr.Error(),
				"attempt": attempt,
			})
		}
		log.WarnCtx(ctx, "Database ping failed", map[string]any{
			"error":   err.Error(),
			"attempt": attempt,
		})
		return nil, err
	}

	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	if lifetime, parseErr := time.ParseDuration(cfg.ConnMaxLifetime); parseErr == nil {
		sqlDB.SetConnMaxLifetime(lifetime)
	}
	if idleTime, parseErr := time.ParseDuration(cfg.ConnMaxIdleTime); parseErr == nil {
		sqlDB.SetConnMaxIdleTime(idleTime)
	}
	return db, nil
}

// Close closes the underlying sql.DB connection pool. Safe to call multiple times.
func (d *DB) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return nil
	}

	sqlDB, err := d.GormDB.DB()
	if err != nil {
		return err
	}
	d.log.Debug("Closing database connection") //nolint:contextcheck // Close is invoked from lifecycle Stop without a request context
	d.closed = true
	return sqlDB.Close()
}

// PingContext verifies the database connection is alive, respecting the context.
func (d *DB) PingContext(ctx context.Context) error {
	sqlDB, err := d.GormDB.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

// WithContext returns a GORM session scoped to the given context.
func (d *DB) WithContext(ctx context.Context) *gorm.DB {
	return d.GormDB.WithContext(ctx)
}

// AutoMigrate runs GORM auto-migration for the given models.
func (d *DB) AutoMigrate(models ...any) error {
	d.log.Info("Running auto-migration", map[string]any{
		"models": len(models),
	})
	for _, model := range models {
		if err := d.GormDB.AutoMigrate(model); err != nil {
			return fmt.Errorf("failed to migrate %T: %w", model, err)
		}
	}
	d.log.Info("Auto-migration completed") //nolint:contextcheck // AutoMigrate is a synchronous schema operation without a request context
	return nil
}

// Transaction executes fn inside a database transaction.
func (d *DB) Transaction(fn func(*gorm.DB) error) error {
	return d.GormDB.Transaction(fn)
}

// TransactionFunc defines a function that runs within a transaction.
type TransactionFunc func(tx *gorm.DB) error

// WithTransaction executes fn within a transaction with panic recovery.
func (d *DB) WithTransaction(ctx context.Context, fn TransactionFunc) error {
	tx := d.GormDB.WithContext(ctx).Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			d.log.ErrorCtx(ctx, "Transaction rolled back due to panic", map[string]any{
				"panic": fmt.Sprintf("%v", r),
			})
			panic(r)
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback().Error; rbErr != nil {
			return fmt.Errorf("transaction failed: %w (rollback also failed: %w)", err, rbErr)
		}
		return err
	}

	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

// WithReadOnlyTransaction executes fn in a read-only transaction (always rolls back).
func (d *DB) WithReadOnlyTransaction(ctx context.Context, fn TransactionFunc) error {
	tx := d.GormDB.WithContext(ctx).Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to begin read-only transaction: %w", tx.Error)
	}
	defer tx.Rollback()

	return fn(tx)
}
