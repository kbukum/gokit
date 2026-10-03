package middleware_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/kbukum/gokit/server/middleware"
)

func TestRandomPathsHaveBoundedMetrics(t *testing.T) {
	t.Parallel()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	defer func() { _ = provider.Shutdown(t.Context()) }()
	instrument, err := middleware.PrometheusMetrics("test", provider.Meter("test"))
	if err != nil {
		t.Fatal(err)
	}
	handler := instrument(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(404) }))
	for i := range 1000 {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, fmt.Sprintf("/unknown/%d", i), http.NoBody))
	}
	var data metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &data); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, scope := range data.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name == "http_requests_total" {
				sum, ok := metric.Data.(metricdata.Sum[int64])
				if !ok || len(sum.DataPoints) != 1 || sum.DataPoints[0].Value != 1000 {
					t.Fatalf("random paths expanded series: %+v", metric.Data)
				}
				found = true
			}
		}
	}
	if !found {
		t.Fatal("request counter missing")
	}
}
