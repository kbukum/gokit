package main

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/kbukum/gokit/observability"
)

func TestRunDropsOverflowDeterministically(t *testing.T) {
	t.Parallel()

	cfg := RunConfig{Subscribers: 3, Buffer: 4, Events: 12, Source: "config-file"}
	result, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(result.Healthy) != cfg.Subscribers {
		t.Fatalf("healthy subscribers = %d, want %d", len(result.Healthy), cfg.Subscribers)
	}
	for i, got := range result.Healthy {
		if got != cfg.Events {
			t.Errorf("healthy subscriber %d received %d, want %d (should keep up)", i, got, cfg.Events)
		}
	}
	if result.SlowReceived != cfg.Buffer {
		t.Errorf("slow subscriber buffered %d, want %d", result.SlowReceived, cfg.Buffer)
	}
	wantDropped := uint64(cfg.Events - cfg.Buffer)
	if result.Dropped != wantDropped {
		t.Errorf("dropped = %d, want %d", result.Dropped, wantDropped)
	}
}

func TestRunNoDropsWhenBufferCoversEvents(t *testing.T) {
	t.Parallel()

	cfg := RunConfig{Subscribers: 2, Buffer: 8, Events: 5, Source: "config-file"}
	result, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Dropped != 0 {
		t.Errorf("dropped = %d, want 0 when buffer covers all events", result.Dropped)
	}
	if result.SlowReceived != cfg.Events {
		t.Errorf("slow buffered %d, want %d (no overflow)", result.SlowReceived, cfg.Events)
	}
}

func TestRunRejectsInvalidConfig(t *testing.T) {
	t.Parallel()

	cases := map[string]RunConfig{
		"no subscribers":  {Subscribers: 0, Buffer: 1, Events: 1},
		"zero buffer":     {Subscribers: 1, Buffer: 0, Events: 1},
		"negative events": {Subscribers: 1, Buffer: 1, Events: -1},
	}
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := Run(context.Background(), cfg); err == nil {
				t.Fatalf("Run(%+v) = nil error, want validation error", cfg)
			}
		})
	}
}

// TestRunBridgesDropsToObservabilityCounter proves the OnDrop hook reaches the
// injected observability counter: the counter's collected sum must equal the
// broadcaster's own drop count. It drives a real SDK ManualReader (the idiomatic
// OTEL in-memory verification, as in observability's own tests) rather than a
// hand-rolled counter fake. It mutates the global meter provider, so it is not
// parallel and restores the previous provider on cleanup.
func TestRunBridgesDropsToObservabilityCounter(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	prev := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)
	t.Cleanup(func() {
		otel.SetMeterProvider(prev)
		_ = provider.Shutdown(context.Background())
	})

	const metricName = "config_change_dropped_total"
	counter, err := observability.NewInt64Counter("broadcast-demo", metricName)
	if err != nil {
		t.Fatalf("NewInt64Counter: %v", err)
	}

	cfg := RunConfig{Subscribers: 2, Buffer: 3, Events: 10, Source: "config-file", DropCounter: counter}
	result, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	wantDropped := uint64(cfg.Events - cfg.Buffer)
	if result.Dropped != wantDropped {
		t.Fatalf("dropped = %d, want %d", result.Dropped, wantDropped)
	}

	got := collectCounterSum(t, reader, metricName)
	if got != int64(result.Dropped) {
		t.Fatalf("observability counter = %d, want %d (== broadcaster DroppedCount)", got, result.Dropped)
	}
}

// collectCounterSum reads the summed value of the named int64 counter from reader.
func collectCounterSum(t *testing.T, reader sdkmetric.Reader, name string) int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	for _, sm := range rm.ScopeMetrics {
		for _, md := range sm.Metrics {
			if md.Name != name {
				continue
			}
			sum, ok := md.Data.(metricdata.Sum[int64])
			if !ok {
				t.Fatalf("metric %q is %T, want metricdata.Sum[int64]", name, md.Data)
			}
			var total int64
			for _, dp := range sum.DataPoints {
				total += dp.Value
			}
			return total
		}
	}
	t.Fatalf("counter %q not found in collected metrics", name)
	return 0
}
