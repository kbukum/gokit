package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/kbukum/gokit/observability"
)

// httpMetrics holds OTel metric instruments for HTTP request instrumentation.
type httpMetrics struct {
	requestsTotal metric.Int64Counter
	requestDur    metric.Float64Histogram
	requestSize   metric.Float64Histogram
	responseSize  metric.Float64Histogram
}

func newHTTPMetrics(serviceName string) (*httpMetrics, error) {
	meter := observability.Meter(serviceName)

	requestsTotal, err := meter.Int64Counter("http_requests_total",
		metric.WithDescription("Total number of HTTP requests"),
	)
	if err != nil {
		return nil, err
	}

	requestDur, err := meter.Float64Histogram("http_request_duration_seconds",
		metric.WithDescription("Duration of HTTP requests in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	requestSize, err := meter.Float64Histogram("http_request_size_bytes",
		metric.WithDescription("Size of HTTP request bodies in bytes"),
		metric.WithUnit("By"),
	)
	if err != nil {
		return nil, err
	}

	responseSize, err := meter.Float64Histogram("http_response_size_bytes",
		metric.WithDescription("Size of HTTP response bodies in bytes"),
		metric.WithUnit("By"),
	)
	if err != nil {
		return nil, err
	}

	return &httpMetrics{
		requestsTotal: requestsTotal,
		requestDur:    requestDur,
		requestSize:   requestSize,
		responseSize:  responseSize,
	}, nil
}

// PrometheusMetrics returns middleware that instruments HTTP requests with
// counters and histograms for request count, duration, request size, and
// response size. Labels: method, path, status_code.
func PrometheusMetrics(serviceName string) Middleware {
	metrics, err := newHTTPMetrics(serviceName)
	if err != nil {
		// If metric creation fails, return a pass-through middleware.
		return func(next http.Handler) http.Handler { return next }
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			mw := &metricsWriter{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(mw, r)

			duration := time.Since(start)
			attrs := metric.WithAttributes(
				attribute.String("method", r.Method),
				attribute.String("path", r.URL.Path),
				attribute.String("status_code", strconv.Itoa(mw.status)),
			)

			ctx := r.Context()
			metrics.requestsTotal.Add(ctx, 1, attrs)
			metrics.requestDur.Record(ctx, duration.Seconds(), attrs)
			if r.ContentLength >= 0 {
				metrics.requestSize.Record(ctx, float64(r.ContentLength), attrs)
			}
			metrics.responseSize.Record(ctx, float64(mw.written), attrs)
		})
	}
}

// RegisterMetricsEndpoint registers a /metrics endpoint on the given mux
// that exposes Prometheus metrics.
func RegisterMetricsEndpoint(mux *http.ServeMux) {
	mux.Handle("/metrics", promhttp.Handler())
}

// metricsWriter wraps http.ResponseWriter to capture status code and response size.
type metricsWriter struct {
	http.ResponseWriter
	status      int
	written     int64
	wroteHeader bool
}

func (mw *metricsWriter) WriteHeader(code int) {
	if !mw.wroteHeader {
		mw.status = code
		mw.wroteHeader = true
	}
	mw.ResponseWriter.WriteHeader(code)
}

func (mw *metricsWriter) Write(b []byte) (int, error) {
	if !mw.wroteHeader {
		mw.wroteHeader = true
	}
	n, err := mw.ResponseWriter.Write(b)
	mw.written += int64(n)
	return n, err
}

// Flush implements http.Flusher for streaming support.
func (mw *metricsWriter) Flush() {
	if f, ok := mw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap returns the underlying ResponseWriter for http.ResponseController.
func (mw *metricsWriter) Unwrap() http.ResponseWriter {
	return mw.ResponseWriter
}
