package middleware

import (
	"fmt"
	"net/http"

	"github.com/kbukum/gokit/observability"
)

// Tracing returns middleware that creates spans for HTTP requests.
// It extracts W3C TraceContext from incoming headers, creates a server span
// with HTTP method/path/status attributes, and injects trace context into
// the response headers for downstream propagation.
func Tracing(serviceName string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract W3C TraceContext from incoming request headers.
			ctx := observability.ExtractTraceContext(r.Context(), observability.HeaderCarrier(r.Header))

			spanName := fmt.Sprintf("%s %s", r.Method, r.URL.Path)
			ctx, span := observability.StartNamedSpan(ctx, serviceName, spanName,
				observability.WithSpanKind(observability.SpanKindServer),
				observability.WithSpanAttributes(
					observability.StringAttribute("http.method", r.Method),
					observability.StringAttribute("http.target", r.URL.Path),
					observability.StringAttribute("http.scheme", httpScheme(r)),
				),
			)
			defer span.End()

			// Inject trace context into response headers before the handler writes.
			observability.InjectTraceContext(ctx, observability.HeaderCarrier(w.Header()))

			sw := newStatusWriter(w)
			next.ServeHTTP(sw, r.WithContext(ctx))

			span.SetAttributes(observability.IntAttribute("http.status_code", sw.status))

			if sw.status >= 500 {
				span.SetError(http.StatusText(sw.status))
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
