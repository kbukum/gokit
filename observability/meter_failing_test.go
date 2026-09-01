package observability

import (
	"errors"
	"testing"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"

	apperr "github.com/kbukum/gokit/errors"
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
		_, err := NewMetrics(&failingMeter{Meter: base, failOn: name})
		if err == nil {
			t.Fatalf("expected error when %s creation fails", name)
		}
		appErr, ok := apperr.AsAppError(err)
		if !ok {
			t.Fatalf("%s: expected AppError, got %T", name, err)
		}
		if appErr.Code != apperr.ErrCodeInternal || appErr.HTTPStatus != 500 {
			t.Fatalf("%s: code/status = %q/%d, want INTERNAL_ERROR/500", name, appErr.Code, appErr.HTTPStatus)
		}
		if !errors.Is(err, errInstrument) {
			t.Fatalf("%s: expected underlying instrument error preserved as cause, got %v", name, err)
		}
	}
}
