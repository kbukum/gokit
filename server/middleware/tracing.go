package middleware

import (
	"context"
	"net/http"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/observability"
	"github.com/kbukum/gokit/util"
)

// TracingConfig supplies instance-owned tracing and trusted request classification.
type TracingConfig struct {
	Tracer     trace.Tracer
	Propagator propagation.TextMapPropagator
	// Route returns a registered route template or procedure, never raw input. A nil classifier or empty result uses the fixed "http.request" name.
	Route func(*http.Request) string
	// TrustParent may accept a parent only after peer authentication. Nil means public ingress: start a fresh trace regardless of incoming sampling.
	TrustParent func(*http.Request) bool
}

type traceRouteKey struct{}

// Tracing creates server spans without recording URLs, headers, bodies or baggage. Install inside authentication for trusted peers and outside RequestLogger. Response bodies and status codes remain unchanged; HTTP 5xx responses mark the span as failed.
func Tracing(cfg TracingConfig) (Middleware, error) {
	if util.IsNil(cfg.Tracer) {
		return nil, apperrors.Validation("HTTP tracing requires a tracer")
	}
	if util.IsNil(cfg.Propagator) {
		cfg.Propagator = propagation.TraceContext{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := baggage.ContextWithBaggage(r.Context(), baggage.Baggage{})
			options := []trace.SpanStartOption{trace.WithSpanKind(trace.SpanKindServer)}
			if cfg.TrustParent != nil && cfg.TrustParent(r) {
				ctx = cfg.Propagator.Extract(ctx, propagation.HeaderCarrier(r.Header))
			} else {
				options = append(options, trace.WithNewRoot())
			}
			// Even an injected composite propagator cannot carry incoming baggage.
			ctx = baggage.ContextWithBaggage(ctx, baggage.Baggage{})
			route := ""
			if cfg.Route != nil {
				route = cfg.Route(r)
			}
			name := route
			if name == "" {
				name = observability.SpanHTTPRequest
			}
			options = append(options, trace.WithAttributes(
				attribute.String("http.request.method", traceMethod(r.Method)),
				attribute.String("url.scheme", httpScheme(r)),
			))
			if route != "" {
				options = append(options, trace.WithAttributes(attribute.String("http.route", route)))
			}
			ctx, span := cfg.Tracer.Start(ctx, name, options...)
			defer span.End()
			ctx = observability.Correlate(ctx)
			ctx = context.WithValue(ctx, traceRouteKey{}, route)
			cfg.Propagator.Inject(ctx, propagation.HeaderCarrier(w.Header()))
			if id := observability.TraceID(ctx); id != "" {
				w.Header().Set("X-Request-Id", id)
			}
			sw := newStatusWriter(w)
			next.ServeHTTP(sw, r.WithContext(ctx))
			span.SetAttributes(attribute.Int("http.response.status_code", sw.status))
			if sw.status >= http.StatusInternalServerError {
				span.SetStatus(codes.Error, http.StatusText(sw.status))
			}
		})
	}, nil
}

func httpScheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

func traceMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodDelete,
		http.MethodConnect, http.MethodOptions, http.MethodTrace, http.MethodPatch:
		return method
	default:
		return "_OTHER"
	}
}
