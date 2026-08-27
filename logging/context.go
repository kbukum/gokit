package logging

import "context"

// contextKey is an unexported context key type, avoiding collisions with keys
// defined in other packages.
type contextKey string

// loggerCtxKey is a distinct unexported key type for carrying a *Logger on a
// context, kept separate from the string-based identifier keys.
type loggerCtxKey struct{}

// ContextWithLogger returns a context carrying log, so request-scoped helpers
// and free functions can retrieve an injected logger with [LoggerFromContext]
// instead of reaching for a package global. A nil logger is not stored.
func ContextWithLogger(ctx context.Context, log *Logger) context.Context {
	if log == nil {
		return ctx
	}
	return context.WithValue(ctx, loggerCtxKey{}, log)
}

// LoggerFromContext returns the logger carried by ctx and whether one was
// present. It never returns a nil logger together with ok=true.
func LoggerFromContext(ctx context.Context) (*Logger, bool) {
	log, ok := ctx.Value(loggerCtxKey{}).(*Logger)
	return log, ok && log != nil
}

// Span-level context helpers — the gokit equivalent of rskit-logging's
// context module (component tagging, request enrichment, identifier injection).
//
// rskit records identifiers onto the current tracing span, which nested spans
// inherit automatically. Go contexts are immutable and gokit's Logger reads
// identifiers back out of the context (see (*Logger).WithContext), so each
// ContextWith* helper returns a derived context carrying the identifier and the
// *Span helpers fold those identifiers into structured log fields.

// ContextWithTraceID returns a context carrying the trace ID, which
// (*Logger).WithContext folds into the trace_id field.
func ContextWithTraceID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, contextKey(FieldTraceID), id)
}

// ContextWithSpanID returns a context carrying the span ID, which
// (*Logger).WithContext folds into the span_id field.
func ContextWithSpanID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, contextKey(FieldSpanID), id)
}

// ContextWithRequestID returns a context carrying the request ID, which
// (*Logger).WithContext folds into the request_id field.
func ContextWithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, contextKey(FieldRequestID), id)
}

// ContextWithUserID returns a context carrying the user ID, which
// (*Logger).WithContext folds into the user_id field.
func ContextWithUserID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, contextKey(FieldUserID), id)
}

// ContextWithCorrelationID returns a context carrying the correlation ID, which
// (*Logger).WithContext folds into the correlation_id field.
func ContextWithCorrelationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, contextKey(FieldCorrelationID), id)
}

// ComponentSpan returns a logger tagged with the component name and enriched
// with any identifiers already present in ctx — the gokit equivalent of
// rskit-logging's component_span.
func (l *Logger) ComponentSpan(ctx context.Context, name string) *Logger {
	return l.WithContext(ctx).WithComponent(name)
}

// RequestSpan returns a logger enriched with HTTP request metadata and any
// identifiers already present in ctx — the gokit equivalent of rskit-logging's
// request_span.
func (l *Logger) RequestSpan(ctx context.Context, method, path, requestID string) *Logger {
	return l.WithContext(ctx).WithFields(map[string]any{
		FieldHTTPMethod: method,
		FieldHTTPPath:   path,
		FieldRequestID:  requestID,
	})
}
