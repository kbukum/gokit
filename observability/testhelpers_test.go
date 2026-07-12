package observability

import (
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// setTracerProvider installs tp as the global tracer provider for the duration
// of the test and restores the previous provider on cleanup. Because `go test`
// runs packages in parallel, leaving global OTel state mutated can make other
// packages' tests flaky.
func setTracerProvider(t *testing.T, tp oteltrace.TracerProvider) {
	t.Helper()
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
