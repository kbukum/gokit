package sqlite_test

import (
	"context"
	"strings"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	dbmetrics "github.com/kbukum/gokit/database/metrics"
)

func TestPoolMetricsRegistration(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := newTestDB(t)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, err := dbmetrics.Register(db, nil, "app"); err == nil {
		t.Fatal("nil meter accepted")
	}
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() {
		if err := provider.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	meter := provider.Meter("database-test")
	for _, name := range []string{"", " ", " app", "app ", strings.Repeat("a", 129)} {
		if _, err := dbmetrics.Register(db, meter, name); err == nil {
			t.Errorf("invalid database identity %q accepted", name)
		}
	}
	registration, err := dbmetrics.Register(db, meter, "app")
	if err != nil {
		t.Fatal(err)
	}
	var observed metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &observed); err != nil {
		t.Fatal(err)
	}
	if len(observed.ScopeMetrics) != 1 || len(observed.ScopeMetrics[0].Metrics) != 6 {
		t.Fatalf("metrics: %+v", observed)
	}
	for _, observedMetric := range observed.ScopeMetrics[0].Metrics {
		expected := int64(1)
		switch observedMetric.Name {
		case "db.pool.connections.in_use":
			expected = 0
		case "db.pool.connections.open", "db.pool.connections.idle":
		default:
			continue
		}
		gauge, ok := observedMetric.Data.(metricdata.Gauge[int64])
		if !ok || len(gauge.DataPoints) != 1 || gauge.DataPoints[0].Value != expected {
			t.Fatalf("%s = %+v, expected %d", observedMetric.Name, observedMetric.Data, expected)
		}
	}
	if err := registration.Unregister(); err != nil {
		t.Fatal(err)
	}
}
