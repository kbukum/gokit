package provider

import (
	"context"
	"time"

	"github.com/kbukum/gokit/observability"
)

// WithMetrics returns a Middleware that records execution metrics
// using the gokit observability.Metrics instruments.
// Records: operation count, duration histogram, and errors.
func WithMetrics[I, O any](metrics *observability.Metrics) Middleware[I, O] {
	return func(inner RequestResponse[I, O]) RequestResponse[I, O] {
		return &metricsRR[I, O]{inner: inner, metrics: metrics}
	}
}

type metricsRR[I, O any] struct {
	inner   RequestResponse[I, O]
	metrics *observability.Metrics
}

func (m *metricsRR[I, O]) Name() string                         { return m.inner.Name() }
func (m *metricsRR[I, O]) IsAvailable(ctx context.Context) bool { return m.inner.IsAvailable(ctx) }

func (m *metricsRR[I, O]) Execute(ctx context.Context, input I) (O, error) {
	start := time.Now()
	output, err := m.inner.Execute(ctx, input)
	duration := time.Since(start)

	status := "ok"
	if err != nil {
		status = "error"
		m.metrics.RecordError(ctx, "execute", m.inner.Name())
	}
	m.metrics.RecordOperation(ctx, m.inner.Name(), "execute", status, duration)

	return output, err
}
