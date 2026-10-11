package observability

import (
	"context"
	"errors"
	"sync"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/kbukum/gokit/component"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/util"
)

// TraceComponent owns an instance's tracing exporter in the telemetry shutdown phase.
type TraceComponent struct {
	mu        sync.Mutex
	config    TracerConfig
	provider  *sdktrace.TracerProvider
	processor sdktrace.SpanProcessor
	exporter  *ownedTraceExporter
	factory   TraceExporterFactory
	log       *logging.Logger
	stopped   bool
	stopErr   error
}

// TraceExporterFactory creates an owned exporter during Start. It must honor cancellation and return no live resources with an error.
type TraceExporterFactory func(context.Context) (sdktrace.SpanExporter, error)

// TraceOption configures an instance-owned tracing component.
type TraceOption func(*traceOptions) error

type traceOptions struct{ factory TraceExporterFactory }

// WithTraceExporter selects an exporter factory instead of the configured OTLP adapter.
func WithTraceExporter(factory TraceExporterFactory) TraceOption {
	return func(options *traceOptions) error {
		if factory == nil {
			return apperrors.Validation("trace exporter factory is required")
		}
		options.factory = factory
		return nil
	}
}

// NewTraceComponent constructs tracing without starting exporter resources.
func NewTraceComponent(config TracerConfig, log *logging.Logger, opts ...TraceOption) (*TraceComponent, error) {
	if log == nil {
		return nil, apperrors.Validation("tracing requires an injected logger")
	}
	cfg, err := normalizedTracerConfig(&config)
	if err != nil {
		return nil, err
	}
	options := traceOptions{}
	options.factory = func(ctx context.Context) (sdktrace.SpanExporter, error) {
		return newTraceExporter(ctx, &cfg)
	}
	for _, opt := range opts {
		if opt == nil {
			return nil, apperrors.Validation("trace option must not be nil")
		}
		if optionErr := opt(&options); optionErr != nil {
			return nil, optionErr
		}
	}
	provider, err := newTracerProvider(cfg)
	if err != nil {
		return nil, err
	}
	return &TraceComponent{config: cfg, provider: provider, log: log, factory: options.factory}, nil
}

// Name identifies the tracing component within one process.
func (*TraceComponent) Name() string { return "tracing" }

// ShutdownPhase keeps tracing alive through resource teardown.
func (*TraceComponent) ShutdownPhase() component.ShutdownPhase { return component.PhaseTelemetry }

// Tracer returns an instance-owned tracer usable before Start; export begins at Start.
func (c *TraceComponent) Tracer(name string, opts ...trace.TracerOption) trace.Tracer {
	return c.provider.Tracer(name, opts...)
}

// Start initializes the exporter and bounded batch worker.
func (c *TraceComponent) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped {
		return apperrors.ServiceUnavailable("stopped tracing component")
	}
	if c.processor != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, c.config.ExportTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return apperrors.Internal(err)
	}
	exporter, err := c.factory(ctx)
	if err != nil {
		return traceError(err)
	}
	if util.IsNil(exporter) {
		return apperrors.Validation("trace exporter factory returned nil")
	}
	c.exporter = &ownedTraceExporter{
		exporter: &safeTraceExporter{SpanExporter: exporter},
		done:     make(chan struct{}), log: c.log,
	}
	c.processor = newTraceProcessor(c.exporter, c.config)
	c.provider.RegisterSpanProcessor(c.processor)
	return nil
}

// Stop flushes and joins the exporter within the caller's budget. A quarter of that budget is reserved for joining after canceling a stalled SDK drain.
func (c *TraceComponent) Stop(ctx context.Context) (err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped {
		return c.stopErr
	}
	defer func() { c.stopErr = err }()
	c.stopped = true
	ctx, cancel := context.WithTimeout(ctx, c.config.FlushTimeout)
	defer cancel()
	if c.processor == nil {
		return c.provider.Shutdown(ctx)
	}
	deadline, _ := ctx.Deadline()
	remaining := max(0, time.Until(deadline))
	flush, stopFlush := context.WithTimeout(ctx, remaining-remaining/4)
	defer stopFlush()
	cancelDone := make(chan struct{})
	cancelExport := context.AfterFunc(flush, func() {
		c.exporter.cancel()
		close(cancelDone)
	})
	defer func() {
		if !cancelExport() {
			<-cancelDone
		}
	}()
	defer c.exporter.cancel()
	err = c.provider.Shutdown(flush)
	// The SDK skips processors when passed an already-ended context.
	err = errors.Join(err, c.processor.Shutdown(flush))
	select {
	case <-c.exporter.done:
		return err
	case <-ctx.Done():
		return errors.Join(err, ctx.Err())
	}
}

// Health reports exporter failures without making business requests depend on telemetry.
func (c *TraceComponent) Health(context.Context) component.Health {
	c.mu.Lock()
	defer c.mu.Unlock()
	health := component.Health{Name: c.Name(), Status: component.StatusHealthy}
	if c.stopped || c.exporter == nil {
		health.Status, health.Message = component.StatusDegraded, "tracing is not running"
	} else if c.exporter.failed.Load() {
		health.Status, health.Message = component.StatusDegraded, "trace export failed"
	}
	return health
}
