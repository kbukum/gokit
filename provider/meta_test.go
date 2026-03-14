package provider

import (
	"context"
	"testing"
	"time"
)

// mockRR is a simple RequestResponse for testing.
type mockRR struct {
	name string
}

func (m *mockRR) Name() string                       { return m.name }
func (m *mockRR) IsAvailable(_ context.Context) bool { return true }
func (m *mockRR) Execute(_ context.Context, input string) (string, error) {
	return "result:" + input, nil
}

func TestWithMeta_Execute(t *testing.T) {
	inner := &mockRR{name: "test-provider"}
	wrapped := WithMeta[string, string](inner, Meta{
		"cost":       0.5,
		"latency_ms": 100,
		"requires":   "gpu",
	})

	// Execute should delegate to inner.
	result, err := wrapped.Execute(context.Background(), "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "result:hello" {
		t.Errorf("got %q, want %q", result, "result:hello")
	}
}

func TestWithMeta_Name(t *testing.T) {
	inner := &mockRR{name: "my-service"}
	wrapped := WithMeta[string, string](inner, Meta{"cost": 0.1})

	if wrapped.Name() != "my-service" {
		t.Errorf("Name() = %q, want %q", wrapped.Name(), "my-service")
	}
}

func TestWithMeta_IsAvailable(t *testing.T) {
	inner := &mockRR{name: "test"}
	wrapped := WithMeta[string, string](inner, Meta{})

	if !wrapped.IsAvailable(context.Background()) {
		t.Error("IsAvailable() should return true")
	}
}

func TestGetMeta(t *testing.T) {
	inner := &mockRR{name: "test"}
	meta := Meta{"cost": 0.5, "requires": "gpu"}
	wrapped := WithMeta[string, string](inner, meta)

	got := GetMeta[string, string](wrapped)
	if cost, ok := got.Float("cost"); !ok || cost != 0.5 {
		t.Errorf("cost = %v, %v, want 0.5, true", cost, ok)
	}
	if req, ok := got.String("requires"); !ok || req != "gpu" {
		t.Errorf("requires = %q, %v, want %q, true", req, ok, "gpu")
	}
}

func TestGetMeta_NoMeta(t *testing.T) {
	inner := &mockRR{name: "test"}
	got := GetMeta[string, string](inner)
	if len(got) != 0 {
		t.Errorf("expected empty Meta for unwrapped provider, got %v", got)
	}
}

func TestGetMetaFromAny(t *testing.T) {
	inner := &mockRR{name: "test"}
	wrapped := WithMeta[string, string](inner, Meta{"cost": 1.0})

	got := GetMetaFromAny(wrapped)
	if cost, ok := got.Float("cost"); !ok || cost != 1.0 {
		t.Errorf("cost = %v, %v", cost, ok)
	}

	// Non-meta provider.
	got = GetMetaFromAny(inner)
	if len(got) != 0 {
		t.Errorf("expected empty Meta, got %v", got)
	}
}

func TestMeta_Float(t *testing.T) {
	m := Meta{
		"f64": float64(3.14),
		"f32": float32(2.71),
		"int": 42,
		"i64": int64(100),
		"str": "not a number",
	}

	tests := []struct {
		key  string
		want float64
		ok   bool
	}{
		{"f64", 3.14, true},
		{"f32", 2.71, true},
		{"int", 42, true},
		{"i64", 100, true},
		{"str", 0, false},
		{"missing", 0, false},
	}

	for _, tt := range tests {
		got, ok := m.Float(tt.key)
		if ok != tt.ok {
			t.Errorf("Float(%q) ok = %v, want %v", tt.key, ok, tt.ok)
		}
		if tt.ok && !floatClose(got, tt.want) {
			t.Errorf("Float(%q) = %v, want %v", tt.key, got, tt.want)
		}
	}
}

func TestMeta_String(t *testing.T) {
	m := Meta{"region": "us-east-1", "count": 42}

	s, ok := m.String("region")
	if !ok || s != "us-east-1" {
		t.Errorf("String(region) = %q, %v", s, ok)
	}

	_, ok = m.String("count")
	if ok {
		t.Error("String(count) should return false for int")
	}

	_, ok = m.String("missing")
	if ok {
		t.Error("String(missing) should return false")
	}
}

func TestMeta_Duration(t *testing.T) {
	m := Meta{
		"dur":      500 * time.Millisecond,
		"float_ms": 100.5,
		"int_ms":   200,
		"str":      "bad",
	}

	d, ok := m.Duration("dur")
	if !ok || d != 500*time.Millisecond {
		t.Errorf("Duration(dur) = %v, %v", d, ok)
	}

	d, ok = m.Duration("float_ms")
	if !ok || d != 100500*time.Microsecond {
		t.Errorf("Duration(float_ms) = %v, %v", d, ok)
	}

	d, ok = m.Duration("int_ms")
	if !ok || d != 200*time.Millisecond {
		t.Errorf("Duration(int_ms) = %v, %v", d, ok)
	}

	_, ok = m.Duration("str")
	if ok {
		t.Error("Duration(str) should return false")
	}
}

