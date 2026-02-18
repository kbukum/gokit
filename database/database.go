// Package database provides a database wrapper built on GORM
// with connection pooling, health checks, transactions, and auto-migration.
// Use Component.WithDriver() to specify the database driver (postgres, mysql, sqlite, etc).
package database

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/kbukum/gokit/logger"
)

// DB wraps a GORM database with gokit logging.
type DB struct {
	GormDB *gorm.DB
	log    *logger.Logger
	cfg    Config
	closed bool
	mu     sync.Mutex
}

// New opens a database connection with retry logic and connection pooling.
// For most use cases, use Component instead which provides driver flexibility via WithDriver().
// This function is kept for direct instantiation when not using the component framework.
func New(cfg Config, log *logger.Logger, dialector gorm.Dialector) (*DB, error) {
	return NewWithContext(context.Background(), dialector, cfg, log)
}

// NewWithDialector creates a database connection using a provided dialector.
//
// Deprecated: Use NewWithContext for better control, or use Component for lifecycle management.
func NewWithDialector(dialector interface{}, cfg Config, log *logger.Logger) (*DB, error) {
	return NewWithContext(context.Background(), dialector, cfg, log)
}

// NewWithContext creates a database connection with context-aware retry logic.
// The context allows cancellation of connection attempts during retries.
func NewWithContext(ctx context.Context, dialector interface{}, cfg Config, log *logger.Logger) (*DB, error) {
	cfg.ApplyDefaults()

	slowThreshold, _ := time.ParseDuration(cfg.SlowQueryThreshold)
	logLevel := parseLogLevel(cfg.LogLevel)

	gormCfg := &gorm.Config{
		Logger: newGormLogger(log, slowThreshold, logLevel),
	}

	d, ok := dialector.(gorm.Dialector)
	if !ok {
		return nil, fmt.Errorf("invalid dialector type: expected gorm.Dialector, got %T", dialector)
	}

	var db *gorm.DB
	var err error

	// Retry loop with context-aware backoff
	for attempt := 1; attempt <= cfg.MaxRetries; attempt++ {
		// Check if context is canceled before attempting connection
		if ctx.Err() != nil {
			return nil, fmt.Errorf("database connection canceled: %w", ctx.Err())
		}

		db, err = gorm.Open(d, gormCfg)
		if err == nil {
			// Successfully connected, verify with ping
			sqlDB, sqlErr := db.DB()
			if sqlErr != nil {
				err = sqlErr
				log.Warn("Failed to get underlying sql.DB", map[string]interface{}{
					"error":   sqlErr.Error(),
					"attempt": attempt,
				})
			} else if pingErr := sqlDB.PingContext(ctx); pingErr != nil {
				err = pingErr
				log.Warn("Database ping failed", map[string]interface{}{
					"error":   pingErr.Error(),
					"attempt": attempt,
				})
			} else {
				// Connection successful, configure pool
				sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
				sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
				if lifetime, parseErr := time.ParseDuration(cfg.ConnMaxLifetime); parseErr == nil {
					sqlDB.SetConnMaxLifetime(lifetime)
				}
				if idleTime, parseErr := time.ParseDuration(cfg.ConnMaxIdleTime); parseErr == nil {
					sqlDB.SetConnMaxIdleTime(idleTime)
				}

				log.Info("Database connection established", map[string]interface{}{
					"attempt": attempt,
				})
				return &DB{GormDB: db, log: log, cfg: cfg}, nil
			}
		}

		// Connection failed, wait before retry (unless last attempt)
		if attempt < cfg.MaxRetries {
			backoff := time.Duration(attempt) * time.Second
			log.Warn("Database connection attempt failed, retrying", map[string]interface{}{
				"attempt": attempt,
				"error":   err.Error(),
				"backoff": backoff.String(),
			})

			// Context-aware sleep - returns early if context is canceled
			if waitErr := contextSleep(ctx, backoff); waitErr != nil {
				return nil, fmt.Errorf("database connection canceled during retry: %w", waitErr)
			}
		}
	}

	return nil, fmt.Errorf("failed to connect to database after %d attempts: %w", cfg.MaxRetries, err)
}

