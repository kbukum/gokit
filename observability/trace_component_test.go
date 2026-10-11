package observability

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/logging"
)

func TestTraceComponentConstruction(t *testing.T) {
	t.Parallel()
	log, err := logging.New(&logging.Config{Level: "error", Format: "json"}, "test", logging.WithWriter(io.Discard))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewTraceComponent(DefaultTracerConfig("test"), log); err != nil {
		t.Fatal(err)
	}
}

type ownedTestExporter struct {
	mu      sync.Mutex
	names   []string
	stops   atomic.Int32
	active  atomic.Int32
	block   bool
	stopErr error
}

func (e *ownedTestExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	e.active.Add(1)
	defer e.active.Add(-1)
	if e.block {
		<-ctx.Done()
		return errors.Join(ctx.Err(), errors.New("private-export-sentinel"))
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, span := range spans {
		e.names = append(e.names, span.Name())
	}
	return nil
}

func (e *ownedTestExporter) Shutdown(context.Context) error {
	e.stops.Add(1)
	return e.stopErr
}

func componentLogger(t *testing.T, out io.Writer) *logging.Logger {
	t.Helper()
	log, err := logging.New(&logging.Config{Level: "debug", Format: "json"}, "test", logging.WithWriter(out))
	if err != nil {
		t.Fatal(err)
	}
	return log
}

func TestTraceComponentOwnsExporter(t *testing.T) {
	t.Parallel()
	exporter := &ownedTestExporter{}
	created := 0
	c, err := NewTraceComponent(DefaultTracerConfig("test"), componentLogger(t, io.Discard),
		WithTraceExporter(func(context.Context) (sdktrace.SpanExporter, error) {
			created++
			return exporter, nil
		}))
	if err != nil {
		t.Fatal(err)
	}
	tracer := c.Tracer("test")
	if created != 0 || c.ShutdownPhase() != component.PhaseTelemetry || c.Health(t.Context()).Status != component.StatusDegraded {
		t.Fatal("construction acquired resources or advertised readiness")
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := c.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.Start(ctx); err != nil || created != 1 {
		t.Fatal("start created duplicate resources", err)
	}
	if c.Health(ctx).Status != component.StatusHealthy {
		t.Fatal("started component is not healthy")
	}
	_, span := tracer.Start(ctx, "before-shutdown")
	span.End()
	if err := c.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.Stop(ctx); err != nil || exporter.stops.Load() != 1 {
		t.Fatal("stop released exporter more than once", err)
	}
	if len(exporter.names) != 1 || exporter.names[0] != "before-shutdown" || exporter.active.Load() != 0 {
		t.Fatal("shutdown did not flush and join exporter")
	}
	if err := c.Start(ctx); err == nil {
		t.Fatal("stopped component restarted")
	}
}

func TestTraceComponentOutageFitsShutdownBudget(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		var output bytes.Buffer
		cfg := DefaultTracerConfig("test")
		cfg.ExportTimeout, cfg.FlushTimeout = 10*time.Millisecond, 30*time.Millisecond
		cfg.QueueSize, cfg.BatchSize = 16, 1
		exporter := &ownedTestExporter{block: true}
		c, err := NewTraceComponent(cfg, componentLogger(t, &output),
			WithTraceExporter(func(context.Context) (sdktrace.SpanExporter, error) { return exporter, nil }))
		if err != nil {
			t.Fatal(err)
		}
		if err := c.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		for range 10 {
			_, span := c.Tracer("test").Start(t.Context(), "test.Call")
			span.End()
		}
		synctest.Wait()
		start := time.Now()
		if err := c.Stop(t.Context()); err != nil && !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal("shutdown lost timeout cause", err)
		}
		if elapsed := time.Since(start); elapsed > cfg.FlushTimeout {
			t.Fatalf("shutdown took %s, budget %s", elapsed, cfg.FlushTimeout)
		}
		if exporter.active.Load() != 0 || exporter.stops.Load() != 1 {
			t.Fatal("bounded stop did not join the active exporter")
		}
		if !strings.Contains(output.String(), "TRACE_EXPORT_FAILED") || strings.Contains(output.String(), "private-") {
			t.Fatal("export failures were silent or leaked private diagnostics")
		}
	})
}

func TestTraceComponentFailureOwnership(t *testing.T) {
	t.Parallel()
	cause := errors.New("private-start-sentinel")
	c, err := NewTraceComponent(DefaultTracerConfig("test"), componentLogger(t, io.Discard),
		WithTraceExporter(func(context.Context) (sdktrace.SpanExporter, error) { return nil, cause }))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Start(t.Context()); !errors.Is(err, cause) || strings.Contains(err.Error(), "private-") {
		t.Fatal("startup lost cause or exposed private diagnostics", err)
	}
	if err := c.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	exporter := &ownedTestExporter{stopErr: cause}
	c, err = NewTraceComponent(DefaultTracerConfig("test"), componentLogger(t, io.Discard),
		WithTraceExporter(func(context.Context) (sdktrace.SpanExporter, error) { return exporter, nil }))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := c.Stop(t.Context()); !errors.Is(err, cause) || strings.Contains(err.Error(), "private-") {
			t.Fatal("shutdown lost cause or hid prior failure", err)
		}
	}
}

func TestTraceComponentRejectsInvalidConfig(t *testing.T) {
	t.Parallel()
	for _, mutate := range []func(*TracerConfig){
		func(c *TracerConfig) { c.ServiceName = "" },
		func(c *TracerConfig) { c.Endpoint = "https://private-sentinel@host:4318/path?secret=value" },
		func(c *TracerConfig) { c.Endpoint = "host:0" },
		func(c *TracerConfig) { c.SampleRate = math.NaN() },
		func(c *TracerConfig) { c.SampleRate = 2 },
		func(c *TracerConfig) { c.SampleRate = -1 },
		func(c *TracerConfig) { c.Protocol = OTLPProtocol(99) },
		func(c *TracerConfig) { c.ExportTimeout = -1 },
		func(c *TracerConfig) { c.FlushTimeout = -1 },
		func(c *TracerConfig) { c.QueueSize = -1 },
		func(c *TracerConfig) { c.QueueSize = math.MaxInt },
		func(c *TracerConfig) { c.QueueSize, c.BatchSize = 1, 2 },
	} {
		cfg := DefaultTracerConfig("test")
		mutate(&cfg)
		if _, err := NewTraceComponent(cfg, componentLogger(t, io.Discard)); err == nil || strings.Contains(err.Error(), "private-") {
			t.Fatal("invalid configuration was accepted or leaked", err)
		}
	}
	cfg := DefaultTracerConfig("test")
	if _, err := NewTraceComponent(cfg, nil); err == nil {
		t.Fatal("missing logger accepted")
	}
	for _, option := range []TraceOption{nil, WithTraceExporter(nil)} {
		if _, err := NewTraceComponent(cfg, componentLogger(t, io.Discard), option); err == nil {
			t.Fatal("invalid option accepted")
		}
	}
}
