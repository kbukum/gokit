package vectorstore

import "testing"

func TestRegistryRequiresExplicitMemoryRegistration(t *testing.T) {
	t.Parallel()

	reg := NewFactoryRegistry()
	if _, ok := reg.Get(ProviderMemory); ok {
		t.Fatal("memory provider registered without explicit RegisterMemory call")
	}
	if err := RegisterMemory(reg); err != nil {
		t.Fatalf("RegisterMemory: %v", err)
	}
	if _, ok := reg.Get(ProviderMemory); !ok {
		t.Fatal("memory provider missing after registration")
	}
}

func TestNewConfigDrivenMemory(t *testing.T) {
	t.Parallel()

	reg := NewFactoryRegistry()
	if err := RegisterMemory(reg); err != nil {
		t.Fatalf("RegisterMemory: %v", err)
	}
	store, err := New(reg, Config{Provider: ProviderMemory, Metric: MetricDot}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if store == nil {
		t.Fatal("expected store")
	}
}

func TestMetricNames(t *testing.T) {
	t.Parallel()

	for _, metric := range []string{MetricCosine, MetricDot, MetricL2} {
		cfg := Config{Metric: metric}
		cfg.ApplyDefaults()
		if err := cfg.Validate(); err != nil {
			t.Fatalf("metric %q should be supported: %v", metric, err)
		}
	}
}
