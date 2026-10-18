package observability

import (
	"context"
	"errors"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/embedded"
)

// Gauge is one observable gauge in a set registered by [ObserveGauges]. It reads its value from the set's snapshot.
// Build it with [Int64Gauge] or [Float64Gauge].
type Gauge[T any] struct {
	name     string
	opts     []InstrumentOption
	intValue func(T) int64
	value    func(T) float64
}

// Int64Gauge is an integer gauge that reads value from the snapshot.
func Int64Gauge[T any](name string, value func(T) int64, opts ...InstrumentOption) Gauge[T] {
	return Gauge[T]{name: name, opts: opts, intValue: value}
}

// Float64Gauge is a floating-point gauge that reads value from the snapshot.
func Float64Gauge[T any](name string, value func(T) float64, opts ...InstrumentOption) Gauge[T] {
	return Gauge[T]{name: name, opts: opts, value: value}
}

// ObserveGauges registers gauges on meter with one callback that takes a single snapshot per collection, so values
// that belong together are read together. A nil meter registers nothing. Unregister the result when its owner stops.
func ObserveGauges[T any](meter metric.Meter, snapshot func() T, gauges ...Gauge[T]) (metric.Registration, error) {
	if meter == nil {
		return nopRegistration{}, nil
	}
	if snapshot == nil || len(gauges) == 0 {
		return nil, errors.New("observability: gauges need a snapshot and at least one gauge")
	}
	ints := make([]metric.Int64ObservableGauge, len(gauges))
	floats := make([]metric.Float64ObservableGauge, len(gauges))
	instruments := make([]metric.Observable, 0, len(gauges))
	var errs []error
	for i, g := range gauges {
		cfg := instrumentOptions{}
		for _, opt := range g.opts {
			opt(&cfg)
		}
		switch {
		case g.intValue != nil:
			inst, err := meter.Int64ObservableGauge(g.name, metric.WithDescription(cfg.description), metric.WithUnit(cfg.unit))
			ints[i] = inst
			errs = append(errs, err)
			if err == nil {
				instruments = append(instruments, inst)
			}
		case g.value != nil:
			inst, err := meter.Float64ObservableGauge(g.name, metric.WithDescription(cfg.description), metric.WithUnit(cfg.unit))
			floats[i] = inst
			errs = append(errs, err)
			if err == nil {
				instruments = append(instruments, inst)
			}
		default:
			errs = append(errs, errors.New("observability: gauge "+g.name+" has no value"))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		s := snapshot()
		for i, g := range gauges {
			if g.intValue != nil {
				o.ObserveInt64(ints[i], g.intValue(s))
			} else {
				o.ObserveFloat64(floats[i], g.value(s))
			}
		}
		return nil
	}, instruments...)
}

type nopRegistration struct{ embedded.Registration }

func (nopRegistration) Unregister() error { return nil }
