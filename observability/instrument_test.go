package observability

import (
	"context"
	"testing"
)

func TestInstrumentWrappers(t *testing.T) {
	counter, err := NewInt64Counter("test-meter", "test_counter_total",
		WithInstrumentDescription("test counter"),
	)
	if err != nil {
		t.Fatalf("unexpected counter error: %v", err)
	}
	counter.Add(context.Background(), 1, MetricStringAttribute("kind", "test"))

	unitCounter, err := NewInt64Counter("test-meter", "test_unit_counter_total",
		WithInstrumentUnit("1"),
	)
	if err != nil {
		t.Fatalf("unexpected unit counter error: %v", err)
	}
	unitCounter.Add(context.Background(), 1)

	histogram, err := NewFloat64Histogram("test-meter", "test_duration_seconds",
		WithInstrumentDescription("test histogram"),
		WithInstrumentUnit("s"),
	)
	if err != nil {
		t.Fatalf("unexpected histogram error: %v", err)
	}
	histogram.Record(context.Background(), 1.5, MetricBoolAttribute("ok", true))
}

func TestMetricAttributeConstructors(t *testing.T) {
	counter, err := NewInt64Counter("test-meter", "attr_counter_total")
	if err != nil {
		t.Fatalf("unexpected counter error: %v", err)
	}
	counter.Add(context.Background(), 1,
		MetricStringAttribute("s", "v"),
		MetricIntAttribute("i", 1),
		MetricInt64Attribute("i64", 2),
		MetricFloat64Attribute("f", 1.5),
		MetricBoolAttribute("b", true),
	)
}
