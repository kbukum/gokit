package logging

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"
)

// otlpShutdownTimeout bounds how long Close waits for the OTLP exporter to
// flush and shut down, so an unreachable collector cannot stall process
// shutdown indefinitely.
const otlpShutdownTimeout = 5 * time.Second

// Logger is the injected logging handle for gokit consumers. It is a thin
// ergonomic facade over the standard library's [log/slog]: the value-add
// behavior (masking, sampling, per-module levels, context enrichment, OTLP
// export) lives in a composable [slog.Handler] chain, and the underlying
// *slog.Logger is always reachable via [Logger.Slog] for stdlib-native use or
// interop. There is no process-global logger on the consumer path; construct
// one and inject it.
type Logger struct {
	slog    *slog.Logger
	service string
	level   *slog.LevelVar
	// otlp is non-nil only on the root logger returned by a constructor, so
	// Close shuts the exporter down exactly once.
	otlp *OTLPProvider
}

// New builds a Logger from cfg for the named service. Options customize the
// sink and middleware — see [WithHandler], [WithBaseSink], [WithMasker].
// It returns an error only when an enabled OTLP exporter fails to initialize.
func New(cfg *Config, serviceName string, opts ...Option) (*Logger, error) {
	var o options
	for _, fn := range opts {
		fn(&o)
	}
	p, err := buildPipeline(cfg, serviceName, o)
	if err != nil {
		return nil, err
	}
	return &Logger{
		slog:    slog.New(p.handler),
		service: serviceName,
		level:   p.level,
		otlp:    p.otlp,
	}, nil
}

// MustNew is like [New] but panics if construction fails. It is the sanctioned
// Must-twin for configurations known good at author time (no OTLP, or a config
// validated elsewhere) and for tests, mirroring the standard library's
// regexp.MustCompile. Do not use it on runtime or user-supplied configuration
// paths — call [New] and handle the error there.
func MustNew(cfg *Config, serviceName string, opts ...Option) *Logger {
	l, err := New(cfg, serviceName, opts...)
	if err != nil {
		panic(fmt.Sprintf("logging: MustNew: %v", err))
	}
	return l
}

// NewDefault builds a Logger with a sane console configuration. It never
// enables OTLP, so it cannot fail and returns a single value for ergonomic
// nil-fallback wiring inside the kit. Defaults are applied so the secure
// baseline — including value masking — is in effect.
func NewDefault(serviceName string) *Logger {
	cfg := &Config{Level: "info", Format: "console", Output: "stdout", Timestamp: true}
	cfg.ApplyDefaults()
	l, _ := New(cfg, serviceName) //nolint:errcheck // OTLP disabled, so New cannot error here
	return l
}

// NewFromEnv builds a Logger configured from LOG_* environment variables.
func NewFromEnv(serviceName string) (*Logger, error) {
	cfg := &Config{
		Level:     getEnvOrDefault("LOG_LEVEL", "info"),
		Format:    getEnvOrDefault("LOG_FORMAT", "console"),
		Output:    getEnvOrDefault("LOG_OUTPUT", "stdout"),
		NoColor:   getEnvOrDefault("LOG_NO_COLOR", "false") == BooleanTrue,
		Timestamp: getEnvOrDefault("LOG_TIMESTAMP", "true") == BooleanTrue,
	}
	cfg.ApplyDefaults()
	return New(cfg, serviceName)
}

// Slog returns the underlying *slog.Logger — the idiomatic escape hatch for
// stdlib-native logging and for passing gokit's governed handler chain to code
// that speaks slog directly.
func (l *Logger) Slog() *slog.Logger { return l.slog }

// Handler returns the root slog.Handler backing the logger.
func (l *Logger) Handler() slog.Handler { return l.slog.Handler() }

// SetLevel updates the base level at runtime. Unrecognized values are ignored.
func (l *Logger) SetLevel(level string) {
	if parsed, ok := ParseLevel(level); ok && l.level != nil {
		l.level.Set(parsed)
	}
}

// Level reports the current base level.
func (l *Logger) Level() slog.Level {
	if l.level == nil {
		return slog.LevelInfo
	}
	return l.level.Level()
}

// Close shuts down the OTLP exporter, flushing pending logs. It is a no-op on
// derived loggers and when OTLP is disabled. The flush is bounded by
// [otlpShutdownTimeout] so an unavailable collector cannot stall shutdown.
func (l *Logger) Close() error {
	if l.otlp == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), otlpShutdownTimeout)
	defer cancel()
	return l.otlp.Shutdown(ctx)
}

// derive returns a Logger backed by a new *slog.Logger, sharing the level var
// but not the OTLP ownership (Close stays bound to the constructed root).
func (l *Logger) derive(s *slog.Logger) *Logger {
	return &Logger{slog: s, service: l.service, level: l.level}
}

