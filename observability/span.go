package observability

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// SpanKind identifies the role a span plays in a distributed trace.
type SpanKind int

const (
	// SpanKindInternal is for work internal to a process.
	SpanKindInternal SpanKind = iota
	// SpanKindServer is for inbound request handling.
	SpanKindServer
	// SpanKindClient is for outbound requests.
	SpanKindClient
	// SpanKindProducer is for message production.
	SpanKindProducer
	// SpanKindConsumer is for message consumption.
	SpanKindConsumer
)

// SpanAttribute is a typed, transport-neutral span attribute.
type SpanAttribute struct {
	key string
	val attrValue
}

// StringAttribute creates a string span attribute.
func StringAttribute(key, value string) SpanAttribute {
	return SpanAttribute{key: key, val: stringValue(value)}
}

// IntAttribute creates an int span attribute.
func IntAttribute(key string, value int) SpanAttribute {
	return SpanAttribute{key: key, val: intValue(value)}
}

// Int64Attribute creates an int64 span attribute.
func Int64Attribute(key string, value int64) SpanAttribute {
	return SpanAttribute{key: key, val: int64Value(value)}
}

// Float64Attribute creates a float64 span attribute.
func Float64Attribute(key string, value float64) SpanAttribute {
	return SpanAttribute{key: key, val: float64Value(value)}
}

// BoolAttribute creates a bool span attribute.
func BoolAttribute(key string, value bool) SpanAttribute {
	return SpanAttribute{key: key, val: boolValue(value)}
}

// StringSliceAttribute creates a string-slice span attribute.
func StringSliceAttribute(key string, value []string) SpanAttribute {
	return SpanAttribute{key: key, val: stringSliceValue(value)}
}

// SpanOption configures a span without exposing OpenTelemetry option types.
type SpanOption func(*spanOptions)

type spanOptions struct {
	kind       SpanKind
	attributes []SpanAttribute
}

// WithSpanKind sets the span kind.
func WithSpanKind(kind SpanKind) SpanOption {
	return func(opts *spanOptions) {
		opts.kind = kind
	}
}

// WithSpanAttributes adds attributes to the span.
func WithSpanAttributes(attrs ...SpanAttribute) SpanOption {
	return func(opts *spanOptions) {
		opts.attributes = append(opts.attributes, attrs...)
	}
}

// Span wraps an OpenTelemetry span behind the kit observability API.
type Span struct {
	span trace.Span
}

// StartNamedSpan starts a span from a named tracer.
func StartNamedSpan(ctx context.Context, tracerName, spanName string, opts ...SpanOption) (context.Context, *Span) {
	cfg := spanOptions{kind: SpanKindInternal}
	for _, opt := range opts {
		opt(&cfg)
	}

	startOpts := []trace.SpanStartOption{trace.WithSpanKind(toOTelSpanKind(cfg.kind))}
	if len(cfg.attributes) > 0 {
		startOpts = append(startOpts, trace.WithAttributes(toOTelSpanAttributes(cfg.attributes)...))
	}

	ctx, span := Tracer(tracerName).Start(ctx, spanName, startOpts...)
	return ctx, &Span{span: span}
}

func toOTelSpanKind(kind SpanKind) trace.SpanKind {
	switch kind {
	case SpanKindServer:
		return trace.SpanKindServer
	case SpanKindClient:
		return trace.SpanKindClient
	case SpanKindProducer:
		return trace.SpanKindProducer
	case SpanKindConsumer:
		return trace.SpanKindConsumer
	default:
		return trace.SpanKindInternal
	}
}

func toOTelSpanAttributes(attrs []SpanAttribute) []attribute.KeyValue {
	out := make([]attribute.KeyValue, 0, len(attrs))
	for _, attr := range attrs {
		out = append(out, attr.val.keyValue(attr.key))
	}
	return out
}

// End completes the span.
func (s *Span) End() {
	if s != nil && s.span != nil {
		s.span.End()
	}
}

// SetAttributes sets attributes on the span.
func (s *Span) SetAttributes(attrs ...SpanAttribute) {
	if s != nil && s.span != nil {
		s.span.SetAttributes(toOTelSpanAttributes(attrs)...)
	}
}

// RecordError records err on the span.
func (s *Span) RecordError(err error) {
	if s != nil && s.span != nil && err != nil {
		s.span.RecordError(err)
	}
}

// SetError marks the span status as error.
func (s *Span) SetError(message string) {
	if s != nil && s.span != nil {
		s.span.SetStatus(codes.Error, message)
	}
}
