// Package database provides a PostgreSQL database wrapper built on GORM
// with connection pooling, health checks, transactions, and auto-migration.
package database

import (
	"context"
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/skillsenselab/gokit/logger"
)

// DB wraps a GORM database with gokit logging.
type DB struct {
	GormDB *gorm.DB
	log    *logger.Logger
	cfg    Config
}

// New opens a PostgreSQL connection with retry logic and connection pooling.
func New(cfg Config, log *logger.Logger) (*DB, error) {
	cfg.ApplyDefaults()

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("database config: %w", err)
	}

	if !cfg.Enabled {
		return nil, fmt.Errorf("database is disabled")
	}

	slowThreshold, _ := time.ParseDuration(cfg.SlowQueryThreshold) // already validated

	gormCfg := &gorm.Config{
		Logger: newGormLogger(log, slowThreshold),
	}

	var db *gorm.DB
	var err error

	for attempt := 1; attempt <= cfg.MaxRetries; attempt++ {
		db, err = gorm.Open(postgres.Open(cfg.DSN), gormCfg)
		if err == nil {
			sqlDB, sqlErr := db.DB()
			if sqlErr != nil {
				err = sqlErr
				log.Warn("Failed to get underlying sql.DB", map[string]interface{}{
					"error": sqlErr.Error(), "attempt": attempt,
				})
				if attempt < cfg.MaxRetries {
					time.Sleep(time.Duration(attempt) * time.Second)
				}
				continue
			}

			if pingErr := sqlDB.Ping(); pingErr != nil {
				err = pingErr
				log.Warn("Database ping failed", map[string]interface{}{
					"error": pingErr.Error(), "attempt": attempt,
				})
				if attempt < cfg.MaxRetries {
					time.Sleep(time.Duration(attempt) * time.Second)
				}
				continue
			}

			// Configure connection pool
			sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
			sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
			if lifetime, parseErr := time.ParseDuration(cfg.ConnMaxLifetime); parseErr == nil {
				sqlDB.SetConnMaxLifetime(lifetime)
			}

			log.Info("Database connection established", map[string]interface{}{
				"max_open_conns": cfg.MaxOpenConns,
				"max_idle_conns": cfg.MaxIdleConns,
				"attempts":       attempt,
			})

			return &DB{GormDB: db, log: log, cfg: cfg}, nil
		}

		log.Warn("Database connection attempt failed", map[string]interface{}{
			"error": err.Error(), "attempt": attempt, "max_retries": cfg.MaxRetries,
		})
		if attempt < cfg.MaxRetries {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
	}

	return nil, fmt.Errorf("failed to connect to database after %d attempts: %w", cfg.MaxRetries, err)
}

// Close closes the underlying sql.DB connection pool.
func (d *DB) Close() error {
	sqlDB, err := d.GormDB.DB()
	if err != nil {
		return err
	}
	d.log.Info("Closing database connection")
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

type gormLoggerAdapter struct {
	log           *logger.Logger
	logLevel      gormlogger.LogLevel
	slowThreshold time.Duration
}

func newGormLogger(log *logger.Logger, slowThreshold time.Duration) gormlogger.Interface {
	return &gormLoggerAdapter{
		log:           log.WithComponent("gorm"),
		logLevel:      gormlogger.Info,
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
