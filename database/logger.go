package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/kbukum/gokit/logging"
)

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
	log           *logging.Logger
	logLevel      gormlogger.LogLevel
	slowThreshold time.Duration
}

func newGormLogger(log *logging.Logger, slowThreshold time.Duration, logLevel gormlogger.LogLevel) gormlogger.Interface {
	return &gormLoggerAdapter{
		log:           log.WithComponent("gorm"),
		logLevel:      logLevel,
		slowThreshold: slowThreshold,
	}
}

func (l *gormLoggerAdapter) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	return &gormLoggerAdapter{log: l.log, logLevel: level, slowThreshold: l.slowThreshold}
}

func (l *gormLoggerAdapter) Info(ctx context.Context, msg string, data ...interface{}) {
	if l.logLevel >= gormlogger.Info {
		l.log.InfoCtx(ctx, fmt.Sprintf(msg, data...))
	}
}

func (l *gormLoggerAdapter) Warn(ctx context.Context, msg string, data ...interface{}) {
	if l.logLevel >= gormlogger.Warn {
		l.log.WarnCtx(ctx, fmt.Sprintf(msg, data...))
	}
}

func (l *gormLoggerAdapter) Error(ctx context.Context, msg string, data ...interface{}) {
	if l.logLevel >= gormlogger.Error {
		l.log.ErrorCtx(ctx, fmt.Sprintf(msg, data...))
	}
}

func (l *gormLoggerAdapter) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if l.logLevel <= gormlogger.Silent {
		return
	}

	elapsed := time.Since(begin)
	sql, rows := fc()

	switch {
	case err != nil && !errors.Is(err, gorm.ErrRecordNotFound) && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded):
		l.log.ErrorCtx(ctx, "Query error", map[string]interface{}{
			"sql": sql, "duration": elapsed.String(), "rows": rows, "error": err.Error(),
		})
	case err != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)):
		// Client disconnected / request timed out — log at debug only.
		l.log.DebugCtx(ctx, "Query canceled", map[string]interface{}{
			"sql": sql, "duration": elapsed.String(), "error": err.Error(),
		})
	case elapsed > l.slowThreshold:
		l.log.WarnCtx(ctx, "Slow query", map[string]interface{}{
			"sql": sql, "duration": elapsed.String(), "rows": rows,
		})
	case l.logLevel >= gormlogger.Info:
		l.log.DebugCtx(ctx, "Query", map[string]interface{}{
			"sql": sql, "duration": elapsed.String(), "rows": rows,
		})
	}
}
