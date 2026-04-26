package logger

import (
	"context"
	"testing"
	"time"

	otellog "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
)

func TestOTLPConfigDefaults(t *testing.T) {
	t.Parallel()

	cfg := &Config{}
	cfg.ApplyDefaults()

	if cfg.OTLP.Protocol != "grpc" {
		t.Errorf("expected default OTLP protocol 'grpc', got %q", cfg.OTLP.Protocol)
	}
	if cfg.OTLP.Endpoint != "localhost:4317" {
		t.Errorf("expected default OTLP endpoint 'localhost:4317', got %q", cfg.OTLP.Endpoint)
	}
	if cfg.OTLP.Enabled {
		t.Error("expected OTLP to be disabled by default")
	}
}

func TestOTLPConfigPreserveExplicit(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		OTLP: OTLPConfig{
			Protocol: "http",
			Endpoint: "collector.example.com:4318",
		},
	}
	cfg.ApplyDefaults()

	if cfg.OTLP.Protocol != "http" {
		t.Errorf("expected protocol 'http', got %q", cfg.OTLP.Protocol)
	}
	if cfg.OTLP.Endpoint != "collector.example.com:4318" {
		t.Errorf("expected endpoint 'collector.example.com:4318', got %q", cfg.OTLP.Endpoint)
	}
}

