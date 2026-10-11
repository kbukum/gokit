package observability

import (
	"context"

	"go.opentelemetry.io/otel/trace"

	"github.com/kbukum/gokit/logging"
)

// TraceID returns the current span's trace ID, or an empty string without a valid span.
func TraceID(ctx context.Context) string {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return ""
	}
	return sc.TraceID().String()
}

// Correlate replaces logging identifiers with the active span's IDs, not request headers.
func Correlate(ctx context.Context) context.Context {
	id := TraceID(ctx)
	spanID := ""
	if id != "" {
		spanID = trace.SpanContextFromContext(ctx).SpanID().String()
	}
	ctx = logging.ContextWithTraceID(ctx, id)
	ctx = logging.ContextWithSpanID(ctx, spanID)
	ctx = logging.ContextWithRequestID(ctx, id)
	return logging.ContextWithCorrelationID(ctx, id)
}
