package observability

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/logging"
)

type httpTraceExporter struct {
	sdktrace.SpanExporter
	client *http.Client
}

func (e *httpTraceExporter) Shutdown(ctx context.Context) error {
	defer e.client.CloseIdleConnections()
	return e.SpanExporter.Shutdown(ctx)
}

type ownedTraceExporter struct {
	exporter     sdktrace.SpanExporter
	mu           sync.Mutex
	stopping     bool
	cancelExport context.CancelFunc
	done         chan struct{}
	log          *logging.Logger
	failed       atomic.Bool
}

type traceFailure struct{ cause *apperrors.AppError }

func (traceFailure) Error() string   { return "tracing provider failed" }
func (e traceFailure) Unwrap() error { return e.cause }

func traceError(cause error) error {
	return traceFailure{cause: apperrors.Internal(cause)}
}

type safeTraceExporter struct {
	sdktrace.SpanExporter
}

func (e *safeTraceExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	if err := e.SpanExporter.ExportSpans(ctx, spans); err != nil {
		return traceError(err)
	}
	return nil
}

func (e *safeTraceExporter) Shutdown(ctx context.Context) error {
	if err := e.SpanExporter.Shutdown(ctx); err != nil {
		return traceError(err)
	}
	return nil
}

func (e *ownedTraceExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	e.mu.Lock()
	if e.stopping {
		e.mu.Unlock()
		return traceError(context.Canceled)
	}
	// The SDK batch processor serializes exports, including its final drain.
	e.cancelExport = cancel
	e.mu.Unlock()
	defer func() {
		e.mu.Lock()
		e.cancelExport = nil
		e.mu.Unlock()
	}()
	err := e.exporter.ExportSpans(ctx, spans)
	e.failed.Store(err != nil)
	if err != nil {
		e.log.WarnCtx(ctx, "Trace export failed", map[string]any{"code": "TRACE_EXPORT_FAILED"})
		// The SDK error handler must not expose exporter URLs or response text.
		return err
	}
	return nil
}

func (e *ownedTraceExporter) Shutdown(ctx context.Context) error {
	defer close(e.done)
	if err := e.exporter.Shutdown(ctx); err != nil {
		e.failed.Store(true)
		e.log.WarnCtx(ctx, "Trace exporter shutdown failed", map[string]any{"code": "TRACE_SHUTDOWN_FAILED"})
		return err
	}
	return nil
}

func (e *ownedTraceExporter) cancel() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.stopping = true
	if e.cancelExport != nil {
		e.cancelExport()
	}
}
