package logging

import "context"

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

// ComponentSpan returns a component-tagged logger from the default logger.
func ComponentSpan(ctx context.Context, name string) *Logger {
	return Default().ComponentSpan(ctx, name)
}

// RequestSpan returns a request-enriched logger from the default logger.
func RequestSpan(ctx context.Context, method, path, requestID string) *Logger {
	return Default().RequestSpan(ctx, method, path, requestID)
}