func TestMapSeverity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected otellog.Severity
	}{
		{"trace", otellog.SeverityTrace},
		{"TRACE", otellog.SeverityTrace},
		{"debug", otellog.SeverityDebug},
		{"DEBUG", otellog.SeverityDebug},
		{"info", otellog.SeverityInfo},
		{"INFO", otellog.SeverityInfo},
		{"warn", otellog.SeverityWarn},
		{"WARN", otellog.SeverityWarn},
		{"warning", otellog.SeverityWarn},
		{"error", otellog.SeverityError},
		{"ERROR", otellog.SeverityError},
		{"fatal", otellog.SeverityFatal},
		{"FATAL", otellog.SeverityFatal},
		{"unknown", otellog.SeverityUndefined},
		{"", otellog.SeverityUndefined},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got := mapSeverity(tt.input)
			if got != tt.expected {
				t.Errorf("mapSeverity(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestToOTLPKeyValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		key   string
		value interface{}
		check func(t *testing.T, kv otellog.KeyValue)
	}{
		{
			name: "string",
			key:  "name", value: "alice",
			check: func(t *testing.T, kv otellog.KeyValue) {
				if kv.Key != "name" {
					t.Errorf("key = %q, want 'name'", kv.Key)
				}
				if kv.Value.AsString() != "alice" {
					t.Errorf("value = %q, want 'alice'", kv.Value.AsString())
				}
			},
		},
		{
			name: "bool",
			key:  "active", value: true,
			check: func(t *testing.T, kv otellog.KeyValue) {
				if !kv.Value.AsBool() {
					t.Error("expected true")
				}
			},
		},
		{
			name: "int",
			key:  "count", value: 42,
			check: func(t *testing.T, kv otellog.KeyValue) {
				if kv.Value.AsInt64() != 42 {
					t.Errorf("value = %d, want 42", kv.Value.AsInt64())
				}
			},
		},
		{
			name: "int64",
			key:  "big", value: int64(1234567890),
			check: func(t *testing.T, kv otellog.KeyValue) {
				if kv.Value.AsInt64() != 1234567890 {
					t.Errorf("value = %d, want 1234567890", kv.Value.AsInt64())
				}
			},
		},
		{
			name: "float64",
			key:  "rate", value: 3.14,
			check: func(t *testing.T, kv otellog.KeyValue) {
				if kv.Value.AsFloat64() != 3.14 {
					t.Errorf("value = %f, want 3.14", kv.Value.AsFloat64())
				}
			},
		},
		{
			name: "bytes",
			key:  "data", value: []byte{0x01, 0x02},
			check: func(t *testing.T, kv otellog.KeyValue) {
				b := kv.Value.AsBytes()
				if len(b) != 2 || b[0] != 0x01 || b[1] != 0x02 {
					t.Errorf("unexpected bytes: %v", b)
				}
			},
		},
		{
			name: "fallback to string",
			key:  "custom", value: struct{ X int }{X: 1},
			check: func(t *testing.T, kv otellog.KeyValue) {
				if kv.Value.AsString() != "{1}" {
					t.Errorf("value = %q, want '{1}'", kv.Value.AsString())
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			kv := toOTLPKeyValue(tt.key, tt.value)
			tt.check(t, kv)
		})
	}
}

// inMemoryExporter captures log records for testing.
type inMemoryExporter struct {
	records []sdklog.Record
}

func (e *inMemoryExporter) Export(_ context.Context, records []sdklog.Record) error {
	e.records = append(e.records, records...)
	return nil
}

func (e *inMemoryExporter) Shutdown(_ context.Context) error   { return nil }
func (e *inMemoryExporter) ForceFlush(_ context.Context) error { return nil }

func newTestOTLPProvider(exporter sdklog.Exporter) *OTLPProvider {
	processor := sdklog.NewSimpleProcessor(exporter)
	provider := sdklog.NewLoggerProvider(sdklog.WithProcessor(processor))
	return &OTLPProvider{
		provider: provider,
		logger:   provider.Logger(otlpLoggerName),
	}
}

func TestEmitLogFieldConversion(t *testing.T) {
	t.Parallel()

	exp := &inMemoryExporter{}
	p := newTestOTLPProvider(exp)
	defer p.Shutdown(context.Background())

	fields := map[string]interface{}{
		"user":  "bob",
		"count": 5,
	}
	p.EmitLog(context.Background(), "info", "hello world", fields)

	if len(exp.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(exp.records))
	}

	rec := exp.records[0]

	if rec.Severity() != otellog.SeverityInfo {
		t.Errorf("severity = %v, want INFO", rec.Severity())
	}
	if rec.SeverityText() != "INFO" {
		t.Errorf("severity text = %q, want 'INFO'", rec.SeverityText())
	}
	if rec.Body().AsString() != "hello world" {
		t.Errorf("body = %q, want 'hello world'", rec.Body().AsString())
	}

	attrs := make(map[string]otellog.Value)
	rec.WalkAttributes(func(kv otellog.KeyValue) bool {
		attrs[kv.Key] = kv.Value
		return true
	})

	if v, ok := attrs["user"]; !ok || v.AsString() != "bob" {
		t.Errorf("expected attr user=bob, got %v", attrs["user"])
	}
	if v, ok := attrs["count"]; !ok || v.AsInt64() != 5 {
		t.Errorf("expected attr count=5, got %v", attrs["count"])
	}
}

func TestEmitLogNilProvider(t *testing.T) {
	t.Parallel()

	// Should not panic.
	var p *OTLPProvider
	p.EmitLog(context.Background(), "info", "should not panic", nil)
}

func TestShutdownNilProvider(t *testing.T) {
	t.Parallel()

	var p *OTLPProvider
	if err := p.Shutdown(context.Background()); err != nil {
		t.Errorf("expected nil error from nil provider shutdown, got %v", err)
	}
}

func TestShutdownGraceful(t *testing.T) {
	t.Parallel()

	exp := &inMemoryExporter{}
	p := newTestOTLPProvider(exp)

	p.EmitLog(context.Background(), "info", "before shutdown", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := p.Shutdown(ctx); err != nil {
		t.Errorf("shutdown error: %v", err)
	}
}

func TestLoggerWithOTLP(t *testing.T) {
	t.Parallel()

	exp := &inMemoryExporter{}
	provider := newTestOTLPProvider(exp)

	l := NewDefault("test-svc").WithOTLP(provider)
	defer l.Close()

	l.Info("test message", map[string]interface{}{"key": "value"})

	if len(exp.records) != 1 {
		t.Fatalf("expected 1 OTLP record, got %d", len(exp.records))
	}

	rec := exp.records[0]
	if rec.Body().AsString() != "test message" {
		t.Errorf("body = %q, want 'test message'", rec.Body().AsString())
	}
}

func TestLoggerCloseWithoutOTLP(t *testing.T) {
	t.Parallel()

	l := NewDefault("test-svc")
	if err := l.Close(); err != nil {
		t.Errorf("expected nil error from close without OTLP, got %v", err)
	}
}

func TestLoggerOTLPPropagation(t *testing.T) {
	t.Parallel()

	exp := &inMemoryExporter{}
	provider := newTestOTLPProvider(exp)

	l := NewDefault("test-svc").WithOTLP(provider)
	defer l.Close()

	// WithComponent should propagate the OTLP provider.
	cl := l.WithComponent("db")
	cl.Warn("component log")

	if len(exp.records) != 1 {
		t.Fatalf("expected 1 OTLP record from child logger, got %d", len(exp.records))
	}
	if exp.records[0].Severity() != otellog.SeverityWarn {
		t.Errorf("severity = %v, want WARN", exp.records[0].Severity())
	}
}

func TestNewOTLPProviderGRPC(t *testing.T) {
	t.Parallel()

	cfg := OTLPConfig{
		Enabled:  true,
		Endpoint: "localhost:4317",
		Protocol: "grpc",
		Insecure: true,
	}

	p, err := NewOTLPProvider(cfg, "test-svc", "test", "1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer p.Shutdown(context.Background())

	if p.provider == nil {
		t.Error("expected non-nil provider")
	}
	if p.logger == nil {
		t.Error("expected non-nil logger")
	}
}

func TestNewOTLPProviderHTTP(t *testing.T) {
	t.Parallel()

	cfg := OTLPConfig{
		Enabled:  true,
		Endpoint: "localhost:4318",
		Protocol: "http",
		Insecure: true,
	}

	p, err := NewOTLPProvider(cfg, "test-svc", "test", "1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer p.Shutdown(context.Background())

	if p.provider == nil {
		t.Error("expected non-nil provider")
	}
}
