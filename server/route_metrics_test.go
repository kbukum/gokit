package server_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/kbukum/gokit/server"
)

func TestServerMetricsUseRegisteredPatterns(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	defer func() { _ = provider.Shutdown(t.Context()) }()
	s := server.New(&server.Config{}, nil)
	if err := s.Instrument(provider.Meter("routes")); err != nil {
		t.Fatal(err)
	}
	s.GinEngine().GET("/users/:id", func(c *gin.Context) { c.Status(http.StatusOK) })
	s.Handle("/service.Method/", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	s.ApplyMiddleware()
	for _, prefix := range []string{"/users/", "/missing/", "/service.Method/"} {
		for i := range 1000 {
			s.Handler().ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, fmt.Sprintf("%s%d", prefix, i), http.NoBody))
		}
	}
	var data metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &data); err != nil {
		t.Fatal(err)
	}
	for _, scope := range data.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != "http_requests_total" {
				continue
			}
			sum, ok := metric.Data.(metricdata.Sum[int64])
			if !ok || len(sum.DataPoints) != 3 {
				t.Fatalf("route series: %+v", metric.Data)
			}
			for _, point := range sum.DataPoints {
				value, ok := point.Attributes.Value("path")
				if !ok {
					t.Fatal("route label missing")
				}
				switch value.AsString() {
				case "/users/:id", "/service.Method/", "unmatched":
				default:
					t.Fatalf("raw route label %q", value.AsString())
				}
				if point.Value != 1000 {
					t.Errorf("route count %d", point.Value)
				}
			}
			return
		}
	}
	t.Fatal("request metrics missing")
}

// TestServerMetricsPreserveRouteOnTimeout guards the ordering fix: the metric
// route is recorded before the Gin handler runs, so a REST request abandoned by
// the per-request timeout still carries its registered pattern instead of the
// "unmatched" fallback.
func TestServerMetricsPreserveRouteOnTimeout(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	defer func() { _ = provider.Shutdown(t.Context()) }()
	s := server.New(&server.Config{RequestTimeout: 1}, nil)
	if err := s.Instrument(provider.Meter("routes")); err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	defer close(release)
	s.GinEngine().GET("/slow/:id", func(c *gin.Context) {
		<-release
		c.Status(http.StatusOK)
	})
	s.ApplyMiddleware()

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/slow/42", http.NoBody))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d", rec.Code)
	}

	var data metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &data); err != nil {
		t.Fatal(err)
	}
	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name != "http_requests_total" {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok {
				t.Fatalf("metric type %T", m.Data)
			}
			for _, point := range sum.DataPoints {
				value, ok := point.Attributes.Value("path")
				if !ok {
					t.Fatal("route label missing")
				}
				if got := value.AsString(); got != "/slow/:id" {
					t.Fatalf("timed-out request labeled %q, want /slow/:id", got)
				}
			}
			return
		}
	}
	t.Fatal("request metrics missing")
}
