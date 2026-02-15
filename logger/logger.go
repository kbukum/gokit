package logger

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const (
	FormatPretty = "pretty"
	BooleanTrue  = "true"
)

// Logger wraps zerolog.Logger with additional context.
type Logger struct {
	logger  zerolog.Logger
	service string
}

// Init initializes the global logger from config.
func Init(cfg Config) {
	cfg.ApplyDefaults()
	globalLogger = New(&cfg, "default")

	level, _ := zerolog.ParseLevel(cfg.Level)
	zerolog.SetGlobalLevel(level)

	if cfg.Format == "console" || cfg.Format == FormatPretty {
		log.Logger = newConsoleLogger(&cfg, "default")
	}
}

// New creates a new logger instance with configuration.
func New(cfg *Config, serviceName string) *Logger {
	level, err := zerolog.ParseLevel(cfg.Level)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	output := outputWriter(cfg.Output)

	var zl zerolog.Logger
	if strings.ToLower(cfg.Format) == "console" || strings.ToLower(cfg.Format) == FormatPretty {
		zl = newConsoleLogger(cfg, serviceName)
	} else {
		zl = zerolog.New(output)
	}

	if cfg.Timestamp {
		zl = zl.With().Timestamp().Logger()
	}
	if cfg.Caller {
		zl = zl.With().Caller().Logger()
	}

	return &Logger{
		logger:  zl,
		service: serviceName,
	}
}

// NewDefault creates a logger with default configuration.
func NewDefault(serviceName string) *Logger {
	cfg := &Config{
		Level:     "info",
		Format:    "console",
		Output:    "stdout",
		NoColor:   false,
		Timestamp: true,
	}
	return New(cfg, serviceName)
}

// NewFromEnv creates a logger configured from environment variables.
func NewFromEnv(serviceName string) *Logger {
	cfg := &Config{
		Level:     getEnvOrDefault("LOG_LEVEL", "info"),
		Format:    getEnvOrDefault("LOG_FORMAT", "console"),
		Output:    getEnvOrDefault("LOG_OUTPUT", "stdout"),
		NoColor:   getEnvOrDefault("LOG_NO_COLOR", "false") == BooleanTrue,
		Timestamp: getEnvOrDefault("LOG_TIMESTAMP", "true") == BooleanTrue,
	}
	return New(cfg, serviceName)
}

// WithContext returns a logger enriched with trace/span/request IDs from context.
func (l *Logger) WithContext(ctx context.Context) *Logger {
	zc := l.logger.With()

	if v := ctx.Value(contextKey("trace_id")); v != nil {
		zc = zc.Str(FieldTraceID, fmt.Sprintf("%v", v))
	}
	if v := ctx.Value(contextKey("span_id")); v != nil {
		zc = zc.Str(FieldSpanID, fmt.Sprintf("%v", v))
	}
	if v := ctx.Value(contextKey("request_id")); v != nil {
		zc = zc.Str(FieldRequestID, fmt.Sprintf("%v", v))
	}
	if v := ctx.Value(contextKey("user_id")); v != nil {
		zc = zc.Str(FieldUserID, fmt.Sprintf("%v", v))
	}
	if v := ctx.Value(contextKey("correlation_id")); v != nil {
		zc = zc.Str(FieldCorrelationID, fmt.Sprintf("%v", v))
	}

	return &Logger{logger: zc.Logger(), service: l.service}
}

// contextKey is an unexported type for context keys to avoid collisions.
type contextKey string

// WithComponent returns a logger tagged with a component name.
func (l *Logger) WithComponent(name string) *Logger {
	return &Logger{
		logger:  l.logger.With().Str(FieldComponent, name).Logger(),
		service: l.service,
	}
}

// WithFields returns a logger with additional fields.
func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
	zc := l.logger.With()
	for k, v := range fields {
		zc = zc.Interface(k, v)
	}
	return &Logger{logger: zc.Logger(), service: l.service}
}

// WithError returns a logger with an error field.
func (l *Logger) WithError(err error) *Logger {
	return &Logger{
		logger:  l.logger.With().Err(err).Logger(),
		service: l.service,
	}
}

// GetLogger returns the underlying zerolog.Logger.
func (l *Logger) GetLogger() zerolog.Logger {
	return l.logger
}

// Debug logs a debug message.
func (l *Logger) Debug(msg string, fields ...map[string]interface{}) {
	event := l.logger.Debug()
	addFields(event, fields...)
	event.Msg(msg)
}

// Info logs an info message.
func (l *Logger) Info(msg string, fields ...map[string]interface{}) {
	event := l.logger.Info()
	addFields(event, fields...)
	event.Msg(msg)
}

