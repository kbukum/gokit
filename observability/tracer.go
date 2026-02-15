package observability

import (
	"context"
	"fmt"

	"github.com/skillsenselab/gokit/logger"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
)

const defaultTracerName = "github.com/skillsenselab/gokit/observability"

// TracerConfig configures the OpenTelemetry tracer.
type TracerConfig struct {
	// ServiceName is the name of the service.
	ServiceName string
	// ServiceVersion is the version of the service.
	ServiceVersion string
	// Environment is the deployment environment (dev, staging, prod).
	Environment string
	// Endpoint is the OTLP HTTP endpoint host:port (e.g., "localhost:4318").
	Endpoint string
	// Insecure allows insecure connections (for development).
	Insecure bool
	// SampleRate is the sampling rate (0.0 to 1.0).
	SampleRate float64
}

// DefaultTracerConfig returns sensible defaults for development.
func DefaultTracerConfig(serviceName string) TracerConfig {
	return TracerConfig{
		ServiceName:    serviceName,
		ServiceVersion: "1.0.0",
		Environment:    "development",
		Endpoint:       "localhost:4318",
		Insecure:       true,
		SampleRate:     1.0,
	}
}

// InitTracer initializes the OpenTelemetry tracer provider.
// Returns a TracerProvider that should be shut down on application exit.
func InitTracer(ctx context.Context, config TracerConfig) (*sdktrace.TracerProvider, error) {
	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(config.Endpoint),
	}
	if config.Insecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}

	exporter, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("creating trace exporter: %w", err)
	}

	res, err := newResource(config.ServiceName, config.ServiceVersion, config.Environment)
	if err != nil {
		return nil, fmt.Errorf("creating resource: %w", err)
	}

	var sampler sdktrace.Sampler
	switch {
	case config.SampleRate >= 1.0:
		sampler = sdktrace.AlwaysSample()
	case config.SampleRate <= 0:
		sampler = sdktrace.NeverSample()
	default:
		sampler = sdktrace.TraceIDRatioBased(config.SampleRate)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	logger.Info("tracer initialized", logger.Fields(
		"service", config.ServiceName,
		"endpoint", config.Endpoint,
		"sample_rate", config.SampleRate,
	))

	return tp, nil
}

// newResource creates an OpenTelemetry resource with service metadata.
func newResource(serviceName, serviceVersion, environment string) (*resource.Resource, error) {
	return resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(serviceVersion),
			attribute.String("environment", environment),
		),
	)
}

// Tracer returns a named tracer from the global provider.
func Tracer(name string) trace.Tracer {
	return otel.Tracer(name)
}

// StartSpan starts a new span using the default tracer.
func StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return Tracer(defaultTracerName).Start(ctx, name, opts...)
}

// SpanFromContext returns the span from context.
func SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// SetSpanAttribute sets an attribute on the current span in context.
func SetSpanAttribute(ctx context.Context, key string, value any) {
	span := SpanFromContext(ctx)
	if span == nil || !span.IsRecording() {
		return
	}

	switch v := value.(type) {
	case string:
		span.SetAttributes(attribute.String(key, v))
	case int:
		span.SetAttributes(attribute.Int(key, v))
	case int64:
		span.SetAttributes(attribute.Int64(key, v))
	case float64:
		span.SetAttributes(attribute.Float64(key, v))
	case bool:
		span.SetAttributes(attribute.Bool(key, v))
	case []string:
		span.SetAttributes(attribute.StringSlice(key, v))
	}
}

// SetSpanError records an error on the current span in context.
func SetSpanError(ctx context.Context, err error) {
	span := SpanFromContext(ctx)
	if span != nil && span.IsRecording() {
		span.RecordError(err)
	}
}

// Common span names.
const (
	SpanHTTPRequest = "http.request"
	SpanGRPCCall    = "grpc.call"
	SpanDBQuery     = "db.query"
)

// Common attribute keys.
const (
	AttrServiceName   = "service.name"
	AttrOperationName = "operation.name"
	AttrRequestID     = "request.id"
	AttrUserID        = "user.id"
	AttrDurationMs    = "duration_ms"
	AttrStatus        = "status"
	AttrErrorMessage  = "error.message"
)
