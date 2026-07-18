package observability

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// OperationContext holds observability context for a tracked operation.
type OperationContext struct {
	ServiceName   string
	OperationName string
	RequestID     string
	UserID        string
	StartTime     time.Time
	Metrics       *Metrics
}

// OperationSpec identifies a tracked operation for [NewOperationContext].
// Metrics may be nil to silently skip metric recording.
type OperationSpec struct {
	ServiceName   string
	OperationName string
	RequestID     string
	UserID        string
	Metrics       *Metrics
}

// NewOperationContext creates a new operation context from spec. If spec.Metrics is nil,
// metric recording is silently skipped.
func NewOperationContext(spec OperationSpec) *OperationContext {
	return &OperationContext{
		ServiceName:   spec.ServiceName,
		OperationName: spec.OperationName,
		RequestID:     spec.RequestID,
		UserID:        spec.UserID,
		StartTime:     time.Now(),
		Metrics:       spec.Metrics,
	}
}

// operationContextKey is the context key for OperationContext.
type operationContextKey struct{}

// WithOperationContext stores an OperationContext in the context.
func WithOperationContext(ctx context.Context, oc *OperationContext) context.Context {
	return context.WithValue(ctx, operationContextKey{}, oc)
}

// OperationContextFromContext retrieves the OperationContext from context, or nil.
func OperationContextFromContext(ctx context.Context) *OperationContext {
	if oc, ok := ctx.Value(operationContextKey{}).(*OperationContext); ok {
		return oc
	}
	return nil
}

// StartSpanForOperation starts a traced span and records the request start metric.
func (oc *OperationContext) StartSpanForOperation(ctx context.Context, spanName string) (context.Context, trace.Span) {
	ctx, span := StartSpan(ctx, spanName)
	span.SetAttributes(
		attribute.String(AttrServiceName, oc.ServiceName),
		attribute.String(AttrOperationName, oc.OperationName),
		attribute.String(AttrRequestID, oc.RequestID),
	)
	if oc.UserID != "" {
		span.SetAttributes(attribute.String(AttrUserID, oc.UserID))
	}

	if oc.Metrics != nil {
		oc.Metrics.RecordRequestStart(ctx)
	}
	return ctx, span
}

// EndOperation ends the span and records request-end metrics.
func (oc *OperationContext) EndOperation(ctx context.Context, span trace.Span, status string, err error) {
	duration := time.Since(oc.StartTime)

	if err != nil {
		span.RecordError(err)
		span.SetAttributes(attribute.String(AttrErrorMessage, err.Error()))
	}

	span.SetAttributes(
		attribute.String(AttrStatus, status),
		attribute.Int64(AttrDurationMs, duration.Milliseconds()),
	)
	span.End()

	if oc.Metrics != nil {
		oc.Metrics.RecordRequestEnd(ctx, RequestMetric{
			Service:  oc.ServiceName,
			Method:   oc.OperationName,
			Status:   status,
			Duration: duration,
		})
	}
}

// Duration returns the elapsed time since operation start.
func (oc *OperationContext) Duration() time.Duration {
	return time.Since(oc.StartTime)
}
