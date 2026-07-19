package database

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func TestParseLogLevel(t *testing.T) {
	t.Parallel()
	tests := map[string]gormlogger.LogLevel{
		"silent": gormlogger.Silent,
		"error":  gormlogger.Error,
		"warn":   gormlogger.Warn,
		"info":   gormlogger.Info,
		"debug":  gormlogger.Info,
		"WARN":   gormlogger.Warn,
	}
	for input, want := range tests {
		if got := parseLogLevel(input); got != want {
			t.Fatalf("parseLogLevel(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestGormLoggerAdapterMethods(t *testing.T) {
	log := newGormLogger(testLogger(), time.Nanosecond, gormlogger.Info)
	ctx := context.Background()
	log.Info(ctx, "hello %s", "world")
	log.Warn(ctx, "warn %d", 1)
	log.Error(ctx, "error %v", "x")

	if log.LogMode(gormlogger.Silent) == log {
		t.Fatal("LogMode should return a new logger")
	}
	log.Trace(ctx, time.Now().Add(-time.Millisecond), func() (string, int64) { return "SELECT 1", 1 }, nil)
	log.Trace(ctx, time.Now(), func() (string, int64) { return "SELECT 1", 0 }, gorm.ErrRecordNotFound)
	log.Trace(ctx, time.Now(), func() (string, int64) { return "SELECT 1", 0 }, context.Canceled)
	log.Trace(ctx, time.Now(), func() (string, int64) { return "SELECT 1", 0 }, errors.New("boom"))
	log.LogMode(gormlogger.Silent).Trace(ctx, time.Now(), func() (string, int64) { return "SELECT 1", 0 }, nil)
}

// TestGormLoggerTraceFastQueryLogsAtDebug covers the non-slow, no-error path where a query
// under the slow threshold is traced at debug level.
func TestGormLoggerTraceFastQueryLogsAtDebug(t *testing.T) {
	log := newGormLogger(testLogger(), time.Hour, gormlogger.Info)
	log.Trace(context.Background(), time.Now(), func() (string, int64) { return "SELECT 1", 1 }, nil)
}
