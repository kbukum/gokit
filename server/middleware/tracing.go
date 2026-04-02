package middleware

import (
	"fmt"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/kbukum/gokit/observability"
)

// Tracing returns middleware that creates spans for HTTP requests.
// It extracts W3C TraceContext from incoming headers, creates a server span
// with HTTP method/path/status attributes, and injects trace context into
// the response headers for downstream propagation.
func Tracing(serviceName string) Middleware {
	tracer := observability.Tracer(serviceName)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract W3C TraceContext from incoming request headers.
			ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))

			spanName := fmt.Sprintf("%s %s", r.Method, r.URL.Path)
			ctx, span := tracer.Start(ctx, spanName, trace.WithSpanKind(trace.SpanKindServer))
			defer span.End()

			span.SetAttributes(
				attribute.String("http.method", r.Method),
				attribute.String("http.target", r.URL.Path),
				attribute.String("http.scheme", httpScheme(r)),
			)

			// Inject trace context into response headers before the handler writes.
			otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(w.Header()))

			sw := newStatusWriter(w)
			next.ServeHTTP(sw, r.WithContext(ctx))

			span.SetAttributes(attribute.Int("http.status_code", sw.status))

			if sw.status >= 500 {
				span.SetStatus(codes.Error, http.StatusText(sw.status))
			}
		})
	}
}

// httpScheme returns "https" or "http" based on the request.
func httpScheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		return proto
	}
	return "http"
}
