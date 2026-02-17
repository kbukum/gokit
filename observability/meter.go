package observability

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	"github.com/kbukum/gokit/logger"
)

// MeterConfig configures the OpenTelemetry meter provider.
type MeterConfig struct {
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
	// Interval is the metric export interval.
	Interval time.Duration
}

// DefaultMeterConfig returns sensible defaults for development.
func DefaultMeterConfig(serviceName string) MeterConfig {
	return MeterConfig{
		ServiceName:    serviceName,
		ServiceVersion: "1.0.0",
		Environment:    "development",
		Endpoint:       "localhost:4318",
		Insecure:       true,
		Interval:       15 * time.Second,
	}
}

// InitMeter initializes the OpenTelemetry meter provider.
// Returns a MeterProvider that should be shut down on application exit.
func InitMeter(ctx context.Context, config *MeterConfig) (*sdkmetric.MeterProvider, error) {
	opts := []otlpmetrichttp.Option{
		otlpmetrichttp.WithEndpoint(config.Endpoint),
	}
	if config.Insecure {
		opts = append(opts, otlpmetrichttp.WithInsecure())
	}

	exporter, err := otlpmetrichttp.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("creating metric exporter: %w", err)
	}

	res, err := newResource(config.ServiceName, config.ServiceVersion, config.Environment)
	if err != nil {
		return nil, fmt.Errorf("creating resource: %w", err)
	}

	readerOpts := []sdkmetric.PeriodicReaderOption{}
	if config.Interval > 0 {
		readerOpts = append(readerOpts, sdkmetric.WithInterval(config.Interval))
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter, readerOpts...)),
		sdkmetric.WithResource(res),
	)

	otel.SetMeterProvider(mp)

	logger.Info("meter initialized", logger.Fields(
		"service", config.ServiceName,
		"endpoint", config.Endpoint,
		"interval", config.Interval.String(),
	))

	return mp, nil
}

// Meter returns a named meter from the global provider.
func Meter(name string) metric.Meter {
	return otel.Meter(name)
}

// Metrics holds OpenTelemetry metric instruments for common service observability.
type Metrics struct {
	requestTotal      metric.Int64Counter
	requestDuration   metric.Float64Histogram
	requestActive     metric.Int64UpDownCounter
	operationTotal    metric.Int64Counter
	operationDuration metric.Float64Histogram
	errorTotal        metric.Int64Counter
}

// NewMetrics creates metric instruments on the given meter.
func NewMetrics(meter metric.Meter) (*Metrics, error) {
	requestTotal, err := meter.Int64Counter("request.total",
		metric.WithDescription("Total number of requests"),
	)
	if err != nil {
		return nil, fmt.Errorf("creating request.total counter: %w", err)
	}

	requestDuration, err := meter.Float64Histogram("request.duration",
		metric.WithDescription("Duration of requests in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, fmt.Errorf("creating request.duration histogram: %w", err)
	}

	requestActive, err := meter.Int64UpDownCounter("request.active",
		metric.WithDescription("Number of currently active requests"),
	)
	if err != nil {
		return nil, fmt.Errorf("creating request.active gauge: %w", err)
	}

	operationTotal, err := meter.Int64Counter("operation.total",
		metric.WithDescription("Total number of operations"),
	)
	if err != nil {
		return nil, fmt.Errorf("creating operation.total counter: %w", err)
	}

	operationDuration, err := meter.Float64Histogram("operation.duration",
		metric.WithDescription("Duration of operations in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, fmt.Errorf("creating operation.duration histogram: %w", err)
	}

	errorTotal, err := meter.Int64Counter("error.total",
		metric.WithDescription("Total errors by type and component"),
	)
	if err != nil {
		return nil, fmt.Errorf("creating error.total counter: %w", err)
	}

	return &Metrics{
		requestTotal:      requestTotal,
		requestDuration:   requestDuration,
		requestActive:     requestActive,
		operationTotal:    operationTotal,
		operationDuration: operationDuration,
		errorTotal:        errorTotal,
	}, nil
}

// RecordRequestStart increments the active request count.
func (m *Metrics) RecordRequestStart(ctx context.Context) {
	m.requestActive.Add(ctx, 1)
}

// RecordRequestEnd decrements active requests and records the completed request.
func (m *Metrics) RecordRequestEnd(ctx context.Context, service, method, status string, duration time.Duration) {
	attrs := metric.WithAttributes(
		attribute.String("service", service),
		attribute.String("method", method),
		attribute.String("status", status),
	)
	m.requestActive.Add(ctx, -1)
	m.requestTotal.Add(ctx, 1, attrs)
	m.requestDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(
		attribute.String("service", service),
		attribute.String("method", method),
	))
}

// RecordOperation records an operation execution.
func (m *Metrics) RecordOperation(ctx context.Context, service, operation, status string, duration time.Duration) {
	attrs := metric.WithAttributes(
		attribute.String("service", service),
		attribute.String("operation", operation),
		attribute.String("status", status),
	)
	m.operationTotal.Add(ctx, 1, attrs)
	m.operationDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(
		attribute.String("service", service),
		attribute.String("operation", operation),
	))
}

// RecordError records an error by type and component.
func (m *Metrics) RecordError(ctx context.Context, errType, component string) {
	m.errorTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String("type", errType),
		attribute.String("component", component),
	))
}
