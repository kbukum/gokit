package logging

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/rs/zerolog"
)

const (
	FormatPretty = "pretty"
	BooleanTrue  = "true"
)

// Logger wraps zerolog.Logger with additional context.
type Logger struct {
	logger       zerolog.Logger
	service      string
	masker       Masker
	moduleLevels *ModuleLevelManager
	otlpProvider *OTLPProvider
}

// New creates a new logger instance with configuration.
func New(cfg *Config, serviceName string) *Logger {
	level, err := zerolog.ParseLevel(cfg.Level)
	if err != nil {
		level = zerolog.InfoLevel
	}

	output := outputWriter(cfg.Output)

	var zl zerolog.Logger
	if strings.EqualFold(cfg.Format, "console") || strings.EqualFold(cfg.Format, FormatPretty) {
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
	if cfg.Stacktrace {
		zl = zl.With().Stack().Logger()
	}

	zl = zl.Level(level)

	// Apply sampling when enabled.
	if cfg.Sampling.Enabled {
		zl = zl.Sample(NewSampler(cfg.Sampling))
	}

	l := &Logger{
		logger:  zl,
		service: serviceName,
	}

	if cfg.Masking.Enabled {
		l.masker = NewDefaultMasker(cfg.Masking)
	}

	// Create module-level manager when overrides are configured.
	if len(cfg.ModuleLevels) > 0 {
		l.moduleLevels = NewModuleLevelManager(cfg.ModuleLevels)
	}

	// Initialize OTLP log bridge when enabled.
	if cfg.OTLP.Enabled {
		provider, err := NewOTLPProvider(cfg.OTLP, serviceName, cfg.Environment, cfg.Version)
		if err != nil {
			l.logger.Warn().Err(err).Msg("failed to initialize OTLP log provider")
		} else {
			l.otlpProvider = provider
		}
	}

	return l
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

	return &Logger{logger: zc.Logger(), service: l.service, masker: l.masker, moduleLevels: l.moduleLevels, otlpProvider: l.otlpProvider}
}

// contextKey is an unexported type for context keys to avoid collisions.
type contextKey string

// WithComponent returns a logger tagged with a component name.
// If a per-module log level override exists for the component, it is applied.
func (l *Logger) WithComponent(name string) *Logger {
	zl := l.logger.With().Str(FieldComponent, name).Logger()

	// Apply per-module level override when available.
	if l.moduleLevels != nil {
		if lvl, ok := l.moduleLevels.Level(name); ok {
			zl = zl.Level(lvl)
		}
	}

	return &Logger{
		logger:       zl,
		service:      l.service,
		masker:       l.masker,
		moduleLevels: l.moduleLevels,
		otlpProvider: l.otlpProvider,
	}
}

// WithFields returns a logger with additional fields.
func (l *Logger) WithFields(fields map[string]any) *Logger {
	zc := l.logger.With()
	for k, v := range fields {
		zc = zc.Interface(k, v)
	}
	return &Logger{logger: zc.Logger(), service: l.service, masker: l.masker, moduleLevels: l.moduleLevels, otlpProvider: l.otlpProvider}
}

// WithError returns a logger with an error field.
func (l *Logger) WithError(err error) *Logger {
	return &Logger{
		logger:       l.logger.With().Err(err).Logger(),
		service:      l.service,
		masker:       l.masker,
		moduleLevels: l.moduleLevels,
		otlpProvider: l.otlpProvider,
	}
}

// WithMasker returns a new Logger with the given Masker applied.
func (l *Logger) WithMasker(m Masker) *Logger {
	return &Logger{
		logger:       l.logger,
		service:      l.service,
		masker:       m,
		moduleLevels: l.moduleLevels,
		otlpProvider: l.otlpProvider,
	}
}

// WithOTLP returns a new Logger with the given OTLPProvider attached.
func (l *Logger) WithOTLP(provider *OTLPProvider) *Logger {
	return &Logger{
		logger:       l.logger,
		service:      l.service,
		masker:       l.masker,
		moduleLevels: l.moduleLevels,
		otlpProvider: provider,
	}
}

// Close gracefully shuts down the OTLP provider, flushing pending logs.
func (l *Logger) Close() error {
	if l.otlpProvider != nil {
		return l.otlpProvider.Shutdown(context.Background())
	}
	return nil
}

// GetLogger returns the underlying zerolog.Logger.
func (l *Logger) GetLogger() zerolog.Logger {
	return l.logger
}

// Debug logs a debug message.
//
// For request- or operation-scoped logging that should propagate cancellation
// and trace correlation to OTLP, prefer DebugCtx.
func (l *Logger) Debug(msg string, fields ...map[string]any) {
	l.DebugCtx(context.Background(), msg, fields...) //nolint:contextcheck // background ctx is intentional for the no-context API; callers with a ctx in scope should use DebugCtx
}

// DebugCtx logs a debug message and propagates ctx to the OTLP exporter.
func (l *Logger) DebugCtx(ctx context.Context, msg string, fields ...map[string]any) {
	event := l.logger.Debug()
	l.addFields(event, fields...)
	event.Msg(msg)
	l.emitOTLP(ctx, "debug", msg, fields...)
}

// Info logs an info message. Prefer InfoCtx when a context is in scope.
func (l *Logger) Info(msg string, fields ...map[string]any) {
	l.InfoCtx(context.Background(), msg, fields...) //nolint:contextcheck // background ctx is intentional for the no-context API
}

// InfoCtx logs an info message and propagates ctx to the OTLP exporter.
func (l *Logger) InfoCtx(ctx context.Context, msg string, fields ...map[string]any) {
	event := l.logger.Info()
	l.addFields(event, fields...)
	event.Msg(msg)
	l.emitOTLP(ctx, "info", msg, fields...)
}

// Warn logs a warning message. Prefer WarnCtx when a context is in scope.
func (l *Logger) Warn(msg string, fields ...map[string]any) {
	l.WarnCtx(context.Background(), msg, fields...) //nolint:contextcheck // background ctx is intentional for the no-context API
}

// WarnCtx logs a warning message and propagates ctx to the OTLP exporter.
func (l *Logger) WarnCtx(ctx context.Context, msg string, fields ...map[string]any) {
	event := l.logger.Warn()
	l.addFields(event, fields...)
	event.Msg(msg)
	l.emitOTLP(ctx, "warn", msg, fields...)
}

// Error logs an error message. Prefer ErrorCtx when a context is in scope.
func (l *Logger) Error(msg string, fields ...map[string]any) {
	l.ErrorCtx(context.Background(), msg, fields...) //nolint:contextcheck // background ctx is intentional for the no-context API
}

// ErrorCtx logs an error message and propagates ctx to the OTLP exporter.
func (l *Logger) ErrorCtx(ctx context.Context, msg string, fields ...map[string]any) {
	event := l.logger.Error()
	l.addFields(event, fields...)
	event.Msg(msg)
	l.emitOTLP(ctx, "error", msg, fields...)
}

// Fatal logs a fatal message and exits. Prefer FatalCtx when a context is in scope.
func (l *Logger) Fatal(msg string, fields ...map[string]any) {
	l.FatalCtx(context.Background(), msg, fields...) //nolint:contextcheck // background ctx is intentional for the no-context API
}

// FatalCtx logs a fatal message and exits, propagating ctx to the OTLP exporter.
func (l *Logger) FatalCtx(ctx context.Context, msg string, fields ...map[string]any) {
	l.emitOTLP(ctx, "fatal", msg, fields...)
	event := l.logger.Fatal()
	l.addFields(event, fields...)
	event.Msg(msg)
}

// --- Process default logger ---

// defaultLogger lazily constructs the process default logger exactly once. It is immutable:
// there is no setter, so package-level convenience helpers and registries derive from a stable,
// race-free instance instead of a mutable global that can be reassigned at runtime.
var defaultLogger = sync.OnceValue(func() *Logger {
	return NewDefault("default")
})

// Default returns the process default logger, constructing it on first use.
// Prefer injecting an explicit *Logger;
// use Default only for incidental logging where threading a logger through the call site adds no value.
func Default() *Logger {
	return defaultLogger()
}

// GetLoggerZ returns the underlying zerolog.Logger from the default logger.
func GetLoggerZ() zerolog.Logger {
	return Default().GetLogger()
}

// Package-level convenience functions delegate to the default logger.

func Debug(msg string, fields ...map[string]any) {
	Default().Debug(msg, fields...) //nolint:contextcheck // package-level helper for callers without a context in scope
}

func DebugCtx(ctx context.Context, msg string, fields ...map[string]any) {
	Default().DebugCtx(ctx, msg, fields...)
}

func Info(msg string, fields ...map[string]any) {
	Default().Info(msg, fields...) //nolint:contextcheck // package-level helper for callers without a context in scope
}

func InfoCtx(ctx context.Context, msg string, fields ...map[string]any) {
	Default().InfoCtx(ctx, msg, fields...)
}

func Warn(msg string, fields ...map[string]any) {
	Default().Warn(msg, fields...) //nolint:contextcheck // package-level helper for callers without a context in scope
}

func WarnCtx(ctx context.Context, msg string, fields ...map[string]any) {
	Default().WarnCtx(ctx, msg, fields...)
}

func Error(msg string, fields ...map[string]any) {
	Default().Error(msg, fields...) //nolint:contextcheck // package-level helper for callers without a context in scope
}

func ErrorCtx(ctx context.Context, msg string, fields ...map[string]any) {
	Default().ErrorCtx(ctx, msg, fields...)
}

func Fatal(msg string, fields ...map[string]any) {
	Default().Fatal(msg, fields...) //nolint:contextcheck // package-level helper for callers without a context in scope
}

func FatalCtx(ctx context.Context, msg string, fields ...map[string]any) {
	Default().FatalCtx(ctx, msg, fields...)
}

// WithContext returns a context-enriched logger from the default logger.
func WithContext(ctx context.Context) *Logger {
	return Default().WithContext(ctx)
}

// WithComponent returns a component-tagged logger from the default logger.
func WithComponent(name string) *Logger {
	return Default().WithComponent(name)
}

// --- internal helpers ---

func (l *Logger) emitOTLP(ctx context.Context, level, msg string, fields ...map[string]any) {
	if l.otlpProvider == nil {
		return
	}
	merged := make(map[string]any)
	for _, fm := range fields {
		for k, v := range fm {
			merged[k] = v
		}
	}
	l.otlpProvider.EmitLog(ctx, level, msg, merged)
}

func (l *Logger) addFields(event *zerolog.Event, fields ...map[string]any) {
	if l.masker != nil {
		for _, fm := range fields {
			for k, v := range fm {
				str, isStr := v.(string)
				if !isStr {
					str = fmt.Sprintf("%v", v)
				}
				masked := l.masker.MaskValue(k, str)
				if masked != str {
					event.Str(k, masked)
				} else {
					event.Interface(k, v)
				}
			}
		}
		return
	}
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
		FormatLevel: func(i any) string {
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
		FormatMessage: func(i any) string {
			if i == nil {
				return ""
			}
			return fmt.Sprintf("%s", i)
		},
		FormatFieldName: func(i any) string {
			return fmt.Sprintf("%s:", i)
		},
		FormatFieldValue: func(i any) string {
			if i == nil {
				return ""
			}
			return fmt.Sprintf("%s", i)
		},
	}).With().Timestamp().Logger()
}
