package observability

import (
	"errors"
	"testing"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
)

// failingMeter embeds a real meter and returns an error when the requested
// instrument name matches failOn, letting tests exercise NewMetrics error paths.
type failingMeter struct {
	metric.Meter
	failOn string
}

var errInstrument = errors.New("instrument creation failed")

func (m *failingMeter) Int64Counter(name string, opts ...metric.Int64CounterOption) (metric.Int64Counter, error) {
	if name == m.failOn {
		return nil, errInstrument
	}
	return m.Meter.Int64Counter(name, opts...)
}

func (m *failingMeter) Float64Histogram(name string, opts ...metric.Float64HistogramOption) (metric.Float64Histogram, error) {
	if name == m.failOn {
		return nil, errInstrument
	}
	return m.Meter.Float64Histogram(name, opts...)
}

func (m *failingMeter) Int64UpDownCounter(name string, opts ...metric.Int64UpDownCounterOption) (metric.Int64UpDownCounter, error) {
	if name == m.failOn {
		return nil, errInstrument
	}
	return m.Meter.Int64UpDownCounter(name, opts...)
}

func TestNewMetricsInstrumentErrors(t *testing.T) {
	base := noop.NewMeterProvider().Meter("test")
	names := []string{
		"request.total",
		"request.duration",
		"request.active",
		"operation.total",
		"operation.duration",
		"error.total",
	}
	for _, name := range names {
		if _, err := NewMetrics(&failingMeter{Meter: base, failOn: name}); err == nil {
			t.Fatalf("expected error when %s creation fails", name)
		}
	}
}