func TestMeta_Bool(t *testing.T) {
	m := Meta{"enabled": true, "count": 1}

	b, ok := m.Bool("enabled")
	if !ok || !b {
		t.Errorf("Bool(enabled) = %v, %v", b, ok)
	}

	_, ok = m.Bool("count")
	if ok {
		t.Error("Bool(count) should return false for int")
	}
}

func TestMeta_Has(t *testing.T) {
	m := Meta{"key": "value"}
	if !m.Has("key") {
		t.Error("Has(key) should be true")
	}
	if m.Has("missing") {
		t.Error("Has(missing) should be false")
	}
}

func TestMeta_Merge(t *testing.T) {
	a := Meta{"cost": 0.5, "region": "us-east-1"}
	b := Meta{"cost": 1.0, "gpu": true}

	merged := a.Merge(b)

	// b's cost should override a's.
	cost, _ := merged.Float("cost")
	if cost != 1.0 {
		t.Errorf("merged cost = %v, want 1.0", cost)
	}

	// a's region should be preserved.
	region, _ := merged.String("region")
	if region != "us-east-1" {
		t.Errorf("merged region = %q, want us-east-1", region)
	}

	// b's gpu should be included.
	gpu, _ := merged.Bool("gpu")
	if !gpu {
		t.Error("merged gpu should be true")
	}

	// Original maps should be unchanged.
	origCost, _ := a.Float("cost")
	if origCost != 0.5 {
		t.Error("original a should not be modified")
	}
}

func TestMetaProvider_Interface(t *testing.T) {
	inner := &mockRR{name: "test"}
	wrapped := WithMeta[string, string](inner, Meta{"x": 1})

	// Should implement MetaProvider.
	mp, ok := wrapped.(MetaProvider)
	if !ok {
		t.Fatal("wrapped provider should implement MetaProvider")
	}
	if !mp.Meta().Has("x") {
		t.Error("Meta should contain key 'x'")
	}
}

func TestMetaRR_String(t *testing.T) {
	inner := &mockRR{name: "my-svc"}
	wrapped := WithMeta[string, string](inner, Meta{})

	s := wrapped.(*metaRR[string, string]).String()
	if s != "MetaProvider(my-svc)" {
		t.Errorf("String() = %q, want MetaProvider(my-svc)", s)
	}
}

// mockSink is a simple Sink for testing.
type mockSink struct {
	name string
	sent []string
}

func (m *mockSink) Name() string                       { return m.name }
func (m *mockSink) IsAvailable(_ context.Context) bool { return true }
func (m *mockSink) Send(_ context.Context, input string) error {
	m.sent = append(m.sent, input)
	return nil
}

func TestWithSinkMeta(t *testing.T) {
	inner := &mockSink{name: "test-sink"}
	wrapped := WithSinkMeta[string](inner, Meta{"cost": 0.5})

	// Test delegation.
	if wrapped.Name() != "test-sink" {
		t.Errorf("Name() = %q", wrapped.Name())
	}
	if !wrapped.IsAvailable(context.Background()) {
		t.Error("IsAvailable should return true")
	}
	if err := wrapped.Send(context.Background(), "hello"); err != nil {
		t.Fatalf("Send error: %v", err)
	}
	if len(inner.sent) != 1 || inner.sent[0] != "hello" {
		t.Errorf("Send not delegated, sent = %v", inner.sent)
	}

	// Test meta.
	mp, ok := wrapped.(MetaProvider)
	if !ok {
		t.Fatal("should implement MetaProvider")
	}
	cost, ok := mp.Meta().Float("cost")
	if !ok || cost != 0.5 {
		t.Errorf("cost = %v, %v", cost, ok)
	}
}

// mockStream is a simple Stream for testing.
type mockStream struct {
	name string
}

func (m *mockStream) Name() string                       { return m.name }
func (m *mockStream) IsAvailable(_ context.Context) bool { return true }
func (m *mockStream) Execute(_ context.Context, _ string) (Iterator[string], error) {
	return nil, nil
}

func TestWithStreamMeta(t *testing.T) {
	inner := &mockStream{name: "test-stream"}
	wrapped := WithStreamMeta[string, string](inner, Meta{"latency_ms": 100.0})

	if wrapped.Name() != "test-stream" {
		t.Errorf("Name() = %q", wrapped.Name())
	}
	if !wrapped.IsAvailable(context.Background()) {
		t.Error("IsAvailable should return true")
	}
	_, err := wrapped.Execute(context.Background(), "input")
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	mp, ok := wrapped.(MetaProvider)
	if !ok {
		t.Fatal("should implement MetaProvider")
	}
	lat, ok := mp.Meta().Float("latency_ms")
	if !ok || lat != 100.0 {
		t.Errorf("latency_ms = %v, %v", lat, ok)
	}
}

func TestMeta_Duration_Int64(t *testing.T) {
	m := Meta{"ms": int64(300)}
	d, ok := m.Duration("ms")
	if !ok || d != 300*time.Millisecond {
		t.Errorf("Duration(int64) = %v, %v", d, ok)
	}
}

func floatClose(a, b float64) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < 0.01
}
