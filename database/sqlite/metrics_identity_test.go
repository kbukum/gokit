package sqlite_test

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	dbmetrics "github.com/kbukum/gokit/database/metrics"
)

func TestPoolMetricsSeparateDatabases(t *testing.T) {
	t.Parallel()
	first, second := newTestDB(t), newTestDB(t)
	t.Cleanup(func() {
		for _, db := range []interface{ Close() error }{first, second} {
			if err := db.Close(); err != nil {
				t.Error(err)
			}
		}
	})
	pool, err := first.GormDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	conn, err := pool.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
	})
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	meter := provider.Meter("database-test")
	firstRegistration, err := dbmetrics.Register(first, meter, "first")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := firstRegistration.Unregister(); err != nil {
			t.Error(err)
		}
	})
	secondRegistration, err := dbmetrics.Register(second, meter, "second")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := secondRegistration.Unregister(); err != nil {
			t.Error(err)
		}
	})
	var observed metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &observed); err != nil {
		t.Fatal(err)
	}
	if len(observed.ScopeMetrics) != 1 || len(observed.ScopeMetrics[0].Metrics) != 6 {
		t.Fatalf("metrics: %+v", observed)
	}
	for _, measured := range observed.ScopeMetrics[0].Metrics {
		values := make(map[string]float64)
		record := func(attributes attribute.Set, value float64) {
			name, ok := attributes.Value("database")
			if !ok {
				t.Fatalf("%s has no database identity", measured.Name)
			}
			if _, exists := values[name.AsString()]; exists {
				t.Fatalf("%s has duplicate database identity %s", measured.Name, name.AsString())
			}
			if measured.Name != "db.queries.slow" {
				poolName, ok := attributes.Value("pool")
				if !ok || poolName.AsString() != "primary" {
					t.Fatalf("%s lost its pool label", measured.Name)
				}
			}
			values[name.AsString()] = value
		}
		switch data := measured.Data.(type) {
		case metricdata.Gauge[int64]:
			for _, point := range data.DataPoints {
				record(point.Attributes, float64(point.Value))
			}
		case metricdata.Sum[int64]:
			for _, point := range data.DataPoints {
				record(point.Attributes, float64(point.Value))
			}
		case metricdata.Sum[float64]:
			for _, point := range data.DataPoints {
				record(point.Attributes, point.Value)
			}
		default:
			t.Fatalf("unexpected metric type: %T", data)
		}
		_, hasFirst := values["first"]
		_, hasSecond := values["second"]
		if len(values) != 2 || !hasFirst || !hasSecond {
			t.Fatalf("%s merged databases: %v", measured.Name, values)
		}
		if measured.Name == "db.pool.connections.in_use" && (values["first"] != 1 || values["second"] != 0) {
			t.Fatalf("pool pressure attributed to the wrong database: %v", values)
		}
	}
}
