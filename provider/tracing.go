package provider

import (
	"context"

	"github.com/kbukum/gokit/observability"
)

// WithTracing returns a Middleware that creates an OpenTelemetry span
// around each Execute call using the gokit observability package.
// The span name is "{serviceName}.{providerName}".
func WithTracing[I, O any](serviceName string) Middleware[I, O] {
	return func(inner RequestResponse[I, O]) RequestResponse[I, O] {
		return &tracingRR[I, O]{inner: inner, serviceName: serviceName}
	}
}

type tracingRR[I, O any] struct {
	inner       RequestResponse[I, O]
	serviceName string
}

func (t *tracingRR[I, O]) Name() string                         { return t.inner.Name() }
func (t *tracingRR[I, O]) IsAvailable(ctx context.Context) bool { return t.inner.IsAvailable(ctx) }

func (t *tracingRR[I, O]) Execute(ctx context.Context, input I) (O, error) {
	spanName := t.serviceName + "." + t.inner.Name()
	ctx, span := observability.StartSpan(ctx, spanName)
	defer span.End()

	observability.SetSpanAttribute(ctx, observability.AttrServiceName, t.serviceName)
	observability.SetSpanAttribute(ctx, observability.AttrOperationName, t.inner.Name())

	output, err := t.inner.Execute(ctx, input)
	if err != nil {
		observability.SetSpanError(ctx, err)
	}

	return output, err
}
