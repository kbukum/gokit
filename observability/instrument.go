package observability

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// MetricAttribute is a typed, transport-neutral metric attribute.
type MetricAttribute struct {
	key   string
	value any
}

// MetricStringAttribute creates a string metric attribute.
func MetricStringAttribute(key, value string) MetricAttribute {
	return MetricAttribute{key: key, value: value}
}

// MetricIntAttribute creates an int metric attribute.
func MetricIntAttribute(key string, value int) MetricAttribute {
	return MetricAttribute{key: key, value: value}
}

// MetricInt64Attribute creates an int64 metric attribute.
func MetricInt64Attribute(key string, value int64) MetricAttribute {
	return MetricAttribute{key: key, value: value}
}

// MetricFloat64Attribute creates a float64 metric attribute.
func MetricFloat64Attribute(key string, value float64) MetricAttribute {
	return MetricAttribute{key: key, value: value}
}

// MetricBoolAttribute creates a bool metric attribute.
func MetricBoolAttribute(key string, value bool) MetricAttribute {
	return MetricAttribute{key: key, value: value}
}

func metricAttributes(attrs []MetricAttribute) metric.MeasurementOption {
	values := make([]attribute.KeyValue, 0, len(attrs))
	for _, attr := range attrs {
		switch value := attr.value.(type) {
		case string:
			values = append(values, attribute.String(attr.key, value))
		case int:
			values = append(values, attribute.Int(attr.key, value))
		case int64:
			values = append(values, attribute.Int64(attr.key, value))
		case float64:
			values = append(values, attribute.Float64(attr.key, value))
		case bool:
			values = append(values, attribute.Bool(attr.key, value))
		}
	}
	return metric.WithAttributes(values...)
}

// InstrumentOption configures a metric instrument.
type InstrumentOption func(*instrumentOptions)

type instrumentOptions struct {
	description string
	unit        string
}

// WithInstrumentDescription sets the instrument description.
func WithInstrumentDescription(description string) InstrumentOption {
	return func(opts *instrumentOptions) {
		opts.description = description
	}
}

// WithInstrumentUnit sets the instrument unit.
func WithInstrumentUnit(unit string) InstrumentOption {
	return func(opts *instrumentOptions) {
		opts.unit = unit
	}
}

func counterOptions(opts []InstrumentOption) []metric.Int64CounterOption {
	cfg := instrumentOptions{}
	for _, opt := range opts {
		opt(&cfg)
	}
	out := []metric.Int64CounterOption{}
	if cfg.description != "" {
		out = append(out, metric.WithDescription(cfg.description))
	}
	if cfg.unit != "" {
		out = append(out, metric.WithUnit(cfg.unit))
	}
	return out
}

func histogramOptions(opts []InstrumentOption) []metric.Float64HistogramOption {
	cfg := instrumentOptions{}
	for _, opt := range opts {
		opt(&cfg)
	}
	out := []metric.Float64HistogramOption{}
	if cfg.description != "" {
		out = append(out, metric.WithDescription(cfg.description))
	}
	if cfg.unit != "" {
		out = append(out, metric.WithUnit(cfg.unit))
	}
	return out
}

// Int64Counter wraps an int64 counter instrument.
type Int64Counter struct {
	counter metric.Int64Counter
}

// NewInt64Counter creates an int64 counter from the named meter.
func NewInt64Counter(meterName, instrumentName string, opts ...InstrumentOption) (*Int64Counter, error) {
	counter, err := Meter(meterName).Int64Counter(instrumentName, counterOptions(opts)...)
	if err != nil {
		return nil, err
	}
	return &Int64Counter{counter: counter}, nil
}

// Add records a counter increment.
func (c *Int64Counter) Add(ctx context.Context, value int64, attrs ...MetricAttribute) {
	if c != nil {
		c.counter.Add(ctx, value, metricAttributes(attrs))
	}
}

// Float64Histogram wraps a float64 histogram instrument.
type Float64Histogram struct {
	histogram metric.Float64Histogram
}

// NewFloat64Histogram creates a float64 histogram from the named meter.
func NewFloat64Histogram(meterName, instrumentName string, opts ...InstrumentOption) (*Float64Histogram, error) {
	histogram, err := Meter(meterName).Float64Histogram(instrumentName, histogramOptions(opts)...)
	if err != nil {
		return nil, err
	}
	return &Float64Histogram{histogram: histogram}, nil
}

// Record records a histogram value.
func (h *Float64Histogram) Record(ctx context.Context, value float64, attrs ...MetricAttribute) {
	if h != nil {
		h.histogram.Record(ctx, value, metricAttributes(attrs))
	}
}
