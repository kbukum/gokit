package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/kbukum/gokit/logger"
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
