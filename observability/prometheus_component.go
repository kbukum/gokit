package observability

import (
	"context"
	"errors"
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	"github.com/kbukum/gokit/component"
	apperrors "github.com/kbukum/gokit/errors"
)

// PrometheusComponent owns a pull meter provider and its private scrape registry.
type PrometheusComponent struct {
	mu       sync.Mutex
	provider *sdkmetric.MeterProvider
	exporter *sdkmetric.MeterProvider
	collect  *prometheusCollection
	handler  http.Handler
	started  bool
	stopped  bool
	closed   bool
	stopErr  error
}

// NewPrometheusComponent constructs an isolated application and runtime exporter without global registration.
func NewPrometheusComponent() (*PrometheusComponent, error) {
	registry, err := runtimeRegistry()
	if err != nil {
		return nil, err
	}
	manual := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(manual), sdkmetric.WithCardinalityLimit(512))
	collect := newPrometheusCollection(manual)
	translated := prometheus.NewRegistry()
	reader, err := otelprom.New(otelprom.WithRegisterer(translated), otelprom.WithProducer(collect))
	if err != nil {
		return nil, apperrors.Internal(errors.Join(err, provider.Shutdown(context.Background())))
	}
	collect.gatherers = prometheus.Gatherers{registry, translated}
	exporter := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	handler := promhttp.HandlerFor(collect, promhttp.HandlerOpts{
		MaxRequestsInFlight: 2, Timeout: collectionTimeout,
	})
	return &PrometheusComponent{provider: provider, exporter: exporter, collect: collect, handler: handler}, nil
}

// Name identifies the instance's exporter.
func (*PrometheusComponent) Name() string { return "metrics" }

// ShutdownPhase preserves metrics until application consumers have stopped.
func (*PrometheusComponent) ShutdownPhase() component.ShutdownPhase { return component.PhaseTelemetry }

// Meter returns an instance-owned meter; instrument names and attributes must use bounded vocabularies.
func (c *PrometheusComponent) Meter(name string, opts ...metric.MeterOption) metric.Meter {
	return c.provider.Meter(name, opts...)
}

// Handler exposes this instance's metrics on a private administration listener.
func (c *PrometheusComponent) Handler() http.Handler { return c.handler }

// Start makes the exporter available until its terminal Stop.
func (c *PrometheusComponent) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return apperrors.Internal(err)
	}
	if c.stopped {
		return apperrors.ServiceUnavailable("stopped metrics component")
	}
	c.started = true
	return nil
}

// Stop cancels and drains collection before releasing readers. Incomplete teardown is retryable.
func (c *PrometheusComponent) Stop(ctx context.Context) error {
	c.mu.Lock()
	c.stopped = true
	c.mu.Unlock()
	if err := c.collect.stop(ctx); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		c.stopErr = errors.Join(c.provider.Shutdown(ctx), c.exporter.Shutdown(ctx))
		c.closed = true
	}
	return c.stopErr
}

// Health reports lifecycle state without making product requests depend on telemetry.
func (c *PrometheusComponent) Health(context.Context) component.Health {
	c.mu.Lock()
	defer c.mu.Unlock()
	health := component.Health{Name: c.Name(), Status: component.StatusHealthy}
	if !c.started || c.stopped {
		health.Status, health.Message = component.StatusDegraded, "metrics is not running"
	}
	return health
}