// WithComponent returns a logger tagged with a component name. Any per-module
// level override configured for that component then applies.
func (l *Logger) WithComponent(name string) *Logger {
	return l.derive(l.slog.With(slog.String(FieldComponent, name)))
}

// WithFields returns a logger carrying the given fields on every record.
func (l *Logger) WithFields(fields map[string]any) *Logger {
	return l.derive(l.slog.With(fieldsToArgs(fields)...))
}

// WithError returns a logger carrying an error field.
func (l *Logger) WithError(err error) *Logger {
	if err == nil {
		return l
	}
	return l.derive(l.slog.With(slog.String(FieldError, err.Error())))
}

// WithContext returns a logger enriched with correlation identifiers already
// present on ctx. Prefer the *Ctx logging methods, which enrich automatically.
func (l *Logger) WithContext(ctx context.Context) *Logger {
	var args []any
	for _, key := range contextIDs {
		if v, ok := ctx.Value(contextKey(key)).(string); ok && v != "" {
			args = append(args, slog.String(key, v))
		}
	}
	if len(args) == 0 {
		return l
	}
	return l.derive(l.slog.With(args...))
}

// Debug logs at debug level. Prefer DebugCtx when a context is in scope.
func (l *Logger) Debug(msg string, fields ...map[string]any) {
	l.slog.Debug(msg, fieldsToArgs(fields...)...)
}

// DebugCtx logs at debug level, propagating ctx for correlation and export.
func (l *Logger) DebugCtx(ctx context.Context, msg string, fields ...map[string]any) {
	l.slog.DebugContext(ctx, msg, fieldsToArgs(fields...)...)
}

// Info logs at info level. Prefer InfoCtx when a context is in scope.
func (l *Logger) Info(msg string, fields ...map[string]any) {
	l.slog.Info(msg, fieldsToArgs(fields...)...)
}

// InfoCtx logs at info level, propagating ctx for correlation and export.
func (l *Logger) InfoCtx(ctx context.Context, msg string, fields ...map[string]any) {
	l.slog.InfoContext(ctx, msg, fieldsToArgs(fields...)...)
}

// Warn logs at warn level. Prefer WarnCtx when a context is in scope.
func (l *Logger) Warn(msg string, fields ...map[string]any) {
	l.slog.Warn(msg, fieldsToArgs(fields...)...)
}

// WarnCtx logs at warn level, propagating ctx for correlation and export.
func (l *Logger) WarnCtx(ctx context.Context, msg string, fields ...map[string]any) {
	l.slog.WarnContext(ctx, msg, fieldsToArgs(fields...)...)
}

// Error logs at error level. Prefer ErrorCtx when a context is in scope.
func (l *Logger) Error(msg string, fields ...map[string]any) {
	l.slog.Error(msg, fieldsToArgs(fields...)...)
}

// ErrorCtx logs at error level, propagating ctx for correlation and export.
func (l *Logger) ErrorCtx(ctx context.Context, msg string, fields ...map[string]any) {
	l.slog.ErrorContext(ctx, msg, fieldsToArgs(fields...)...)
}

// Trace logs at trace level (more verbose than debug).
func (l *Logger) Trace(msg string, fields ...map[string]any) {
	l.slog.Log(context.Background(), LevelTrace, msg, fieldsToArgs(fields...)...)
}

// TraceCtx logs at trace level, propagating ctx for correlation and export.
func (l *Logger) TraceCtx(ctx context.Context, msg string, fields ...map[string]any) {
	l.slog.Log(ctx, LevelTrace, msg, fieldsToArgs(fields...)...)
}

// Fatal logs at the fatal level. It does NOT terminate the process: whether to
// exit — and running deferred cleanup beforehand — is the application entry
// point's decision, not the library's. Callers that must abort should return
// or propagate an error and let main choose the exit policy.
func (l *Logger) Fatal(msg string, fields ...map[string]any) {
	l.FatalCtx(context.Background(), msg, fields...)
}

// FatalCtx logs at the fatal level, propagating ctx for correlation and export.
// Like [Logger.Fatal], it does not terminate the process.
func (l *Logger) FatalCtx(ctx context.Context, msg string, fields ...map[string]any) {
	l.slog.Log(ctx, LevelFatal, msg, fieldsToArgs(fields...)...)
}

// fieldsToArgs flattens field maps into slog attribute arguments.
func fieldsToArgs(fields ...map[string]any) []any {
	var args []any
	for _, fm := range fields {
		for k, v := range fm {
			args = append(args, slog.Any(k, v))
		}
	}
	return args
}

func getEnvOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