// Warn logs a warning message.
func (l *Logger) Warn(msg string, fields ...map[string]interface{}) {
	event := l.logger.Warn()
	addFields(event, fields...)
	event.Msg(msg)
}

// Error logs an error message.
func (l *Logger) Error(msg string, fields ...map[string]interface{}) {
	event := l.logger.Error()
	addFields(event, fields...)
	event.Msg(msg)
}

// Fatal logs a fatal message and exits.
func (l *Logger) Fatal(msg string, fields ...map[string]interface{}) {
	event := l.logger.Fatal()
	addFields(event, fields...)
	event.Msg(msg)
}

// --- Global logger ---

var globalLogger *Logger

// SetGlobalLogger sets the global logger instance.
func SetGlobalLogger(l *Logger) { globalLogger = l }

// GetGlobalLogger returns the global logger, creating a default one if needed.
func GetGlobalLogger() *Logger {
	if globalLogger == nil {
		globalLogger = NewDefault("default")
	}
	return globalLogger
}

// GetLogger returns the underlying zerolog.Logger from the global logger (package-level).
func GetLoggerZ() zerolog.Logger {
	return GetGlobalLogger().GetLogger()
}

// Package-level convenience functions delegate to the global logger.

func Debug(msg string, fields ...map[string]interface{}) {
	GetGlobalLogger().Debug(msg, fields...)
}

func Info(msg string, fields ...map[string]interface{}) {
	GetGlobalLogger().Info(msg, fields...)
}

func Warn(msg string, fields ...map[string]interface{}) {
	GetGlobalLogger().Warn(msg, fields...)
}

func Error(msg string, fields ...map[string]interface{}) {
	GetGlobalLogger().Error(msg, fields...)
}

func Fatal(msg string, fields ...map[string]interface{}) {
	GetGlobalLogger().Fatal(msg, fields...)
}

// WithContext returns a context-enriched logger from the global logger.
func WithContext(ctx context.Context) *Logger {
	return GetGlobalLogger().WithContext(ctx)
}

// WithComponent returns a component-tagged logger from the global logger.
func WithComponent(name string) *Logger {
	return GetGlobalLogger().WithComponent(name)
}

// --- internal helpers ---

func addFields(event *zerolog.Event, fields ...map[string]interface{}) {
	for _, fm := range fields {
		for k, v := range fm {
			event.Interface(k, v)
		}
	}
}

func outputWriter(output string) *os.File {
	switch strings.ToLower(output) {
	case "stderr":
		return os.Stderr
	default:
		return os.Stdout
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

func newConsoleLogger(cfg *Config, serviceName string) zerolog.Logger {
	output := outputWriter(cfg.Output)
	return zerolog.New(zerolog.ConsoleWriter{
		Out:        output,
		TimeFormat: "15:04:05",
		NoColor:    cfg.NoColor,
		FormatLevel: func(i interface{}) string {
			lvl := strings.ToUpper(fmt.Sprintf("%s", i))
			if !cfg.NoColor {
				switch lvl {
				case "DEBUG":
					lvl = "\033[36m[DBG]\033[0m"
				case "INFO":
					lvl = "\033[32m[INF]\033[0m"
				case "WARN":
					lvl = "\033[33m[WRN]\033[0m"
				case "ERROR":
					lvl = "\033[31m[ERR]\033[0m"
				case "FATAL":
					lvl = "\033[35m[FTL]\033[0m"
				default:
					lvl = fmt.Sprintf("[%s]", lvl)
				}
			} else {
				switch lvl {
				case "DEBUG":
					lvl = "[DBG]"
				case "INFO":
					lvl = "[INF]"
				case "WARN":
					lvl = "[WRN]"
				case "ERROR":
					lvl = "[ERR]"
				case "FATAL":
					lvl = "[FTL]"
				default:
					lvl = fmt.Sprintf("[%s]", lvl)
				}
			}
			if serviceName != "" && serviceName != "default" && len(serviceName) >= 3 {
				tag := strings.ToUpper(serviceName[:3])
				if !cfg.NoColor {
					return fmt.Sprintf("\033[34m[%s]\033[0m%s", tag, lvl)
				}
				return fmt.Sprintf("[%s]%s", tag, lvl)
			}
			return lvl
		},
		FormatMessage: func(i interface{}) string {
			if i == nil {
				return ""
			}
			return fmt.Sprintf("%s", i)
		},
		FormatFieldName: func(i interface{}) string {
			return fmt.Sprintf("%s:", i)
		},
		FormatFieldValue: func(i interface{}) string {
			if i == nil {
				return ""
			}
			return fmt.Sprintf("%s", i)
		},
	}).With().Timestamp().Logger()
}
