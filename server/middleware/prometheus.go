package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/kbukum/gokit/observability"
)

// httpMetrics holds OTel metric instruments for HTTP request instrumentation.
type httpMetrics struct {
	requestsTotal *observability.Int64Counter
	requestDur    *observability.Float64Histogram
	requestSize   *observability.Float64Histogram
	responseSize  *observability.Float64Histogram
}

func newHTTPMetrics(serviceName string) (*httpMetrics, error) {
	requestsTotal, err := observability.NewInt64Counter(serviceName, "http_requests_total",
		observability.WithInstrumentDescription("Total number of HTTP requests"),
	)
	if err != nil {
		return nil, err
	}

	requestDur, err := observability.NewFloat64Histogram(serviceName, "http_request_duration_seconds",
		observability.WithInstrumentDescription("Duration of HTTP requests in seconds"),
		observability.WithInstrumentUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	requestSize, err := observability.NewFloat64Histogram(serviceName, "http_request_size_bytes",
		observability.WithInstrumentDescription("Size of HTTP request bodies in bytes"),
		observability.WithInstrumentUnit("By"),
	)
	if err != nil {
		return nil, err
	}

	responseSize, err := observability.NewFloat64Histogram(serviceName, "http_response_size_bytes",
		observability.WithInstrumentDescription("Size of HTTP response bodies in bytes"),
		observability.WithInstrumentUnit("By"),
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
			attrs := []observability.MetricAttribute{
				observability.MetricStringAttribute("method", r.Method),
				observability.MetricStringAttribute("path", r.URL.Path),
				observability.MetricStringAttribute("status_code", strconv.Itoa(mw.status)),
			}

			ctx := r.Context()
			metrics.requestsTotal.Add(ctx, 1, attrs...)
			metrics.requestDur.Record(ctx, duration.Seconds(), attrs...)
			if r.ContentLength >= 0 {
				metrics.requestSize.Record(ctx, float64(r.ContentLength), attrs...)
			}
			metrics.responseSize.Record(ctx, float64(mw.written), attrs...)
		})
	}
}

// RegisterMetricsEndpoint registers a /metrics endpoint on the given mux
// that exposes Prometheus metrics.
func RegisterMetricsEndpoint(mux *http.ServeMux) {
	observability.RegisterPrometheusEndpoint(mux, "/metrics")
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
