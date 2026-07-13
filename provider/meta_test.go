package provider_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/provider"
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
	wrapped := provider.WithMeta[string, string](inner, provider.Meta{
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
	wrapped := provider.WithMeta[string, string](inner, provider.Meta{"cost": 0.1})

	if wrapped.Name() != "my-service" {
		t.Errorf("Name() = %q, want %q", wrapped.Name(), "my-service")
	}
}

func TestWithMeta_IsAvailable(t *testing.T) {
	inner := &mockRR{name: "test"}
	wrapped := provider.WithMeta[string, string](inner, provider.Meta{})

	if !wrapped.IsAvailable(context.Background()) {
		t.Error("IsAvailable() should return true")
	}
}

func TestGetMeta(t *testing.T) {
	inner := &mockRR{name: "test"}
	meta := provider.Meta{"cost": 0.5, "requires": "gpu"}
	wrapped := provider.WithMeta[string, string](inner, meta)

	got := provider.GetMeta[string, string](wrapped)
	if cost, ok := got.Float("cost"); !ok || cost != 0.5 {
		t.Errorf("cost = %v, %v, want 0.5, true", cost, ok)
	}
	if req, ok := got.String("requires"); !ok || req != "gpu" {
		t.Errorf("requires = %q, %v, want %q, true", req, ok, "gpu")
	}
}

func TestGetMeta_NoMeta(t *testing.T) {
	inner := &mockRR{name: "test"}
	got := provider.GetMeta[string, string](inner)
	if len(got) != 0 {
		t.Errorf("expected empty provider.Meta for unwrapped provider, got %v", got)
	}
}

func TestGetMetaFromAny(t *testing.T) {
	inner := &mockRR{name: "test"}
	wrapped := provider.WithMeta[string, string](inner, provider.Meta{"cost": 1.0})

	got := provider.GetMetaFromAny(wrapped)
	if cost, ok := got.Float("cost"); !ok || cost != 1.0 {
		t.Errorf("cost = %v, %v", cost, ok)
	}

	// Non-meta provider.
	got = provider.GetMetaFromAny(inner)
	if len(got) != 0 {
		t.Errorf("expected empty provider.Meta, got %v", got)
	}
}

func TestMeta_Float(t *testing.T) {
	m := provider.Meta{
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
	m := provider.Meta{"region": "us-east-1", "count": 42}

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
	m := provider.Meta{
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
	m := provider.Meta{"enabled": true, "count": 1}

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
	m := provider.Meta{"key": "value"}
	if !m.Has("key") {
		t.Error("Has(key) should be true")
	}
	if m.Has("missing") {
		t.Error("Has(missing) should be false")
	}
}

func TestMeta_Merge(t *testing.T) {
	a := provider.Meta{"cost": 0.5, "region": "us-east-1"}
	b := provider.Meta{"cost": 1.0, "gpu": true}

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
	wrapped := provider.WithMeta[string, string](inner, provider.Meta{"x": 1})

	// Should implement MetaProvider.
	mp, ok := wrapped.(provider.MetaProvider)
	if !ok {
		t.Fatal("wrapped provider should implement MetaProvider")
	}
	if !mp.Meta().Has("x") {
		t.Error("Meta should contain key 'x'")
	}
}

func TestMetaRR_String(t *testing.T) {
	inner := &mockRR{name: "my-svc"}
	wrapped := provider.WithMeta[string, string](inner, provider.Meta{})

	s := fmt.Sprint(wrapped)
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
	wrapped := provider.WithSinkMeta[string](inner, provider.Meta{"cost": 0.5})

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
	mp, ok := wrapped.(provider.MetaProvider)
	if !ok {
		t.Fatal("should implement provider.MetaProvider")
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
func (m *mockStream) Execute(_ context.Context, _ string) (provider.Iterator[string], error) {
	return nil, nil //nolint:nilnil // test mock: no iterator and no error
}

func TestWithStreamMeta(t *testing.T) {
	inner := &mockStream{name: "test-stream"}
	wrapped := provider.WithStreamMeta[string, string](inner, provider.Meta{"latency_ms": 100.0})

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

	mp, ok := wrapped.(provider.MetaProvider)
	if !ok {
		t.Fatal("should implement provider.MetaProvider")
	}
	lat, ok := mp.Meta().Float("latency_ms")
	if !ok || lat != 100.0 {
		t.Errorf("latency_ms = %v, %v", lat, ok)
	}
}

func TestMeta_Duration_Int64(t *testing.T) {
	m := provider.Meta{"ms": int64(300)}
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

func TestMeta_DurationAndBoolBranches(t *testing.T) {
	m := provider.Meta{
		"int64": int64(5),
		"str":   "nope",
		"flag":  "notbool",
	}
	if d, ok := m.Duration("int64"); !ok || d != 5*time.Millisecond {
		t.Fatalf("expected 5ms from int64, got %v ok=%v", d, ok)
	}
	if _, ok := m.Duration("str"); ok {
		t.Fatal("expected non-duration string to fail")
	}
	if _, ok := m.Bool("flag"); ok {
		t.Fatal("expected non-bool value to fail")
	}
}

func TestWithStreamMeta_ExecuteAndMeta(t *testing.T) {
	t.Parallel()
	stream := &streamTestHelper[string, int]{
		name: "meta-stream",
		fn: func(_ context.Context, _ string) (provider.Iterator[int], error) {
			return newSliceIter(1, 2, 3), nil
		},
	}

	wrapped := provider.WithStreamMeta[string, int](stream, provider.Meta{"latency_ms": 50.0, "cost": 0.1})

	if wrapped.Name() != "meta-stream" {
		t.Fatalf("expected meta-stream, got %s", wrapped.Name())
	}
	if !wrapped.IsAvailable(context.Background()) {
		t.Fatal("expected available")
	}

	// Execute should work normally
	iter, err := wrapped.Execute(context.Background(), "input")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	defer iter.Close()

	var results []int
	for {
		v, ok, err := iter.Next(context.Background())
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		if !ok {
			break
		}
		results = append(results, v)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 items, got %d", len(results))
	}

	// Verify meta
	mp, ok := wrapped.(provider.MetaProvider)
	if !ok {
		t.Fatal("expected provider.MetaProvider interface")
	}
	lat, ok := mp.Meta().Float("latency_ms")
	if !ok || lat != 50.0 {
		t.Fatalf("expected latency_ms=50, got %v", lat)
	}
}

func TestWithSinkMeta_SendAndMeta(t *testing.T) {
	t.Parallel()
	var received []string
	sink := provider.NewSinkFunc("meta-sink", func(_ context.Context, s string) error {
		received = append(received, s)
		return nil
	})

	wrapped := provider.WithSinkMeta[string](sink, provider.Meta{"cost": 0.5})

	if err := wrapped.Send(context.Background(), "hello"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(received) != 1 || received[0] != "hello" {
		t.Fatalf("expected [hello], got %v", received)
	}

	mp, ok := wrapped.(provider.MetaProvider)
	if !ok {
		t.Fatal("expected provider.MetaProvider interface")
	}
	cost, ok := mp.Meta().Float("cost")
	if !ok || cost != 0.5 {
		t.Fatalf("expected cost=0.5, got %v", cost)
	}
}

func TestMetaProvider_InterfaceSatisfaction(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		provider any
	}{
		{
			name:     "metaRR",
			provider: provider.WithMeta[string, string](&echoProvider{name: "rr"}, provider.Meta{"x": 1}),
		},
		{
			name: "metaSink",
			provider: provider.WithSinkMeta[string](
				provider.NewSinkFunc("sink", func(_ context.Context, _ string) error { return nil }),
				provider.Meta{"y": 2},
			),
		},
		{
			name: "metaStream",
			provider: provider.WithStreamMeta[string, int](
				&streamTestHelper[string, int]{
					name: "stream",
					fn: func(_ context.Context, _ string) (provider.Iterator[int], error) {
						return newSliceIter[int](), nil
					},
				},
				provider.Meta{"z": 3},
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mp, ok := tt.provider.(provider.MetaProvider)
			if !ok {
				t.Fatalf("%s should implement provider.MetaProvider", tt.name)
			}
			if len(mp.Meta()) == 0 {
				t.Fatal("expected non-empty meta")
			}
		})
	}
}

func TestMeta_PropagationThroughMiddlewareChain(t *testing.T) {
	t.Parallel()
	inner := &echoProvider{name: "meta-chain"}
	meta := provider.Meta{"cost": 0.5, "tier": "premium"}
	wrapped := provider.WithMeta[string, string](inner, meta)

	// Wrap with middleware chain
	log := logging.NewDefault("test")
	chained := provider.Chain(
		provider.WithLogging[string, string](log),
	)(wrapped)

	// Execute should still work
	result, err := chained.Execute(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "echo:test" {
		t.Fatalf("expected echo:test, got %q", result)
	}

	// provider.Meta should be retrievable from any provider (check via provider.GetMetaFromAny)
	got := provider.GetMetaFromAny(wrapped)
	cost, ok := got.Float("cost")
	if !ok || cost != 0.5 {
		t.Fatalf("expected cost=0.5 from wrapped, got %v", cost)
	}
}
