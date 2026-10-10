package database

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/resilience"
	"github.com/kbukum/gokit/util"
)

// DB wraps a GORM database with gokit logging.
type DB struct {
	GormDB   *gorm.DB
	reader   *gorm.DB
	queryLog *gormLoggerAdapter
	log      *logging.Logger
	cfg      Config
	closed   bool
	closeErr error
	mu       sync.Mutex
}

// Option customizes how a database connection is opened.
type Option func(*connectOptions)

type connectOptions struct {
	policy  *resilience.Policy
	secrets SecretSource
}

// WithSecretSource injects credential resolution into backend preparation.
func WithSecretSource(source SecretSource) Option {
	return func(o *connectOptions) { o.secrets = source }
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
func New(cfg Config, log *logging.Logger, dialect Dialect, opts ...Option) (*DB, error) {
	return NewWithContext(context.Background(), dialect, cfg, log, opts...)
}

// NewWithContext creates a database connection with context-aware retry logic. Connection attempts
// run through a resilience.Policy (canonical retry/backoff/timeout owner) rather than a bespoke
// loop; the context cancels attempts and their backoff waits.
func NewWithContext(ctx context.Context, dialect Dialect, cfg Config, log *logging.Logger, opts ...Option) (*DB, error) {
	if util.IsNil(ctx) || util.IsNil(dialect) || log == nil {
		return nil, apperrors.InvalidInput("connection", "Context, dialect and logger are required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cfg.ApplyDefaults()
	if err := cfg.validateConnection(); err != nil {
		return nil, err
	}

	slowThreshold, _ := time.ParseDuration(cfg.SlowQueryThreshold)
	logLevel := parseLogLevel(cfg.LogLevel)

	queryLog := newGormLogger(log, slowThreshold, logLevel)
	gormConfig := func() *gorm.Config {
		return &gorm.Config{Logger: queryLog, DisableAutomaticPing: true}
	}

	options := connectOptions{}
	for _, opt := range opts {
		if opt == nil {
			return nil, apperrors.InvalidInput("option", "Connection option is required")
		}
		opt(&options)
	}
	connectTimeout, _ := time.ParseDuration(cfg.ConnectTimeout)
	opener, err := prepareConnection(ctx, dialect, ConnectionInput{DSN: cfg.DSN, Params: cfg.Params, Secrets: options.secrets}, connectTimeout)
	if err != nil {
		return nil, err
	}
	if util.IsNil(opener) {
		return nil, apperrors.InvalidInput("opener", "Dialect returned a nil opener")
	}
	policy := options.policy
	if policy == nil {
		policy = defaultConnectPolicy(ctx, cfg, log)
	}

	// A per-attempt deadline bounds each connection attempt so a blackholed endpoint cannot leave
	// New(context.Background(), …) blocked for the driver's unbounded duration; the retry budget
	// then advances between attempts. Zero disables the per-attempt bound.
	attempt := 0
	var d gorm.Dialector
	db, err := resilience.Execute(ctx, policy, func(attemptCtx context.Context) (*gorm.DB, error) {
		attempt++
		if connectTimeout > 0 {
			var cancel context.CancelFunc
			attemptCtx, cancel = context.WithTimeout(attemptCtx, connectTimeout)
			defer cancel()
		}
		dialector, openErr := opener.Open(attemptCtx)
		if openErr != nil {
			return nil, Failure(openErr)
		}
		if util.IsNil(dialector) {
			return nil, apperrors.InvalidInput("dialector", "Opener returned a nil dialector")
		}
		connected, connectErr := connectOnce(attemptCtx, dialector, gormConfig(), cfg)
		if connectErr == nil {
			d = dialector
		}
		return connected, connectErr
	})
	if err != nil {
		// Only the caller's context is a cancellation; a spent per-attempt budget stays a classified connect failure.
		if failure := apperrors.FromContext(ctx, "database.connect"); failure != nil {
			return nil, failure
		}
		return nil, fmt.Errorf("failed to connect to database after %d attempts: %w", attempt, err)
	}

	result := &DB{GormDB: db, log: log, cfg: cfg, queryLog: queryLog}
	if split, ok := d.(ReadPoolDialect); ok {
		if reader := split.ReadDialector(); reader != nil {
			result.reader, err = connectOnce(ctx, reader, gormConfig(), cfg)
			if err != nil {
				return nil, errors.Join(err, result.Close())
			}
		}
	}
	log.InfoCtx(ctx, "Database connection established", map[string]any{"attempt": attempt})
	return result, nil
}

func prepareConnection(ctx context.Context, dialect Dialect, input ConnectionInput, timeout time.Duration) (Opener, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	opener, err := dialect.Prepare(ctx, input)
	err = errors.Join(err, ctx.Err())
	if err != nil {
		return nil, Failure(err)
	}
	return opener, nil
}

// defaultConnectPolicy builds the connection retry policy from Config.MaxRetries. It reuses the
// canonical jittered exponential backoff defaults and logs each retry through the injected logger.
func defaultConnectPolicy(ctx context.Context, cfg Config, log *logging.Logger) *resilience.Policy {
	retry := resilience.DefaultRetryConfig()
	retry.MaxAttempts = cfg.MaxRetries
	retry.RetryIf = func(err error) bool { return ctx.Err() == nil && !permanentConnectFailure(err) }
	retry.OnRetry = func(attempt int, err error, backoff time.Duration) {
		log.WarnCtx(ctx, "Database connection attempt failed, retrying", map[string]any{
			"attempt": attempt,
			"error":   err.Error(),
			"backoff": backoff.String(),
		})
	}
	return resilience.NewPolicy().WithRetry(retry)
}

// permanentConnectFailure reports failures another attempt cannot fix: rejected configuration, SQLSTATE class 28
// (invalid authorization) or 3D (unknown catalog), and certificate verification. A spent per-attempt budget is
// transient and retried.
func permanentConnectFailure(err error) bool {
	if app, ok := err.(*apperrors.AppError); ok && app.Code == apperrors.ErrCodeInvalidInput { //nolint:errorlint // only an unwrapped validation result is a local rejection
		return true
	}
	var state interface{ SQLState() string }
	if errors.As(err, &state) {
		if code := state.SQLState(); strings.HasPrefix(code, "28") || strings.HasPrefix(code, "3D") {
			return true
		}
	}
	var (
		verification *tls.CertificateVerificationError
		authority    x509.UnknownAuthorityError
		hostname     x509.HostnameError
		invalid      x509.CertificateInvalidError
	)
	return errors.As(err, &verification) || errors.As(err, &authority) || errors.As(err, &hostname) || errors.As(err, &invalid)
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
) (*gorm.DB, error) {
	db, err := gorm.Open(d, gormCfg)
	if err != nil {
		if owner, ok := d.(interface{ Close() error }); ok {
			err = errors.Join(err, owner.Close())
		} else if db != nil {
			if pool, poolErr := db.DB(); poolErr == nil {
				err = errors.Join(err, pool.Close())
			} else {
				err = errors.Join(err, poolErr)
			}
		}
		return nil, Failure(err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		if owner, ok := d.(interface{ Close() error }); ok {
			err = errors.Join(err, owner.Close())
		}
		return nil, Failure(err)
	}

	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, Failure(errors.Join(err, sqlDB.Close()))
	}

	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	if lifetime, parseErr := time.ParseDuration(cfg.ConnMaxLifetime); parseErr == nil {
		sqlDB.SetConnMaxLifetime(lifetime)
	}
	if idleTime, parseErr := time.ParseDuration(cfg.ConnMaxIdleTime); parseErr == nil {
		sqlDB.SetConnMaxIdleTime(idleTime)
	}
	if poolConfig, ok := d.(PoolConfigurer); ok {
		poolConfig.ConfigurePool(sqlDB)
	}
	return db, nil
}

// Close closes the underlying sql.DB connection pool. Safe to call multiple times.
func (d *DB) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return d.closeErr
	}

	sqlDB, err := d.GormDB.DB()
	if err != nil {
		return Failure(err)
	}
	d.log.Debug("Closing database connection") //nolint:contextcheck // Close is invoked from lifecycle Stop without a request context
	d.closed = true
	err = sqlDB.Close()
	if d.reader != nil {
		readDB, readErr := d.reader.DB()
		if readErr != nil {
			d.closeErr = Failure(errors.Join(err, readErr))
			return d.closeErr
		}
		err = errors.Join(err, readDB.Close())
	}
	if err != nil {
		d.closeErr = Failure(err)
	}
	return d.closeErr
}

// PingContext verifies the database connection is alive, respecting the context.
func (d *DB) PingContext(ctx context.Context) error {
	sqlDB, err := d.GormDB.DB()
	if err != nil {
		return err
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		return Failure(err)
	}
	return nil
}

// WithContext returns a GORM session scoped to the given context.
func (d *DB) WithContext(ctx context.Context) *gorm.DB {
	return d.GormDB.WithContext(ctx)
}

// AutoMigrate runs context-bound GORM schema migration. Models are opaque GORM model definitions.
func (d *DB) AutoMigrate(ctx context.Context, models ...any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	d.log.InfoCtx(ctx, "Running auto-migration", map[string]any{
		"models": len(models),
	})
	for _, model := range models {
		if err := d.GormDB.WithContext(ctx).AutoMigrate(model); err != nil {
			return fmt.Errorf("failed to migrate %T: %w", model, err)
		}
	}
	d.log.InfoCtx(ctx, "Auto-migration completed")
	return nil
}
