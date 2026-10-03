package middleware

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"time"

	"go.opentelemetry.io/otel/metric"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/observability"
	"github.com/kbukum/gokit/util"
)

// httpMetrics holds OTel metric instruments for HTTP request instrumentation.
type httpMetrics struct {
	requestsTotal *observability.Int64Counter
	requestDur    *observability.Float64Histogram
	requestSize   *observability.Float64Histogram
	responseSize  *observability.Float64Histogram
	active        metric.Int64UpDownCounter
}

func newHTTPMetrics(serviceName string, meter metric.Meter) (*httpMetrics, error) {
	requestsTotal, err := observability.NewInt64Counter(serviceName, "http_requests_total",
		observability.WithInstrumentDescription("Total number of HTTP requests"),
		observability.WithMeter(meter),
	)
	if err != nil {
		return nil, err
	}

	requestDur, err := observability.NewFloat64Histogram(serviceName, "http_request_duration_seconds",
		observability.WithInstrumentDescription("Duration of HTTP requests in seconds"),
		observability.WithInstrumentUnit("s"),
		observability.WithMeter(meter),
	)
	if err != nil {
		return nil, err
	}

	requestSize, err := observability.NewFloat64Histogram(serviceName, "http_request_size_bytes",
		observability.WithInstrumentDescription("Size of HTTP request bodies in bytes"),
		observability.WithInstrumentUnit("By"),
		observability.WithMeter(meter),
	)
	if err != nil {
		return nil, err
	}

	responseSize, err := observability.NewFloat64Histogram(serviceName, "http_response_size_bytes",
		observability.WithInstrumentDescription("Size of HTTP response bodies in bytes"),
		observability.WithInstrumentUnit("By"),
		observability.WithMeter(meter),
	)
	if err != nil {
		return nil, err
	}

	active, err := meter.Int64UpDownCounter("http_requests_active")
	if err != nil {
		return nil, err
	}
	return &httpMetrics{
		requestsTotal: requestsTotal,
		requestDur:    requestDur,
		requestSize:   requestSize,
		responseSize:  responseSize,
		active:        active,
	}, nil
}

// PrometheusMetrics returns middleware that instruments HTTP requests with counters
// and histograms for request count, duration, request size, and response size. Labels: method,
// path, status_code. Route patterns come from SetMetricRoute, never URL paths. Unknown routes and methods use fixed labels. The injected meter owns aggregation and export.
func PrometheusMetrics(serviceName string, meter metric.Meter) (Middleware, error) {
	if util.IsNil(meter) {
		return nil, apperrors.InvalidInput("meter", "HTTP metrics require an injected meter")
	}
	metrics, err := newHTTPMetrics(serviceName, meter)
	if err != nil {
		return nil, err
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			route := &metricRoute{pattern: "unmatched"}
			r = r.WithContext(context.WithValue(r.Context(), metricRouteKey{}, route))
			start := time.Now()
			mw := &metricsWriter{ResponseWriter: w, status: http.StatusOK}
			metrics.active.Add(r.Context(), 1)
			defer func(ctx context.Context) {
				metrics.active.Add(ctx, -1)
				attrs := []observability.MetricAttribute{
					observability.MetricStringAttribute("method", metricMethod(r.Method)),
					observability.MetricStringAttribute("path", route.name()),
					observability.MetricStringAttribute("status_code", strconv.Itoa(mw.status)),
				}

				metrics.requestsTotal.Add(ctx, 1, attrs...)
				metrics.requestDur.Record(ctx, time.Since(start).Seconds(), attrs...)
				if r.ContentLength >= 0 {
					metrics.requestSize.Record(ctx, float64(r.ContentLength), attrs...)
				}
				metrics.responseSize.Record(ctx, float64(mw.written), attrs...)
			}(r.Context())
			next.ServeHTTP(mw, r)
		})
	}, nil
}

type (
	metricRouteKey struct{}
	metricRoute    struct {
		mu      sync.Mutex
		pattern string
	}
)

func (r *metricRoute) name() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.pattern
}

// SetMetricRoute records a registered pattern, not an untrusted URL. Routers call it before returning through metrics middleware.
func SetMetricRoute(r *http.Request, pattern string) {
	if route, ok := r.Context().Value(metricRouteKey{}).(*metricRoute); ok && pattern != "" {
		route.mu.Lock()
		defer route.mu.Unlock()
		route.pattern = pattern
	}
}

func metricMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions, http.MethodConnect, http.MethodTrace:
		return method
	default:
		return "OTHER"
	}
}

// RegisterMetricsEndpoint registers a /metrics endpoint on the given mux that exposes Prometheus metrics.
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
