package metrics

import (
	"context"

	"go.opentelemetry.io/otel/metric"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/sse"
	"github.com/kbukum/gokit/util"
)

// Register observes a coherent bus snapshot without identity labels. The composition root owns the returned registration and unregisters it before closing telemetry.
func Register(bus *sse.Bus, meter metric.Meter) (metric.Registration, error) {
	if bus == nil || util.IsNil(meter) {
		return nil, apperrors.InvalidInput("metrics", "SSE bus and meter are required")
	}
	gauges := make([]metric.Int64ObservableGauge, 5)
	counters := make([]metric.Int64ObservableCounter, 3)
	observables := make([]metric.Observable, 0, len(gauges)+len(counters))
	for i, name := range []string{"sse.streams.active", "sse.queue.depth", "sse.queue.bytes", "sse.replay.events", "sse.replay.bytes"} {
		gauge, err := meter.Int64ObservableGauge(name)
		if err != nil {
			return nil, err
		}
		gauges[i] = gauge
		observables = append(observables, gauge)
	}
	for i, name := range []string{"sse.events.dropped", "sse.streams.reset", "sse.connections.rejected"} {
		counter, err := meter.Int64ObservableCounter(name)
		if err != nil {
			return nil, err
		}
		counters[i] = counter
		observables = append(observables, counter)
	}
	return meter.RegisterCallback(func(_ context.Context, observer metric.Observer) error {
		stats := bus.Stats()
		for i, value := range []int{stats.ActiveStreams, stats.QueueDepth, stats.QueueBytes, stats.ReplayEvents, stats.ReplayBytes} {
			observer.ObserveInt64(gauges[i], int64(value))
		}
		for i, value := range []uint64{stats.Drops, stats.Resets, stats.RejectedConnections} {
			observer.ObserveInt64(counters[i], int64(value))
		}
		return nil
	}, observables...)
}
