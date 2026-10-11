package observability

import (
	"context"
	"net"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/util"
)

const defaultTracerName = "github.com/kbukum/gokit/observability"

// TracerConfig configures the OpenTelemetry tracer.
type TracerConfig struct {
	// ServiceName is the name of the service.
	ServiceName string
	// ServiceVersion is the version of the service.
	ServiceVersion string
	// Environment is the deployment environment (dev, staging, prod).
	Environment string
	// Endpoint is the OTLP endpoint host:port (e.g., "localhost:4318" for HTTP, "localhost:4317" for gRPC).
	Endpoint string
	// Protocol selects the OTLP wire protocol (HTTP or gRPC). Defaults to HTTP.
	Protocol OTLPProtocol
	// Insecure allows insecure connections (for development).
	Insecure bool
	// SampleRate is the sampling rate (0.0 to 1.0) for new root traces; spans with an accepted parent follow the parent's sampling decision.
	SampleRate float64
	// Zero limits use bounded defaults; negative limits are rejected.
	ExportTimeout time.Duration
	FlushTimeout  time.Duration
	BatchTimeout  time.Duration
	QueueSize     int
	BatchSize     int
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

// InitTracer creates an instance-owned OpenTelemetry tracer provider without registering globals.
// The caller owns shutdown and injects provider.Tracer(...) into instrumentation.
func InitTracer(ctx context.Context, config *TracerConfig) (*sdktrace.TracerProvider, error) {
	cfg, err := normalizedTracerConfig(config)
	if err != nil {
		return nil, err
	}
	tp, err := newTracerProvider(cfg)
	if err != nil {
		return nil, err
	}
	exporter, err := newTraceExporter(ctx, &cfg)
	if err != nil {
		return nil, traceError(err)
	}
	tp.RegisterSpanProcessor(newTraceProcessor(&safeTraceExporter{SpanExporter: exporter}, cfg))
	return tp, nil
}

func newTracerProvider(config TracerConfig) (*sdktrace.TracerProvider, error) {
	res, err := newResource(config.ServiceName, config.ServiceVersion, config.Environment)
	if err != nil {
		return nil, traceError(err)
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

	// Transports drop untrusted parents, so an accepted parent's sampling decision is authoritative; the rate applies to new roots.
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sampler)),
		sdktrace.WithoutPanicRecording(),
	)

	return tp, nil
}

func newTraceProcessor(exporter sdktrace.SpanExporter, cfg TracerConfig) sdktrace.SpanProcessor {
	return sdktrace.NewBatchSpanProcessor(exporter,
		sdktrace.WithMaxQueueSize(cfg.QueueSize),
		sdktrace.WithMaxExportBatchSize(cfg.BatchSize),
		sdktrace.WithBatchTimeout(cfg.BatchTimeout),
		sdktrace.WithExportTimeout(cfg.ExportTimeout),
	)
}

// newTraceExporter builds the OTLP trace exporter for the configured protocol.
func newTraceExporter(ctx context.Context, config *TracerConfig) (sdktrace.SpanExporter, error) {
	if err := config.Protocol.Validate(); err != nil {
		return nil, err
	}
	if config.Protocol == OTLPGRPC {
		opts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(config.Endpoint)}
		if config.ExportTimeout > 0 {
			opts = append(opts, otlptracegrpc.WithTimeout(config.ExportTimeout))
		}
		if config.Insecure {
			opts = append(opts, otlptracegrpc.WithInsecure())
		}
		return otlptracegrpc.New(ctx, opts...)
	}
	opts := []otlptracehttp.Option{otlptracehttp.WithEndpoint(config.Endpoint)}
	timeout := config.ExportTimeout
	if timeout == 0 {
		timeout = 2 * time.Second
	}
	// OTLP must not instrument itself; this instance owns the SDK's HTTP pool.
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          2,
		MaxIdleConnsPerHost:   2,
		MaxConnsPerHost:       2,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   timeout,
		ResponseHeaderTimeout: timeout,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	opts = append(opts, otlptracehttp.WithHTTPClient(client))
	if config.Insecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}
	exporter, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		client.CloseIdleConnections()
		return nil, err
	}
	return &httpTraceExporter{SpanExporter: exporter, client: client}, nil
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

type tracerProviderKey struct{}

// TracerProvider supplies instance-owned named tracers. Both TraceComponent and OpenTelemetry providers implement this contract.
type TracerProvider interface {
	Tracer(string, ...trace.TracerOption) trace.Tracer
}

// WithTracerProvider injects the provider for kit instrumentation, including operations without a local parent span. The provider remains caller-owned; nil providers are rejected.
func WithTracerProvider(ctx context.Context, provider TracerProvider) (context.Context, error) {
	if util.IsNil(provider) {
		return nil, apperrors.Validation("tracing requires a provider")
	}
	return context.WithValue(ctx, tracerProviderKey{}, provider), nil
}

// Tracer returns a named tracer from the context's injected provider, or the active span's provider. Without either, tracing is a no-op; process globals are never consulted.
func Tracer(ctx context.Context, name string) trace.Tracer {
	if provider, ok := ctx.Value(tracerProviderKey{}).(TracerProvider); ok {
		return provider.Tracer(name)
	}
	return trace.SpanFromContext(ctx).TracerProvider().Tracer(name)
}

// StartSpan starts a new span using the default tracer.
func StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return Tracer(ctx, defaultTracerName).Start(ctx, name, opts...)
}

// SpanFromContext returns the span from context.
func SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// SetSpanAttributes sets typed attributes on the current span in context.
func SetSpanAttributes(ctx context.Context, attrs ...SpanAttribute) {
	span := SpanFromContext(ctx)
	if span == nil || !span.IsRecording() {
		return
	}
	span.SetAttributes(toOTelSpanAttributes(attrs)...)
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
