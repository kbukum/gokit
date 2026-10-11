package connect

import (
	"context"
	"errors"
	"net/http"

	"connectrpc.com/connect"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/observability"
	"github.com/kbukum/gokit/util"
)

// TracingConfig supplies instance-owned RPC tracing.
type TracingConfig struct {
	Tracer     trace.Tracer
	Propagator propagation.TextMapPropagator
	// TrustParent accepts incoming trace context only after peer authentication. Nil ignores incoming headers while preserving an existing local HTTP span.
	TrustParent func(context.Context) bool
}

// TracingInterceptor creates procedure-named client and server spans without recording payloads, headers, baggage or error text. Place before normalization and logging. Streaming clients must receive to completion or close their response.
func TracingInterceptor(cfg TracingConfig) (connect.Interceptor, error) {
	if util.IsNil(cfg.Tracer) {
		return nil, apperrors.Validation("RPC tracing requires a tracer")
	}
	if util.IsNil(cfg.Propagator) {
		cfg.Propagator = propagation.TraceContext{}
	}
	return &tracingInterceptor{cfg: cfg}, nil
}

type tracingInterceptor struct{ cfg TracingConfig }

func (i *tracingInterceptor) start(ctx context.Context, spec connect.Spec, header http.Header) (context.Context, trace.Span) {
	ctx = baggage.ContextWithBaggage(ctx, baggage.Baggage{})
	kind := trace.SpanKindServer
	var options []trace.SpanStartOption
	switch {
	case spec.IsClient:
		kind = trace.SpanKindClient
	case i.cfg.TrustParent != nil && i.cfg.TrustParent(ctx):
		ctx = i.cfg.Propagator.Extract(ctx, propagation.HeaderCarrier(header))
	case trace.SpanContextFromContext(ctx).IsRemote():
		options = append(options, trace.WithNewRoot())
	}
	ctx = baggage.ContextWithBaggage(ctx, baggage.Baggage{})
	options = append(options, trace.WithSpanKind(kind))
	ctx, span := i.cfg.Tracer.Start(ctx, spec.Procedure, options...)
	ctx = observability.Correlate(ctx)
	if spec.IsClient {
		// Clear incoming baggage and stale propagation before writing our span.
		for _, key := range i.cfg.Propagator.Fields() {
			header.Del(key)
		}
		header.Del("Baggage")
		i.cfg.Propagator.Inject(ctx, propagation.HeaderCarrier(header))
	}
	return ctx, span
}

func (i *tracingInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		ctx, span := i.start(ctx, req.Spec(), req.Header())
		defer span.End()
		resp, err := next(ctx, req)
		setRPCSpanStatus(span, err)
		if !req.Spec().IsClient && resp != nil {
			i.cfg.Propagator.Inject(ctx, propagation.HeaderCarrier(resp.Header()))
		}
		return resp, err
	}
}

func (i *tracingInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		ctx, span := i.start(ctx, conn.Spec(), conn.RequestHeader())
		defer span.End()
		i.cfg.Propagator.Inject(ctx, propagation.HeaderCarrier(conn.ResponseHeader()))
		err := next(ctx, conn)
		setRPCSpanStatus(span, err)
		return err
	}
}

func setRPCSpanStatus(span trace.Span, err error) {
	code := "ok"
	if err != nil {
		var native *connect.Error
		if errors.As(err, &native) {
			code = native.Code().String()
		} else {
			code = connect.Code(apperrors.RPCCodeFor(apperrors.Normalize(err).Code)).String()
		}
		span.SetStatus(codes.Error, code)
	}
	span.SetAttributes(attribute.String("rpc.response.status_code", code))
}
