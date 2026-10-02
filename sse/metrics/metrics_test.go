package metrics_test

import (
	"testing"

	otelmetric "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"google.golang.org/protobuf/types/known/apipb"

	"github.com/kbukum/gokit/sse"
	"github.com/kbukum/gokit/sse/metrics"
)

func TestMetricsObserveAndUnregister(t *testing.T) {
	t.Parallel()
	bus, err := sse.NewBus(sse.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer bus.Close()
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))
	defer provider.Shutdown(t.Context())
	reg, err := metrics.Register(bus, provider.Meter("test"))
	if err != nil {
		t.Fatal(err)
	}
	sub, err := bus.Subscribe(t.Context(), sse.SubscribeRequest{Principal: "p", Route: "r"})
	if err != nil {
		t.Fatal(err)
	}
	if err := bus.Publish(t.Context(), "r", &apipb.Method{}); err != nil {
		t.Fatal(err)
	}
	var data metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &data); err != nil {
		t.Fatal(err)
	}
	values := make(map[string]int64)
	for _, scope := range data.ScopeMetrics {
		for _, measurement := range scope.Metrics {
			switch d := measurement.Data.(type) {
			case metricdata.Gauge[int64]:
				values[measurement.Name] = d.DataPoints[0].Value
			case metricdata.Sum[int64]:
				values[measurement.Name] = d.DataPoints[0].Value
			}
		}
	}
	if len(values) != 8 || values["sse.streams.active"] != 1 || values["sse.queue.depth"] != 1 || values["sse.queue.bytes"] <= 0 {
		t.Fatalf("metrics: %+v", values)
	}
	sub.Close()
	if err := reg.Unregister(); err != nil {
		t.Fatal(err)
	}
}

func TestRegisterRejectsTypedNilMeter(t *testing.T) {
	t.Parallel()
	bus, err := sse.NewBus(sse.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer bus.Close()
	var meter otelmetric.Meter = (*nilMeter)(nil)
	if _, err := metrics.Register(bus, meter); err == nil {
		t.Fatal("typed-nil meter accepted")
	}
}

type nilMeter struct{ otelmetric.Meter }
