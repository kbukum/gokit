package observability

import (
	"context"
	"testing"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func tracerContext(t *testing.T, tp *sdktrace.TracerProvider) context.Context {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), time.Second)
		defer cancel()
		if err := tp.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	ctx, err := WithTracerProvider(t.Context(), tp)
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}
