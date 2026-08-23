package observability

import (
	"context"
	"testing"
)

func TestNewTraceExporterProtocols(t *testing.T) {
	t.Parallel()
	for _, proto := range []OTLPProtocol{OTLPHTTP, OTLPGRPC} {
		cfg := &TracerConfig{Endpoint: "localhost:4318", Insecure: true, Protocol: proto}
		exp, err := newTraceExporter(context.Background(), cfg)
		if err != nil {
			t.Fatalf("newTraceExporter(%v): %v", proto, err)
		}
		if exp == nil {
			t.Fatalf("newTraceExporter(%v) returned nil exporter", proto)
		}
		_ = exp.Shutdown(context.Background())
	}
	if _, err := newTraceExporter(context.Background(), &TracerConfig{Protocol: OTLPProtocol(99)}); err == nil {
		t.Fatal("newTraceExporter with unknown protocol = nil error, want rejection")
	}
}

func TestNewMetricExporterProtocols(t *testing.T) {
	t.Parallel()
	for _, proto := range []OTLPProtocol{OTLPHTTP, OTLPGRPC} {
		cfg := &MeterConfig{Endpoint: "localhost:4317", Insecure: true, Protocol: proto}
		exp, err := newMetricExporter(context.Background(), cfg)
		if err != nil {
			t.Fatalf("newMetricExporter(%v): %v", proto, err)
		}
		if exp == nil {
			t.Fatalf("newMetricExporter(%v) returned nil exporter", proto)
		}
		_ = exp.Shutdown(context.Background())
	}
	if _, err := newMetricExporter(context.Background(), &MeterConfig{Protocol: OTLPProtocol(99)}); err == nil {
		t.Fatal("newMetricExporter with unknown protocol = nil error, want rejection")
	}
}
