package observability

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// setTracerProvider installs tp as the global tracer provider for the duration
// of the test, restoring the previous provider and shutting tp down on cleanup.
// Cleanups run LIFO, so the previous provider is restored before tp is shut down,
// preventing parallel tests in this package from observing a shut-down provider.
func setTracerProvider(t *testing.T, tp *sdktrace.TracerProvider) {
	t.Helper()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prev) })
}

// setTextMapPropagator installs p as the global propagator for the duration of
// the test and restores the previous one on cleanup.
func setTextMapPropagator(t *testing.T, p propagation.TextMapPropagator) {
	t.Helper()
	prev := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(p)
	t.Cleanup(func() { otel.SetTextMapPropagator(prev) })
}
