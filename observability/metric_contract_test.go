package observability

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// The canonical metric names and attribute keys are a cross-kit contract. This
// test drives the recorders through a real SDK reader (no exporter) and locks
// the exact instrument names and attribute keys that are emitted.

func TestCanonicalMetricNamesAndAttributes(t *testing.T) {
	t.Parallel()

	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))
	m, err := NewMetrics(provider.Meter("test"))
	if err != nil {
		t.Fatalf("NewMetrics: %v", err)
	}

	ctx := context.Background()
	m.RecordRequestStart(ctx)
	m.RecordRequestEnd(ctx, RequestMetric{Service: "orders", Method: "GET /x", Status: "ok", Duration: 100 * time.Millisecond})
	m.RecordOperation(ctx, OperationMetric{Service: "orders", Operation: "create", Status: "ok", Duration: 50 * time.Millisecond})
	m.RecordError(ctx, "validation", "handler")

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	names := map[string]map[string]bool{} // metric name -> attribute keys present
	for _, sm := range rm.ScopeMetrics {
		for _, md := range sm.Metrics {
			names[md.Name] = attributeKeys(md.Data)
		}
	}

	wantNames := []string{
		"request.total", "request.duration", "request.active",
		"operation.total", "operation.duration", "error.total",
	}
	for _, n := range wantNames {
		if _, ok := names[n]; !ok {
			t.Errorf("missing canonical metric %q; got %v", n, keys(names))
		}
	}

	assertAttrs(t, names, "request.total", "service", "method", "status")
	assertAttrs(t, names, "operation.total", "service", "operation", "status")
	assertAttrs(t, names, "error.total", "type", "component")
}

func assertAttrs(t *testing.T, names map[string]map[string]bool, metricName string, want ...string) {
	t.Helper()
	got := names[metricName]
	for _, k := range want {
		if !got[k] {
			t.Errorf("metric %q missing attribute %q; got %v", metricName, k, keys(got))
		}
	}
}

func attributeKeys(data metricdata.Aggregation) map[string]bool {
	out := map[string]bool{}
	switch d := data.(type) {
	case metricdata.Sum[int64]:
		for _, dp := range d.DataPoints {
			for _, kv := range dp.Attributes.ToSlice() {
				out[string(kv.Key)] = true
			}
		}
	case metricdata.Sum[float64]:
		for _, dp := range d.DataPoints {
			for _, kv := range dp.Attributes.ToSlice() {
				out[string(kv.Key)] = true
			}
		}
	case metricdata.Histogram[float64]:
		for _, dp := range d.DataPoints {
			for _, kv := range dp.Attributes.ToSlice() {
				out[string(kv.Key)] = true
			}
		}
	}
	return out
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
