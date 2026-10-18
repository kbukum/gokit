package observability

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

type backlog struct {
	rows int64
	age  float64
}

// Every gauge in a set reads the same snapshot, taken once per collection.
func TestObserveGaugesReadsOneSnapshot(t *testing.T) {
	t.Parallel()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	defer func() { _ = provider.Shutdown(context.Background()) }()

	reads := 0
	reg, err := ObserveGauges(provider.Meter("test"), func() backlog { reads++; return backlog{rows: 3, age: 1.5} },
		Int64Gauge("unsent", func(b backlog) int64 { return b.rows }, WithInstrumentDescription("rows")),
		Float64Gauge("age", func(b backlog) float64 { return b.age }, WithInstrumentUnit("s")),
	)
	if err != nil {
		t.Fatal(err)
	}
	got := collect(t, reader)
	if reads != 1 || got["unsent"] != 3 || got["age"] != 1.5 {
		t.Fatalf("reads %d, values %v", reads, got)
	}
	if err := reg.Unregister(); err != nil {
		t.Fatal(err)
	}
	if got := collect(t, reader); reads != 1 || len(got) != 0 {
		t.Fatalf("after unregister: reads %d, values %v", reads, got)
	}
}

func TestObserveGaugesWithoutMeterOrGaugesRegistersNothing(t *testing.T) {
	t.Parallel()
	reg, err := ObserveGauges[backlog](nil, func() backlog { t.Fatal("read"); return backlog{} },
		Int64Gauge("unsent", func(b backlog) int64 { return b.rows }))
	if err != nil || reg.Unregister() != nil {
		t.Fatalf("nil meter: %v", err)
	}
	if _, err := ObserveGauges[backlog](noop.NewMeterProvider().Meter("t"), func() backlog { return backlog{} }); err == nil {
		t.Fatal("no gauges: want error")
	}
	if _, err := ObserveGauges[backlog](noop.NewMeterProvider().Meter("t"), nil, Int64Gauge("x", func(backlog) int64 { return 0 })); err == nil {
		t.Fatal("nil snapshot: want error")
	}
	if _, err := ObserveGauges(noop.NewMeterProvider().Meter("t"), func() backlog { return backlog{} }, Int64Gauge[backlog]("x", nil)); err == nil {
		t.Fatal("nil value: want error")
	}
}

func TestObserveGaugesReportsInstrumentErrors(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	_, err := ObserveGauges(failingGaugeMeter{err: boom}, func() backlog { return backlog{} },
		Int64Gauge("a", func(b backlog) int64 { return b.rows }), Float64Gauge("b", func(b backlog) float64 { return b.age }))
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}

type failingGaugeMeter struct {
	metric.Meter
	err error
}

func (m failingGaugeMeter) Int64ObservableGauge(string, ...metric.Int64ObservableGaugeOption) (metric.Int64ObservableGauge, error) {
	return nil, m.err
}

func (m failingGaugeMeter) Float64ObservableGauge(string, ...metric.Float64ObservableGaugeOption) (metric.Float64ObservableGauge, error) {
	return nil, m.err
}

func collect(t *testing.T, reader *sdkmetric.ManualReader) map[string]float64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}
	out := map[string]float64{}
	for _, scope := range rm.ScopeMetrics {
		for _, m := range scope.Metrics {
			switch data := m.Data.(type) {
			case metricdata.Gauge[int64]:
				for _, p := range data.DataPoints {
					out[m.Name] = float64(p.Value)
				}
			case metricdata.Gauge[float64]:
				for _, p := range data.DataPoints {
					out[m.Name] = p.Value
				}
			}
		}
	}
	return out
}