// contextSleep waits for the given duration or until context is canceled.
// This allows retry backoff to be interrupted if the context is canceled.
func contextSleep(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
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
	d.log.Info("Closing database connection")
	d.closed = true
	return sqlDB.Close()
}

// Ping verifies the database connection is alive.
func (d *DB) Ping() error {
	sqlDB, err := d.GormDB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Ping()
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
func (d *DB) AutoMigrate(models ...interface{}) error {
	d.log.Info("Running auto-migration", map[string]interface{}{
		"models": len(models),
	})
	for _, model := range models {
		if err := d.GormDB.AutoMigrate(model); err != nil {
			return fmt.Errorf("failed to migrate %T: %w", model, err)
		}
	}
	d.log.Info("Auto-migration completed")
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
			d.log.Error("Transaction rolled back due to panic", map[string]interface{}{
				"panic": fmt.Sprintf("%v", r),
			})
			panic(r)
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback().Error; rbErr != nil {
			return fmt.Errorf("transaction failed: %w, rollback failed: %v", err, rbErr)
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

// --- GORM logger adapter ---

// parseLogLevel converts a string log level to GORM's LogLevel.
func parseLogLevel(level string) gormlogger.LogLevel {
	switch strings.ToLower(level) {
	case "silent":
		return gormlogger.Silent
	case "error":
		return gormlogger.Error
	case "warn":
		return gormlogger.Warn
	default:
		return gormlogger.Info
	}
}

type gormLoggerAdapter struct {
	log           *logger.Logger
	logLevel      gormlogger.LogLevel
	slowThreshold time.Duration
}

func newGormLogger(log *logger.Logger, slowThreshold time.Duration, logLevel gormlogger.LogLevel) gormlogger.Interface {
	return &gormLoggerAdapter{
		log:           log.WithComponent("gorm"),
		logLevel:      logLevel,
		slowThreshold: slowThreshold,
	}
}

func (l *gormLoggerAdapter) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	return &gormLoggerAdapter{log: l.log, logLevel: level, slowThreshold: l.slowThreshold}
}

func (l *gormLoggerAdapter) Info(_ context.Context, msg string, data ...interface{}) {
	if l.logLevel >= gormlogger.Info {
		l.log.Info(fmt.Sprintf(msg, data...))
	}
}

func (l *gormLoggerAdapter) Warn(_ context.Context, msg string, data ...interface{}) {
	if l.logLevel >= gormlogger.Warn {
		l.log.Warn(fmt.Sprintf(msg, data...))
	}
}

func (l *gormLoggerAdapter) Error(_ context.Context, msg string, data ...interface{}) {
	if l.logLevel >= gormlogger.Error {
		l.log.Error(fmt.Sprintf(msg, data...))
	}
}

func (l *gormLoggerAdapter) Trace(_ context.Context, begin time.Time, fc func() (string, int64), err error) {
	if l.logLevel <= gormlogger.Silent {
		return
	}

	elapsed := time.Since(begin)
	sql, rows := fc()

	switch {
	case err != nil && err != gorm.ErrRecordNotFound:
		l.log.Error("Query error", map[string]interface{}{
			"sql": sql, "duration": elapsed.String(), "rows": rows, "error": err.Error(),
		})
	case elapsed > l.slowThreshold:
		l.log.Warn("Slow query", map[string]interface{}{
			"sql": sql, "duration": elapsed.String(), "rows": rows,
		})
	case l.logLevel >= gormlogger.Info:
		l.log.Debug("Query", map[string]interface{}{
			"sql": sql, "duration": elapsed.String(), "rows": rows,
		})
	}
}
