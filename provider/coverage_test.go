package provider_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/kbukum/gokit/provider"
	"github.com/kbukum/gokit/resilience"
)

func TestChainStream_AppliesInOrder(t *testing.T) {
	var order []string
	mw := func(tag string) provider.StreamMiddleware[string, byte] {
		return func(inner provider.Stream[string, byte]) provider.Stream[string, byte] {
			order = append(order, tag)
			return inner
		}
	}
	chained := provider.ChainStream(mw("a"), mw("b"), mw("c"))
	result := chained(&splitProvider{})
	if result == nil {
		t.Fatal("expected non-nil chained stream")
	}
	// Innermost (c) is constructed first, outermost (a) last.
	if len(order) != 3 || order[0] != "c" || order[2] != "a" {
		t.Fatalf("unexpected middleware order: %v", order)
	}
}

func TestChainDuplex_AppliesInOrder(t *testing.T) {
	var order []string
	mw := func(tag string) provider.DuplexMiddleware[string, string] {
		return func(inner provider.Duplex[string, string]) provider.Duplex[string, string] {
			order = append(order, tag)
			return inner
		}
	}
	chained := provider.ChainDuplex(mw("a"), mw("b"), mw("c"))
	result := chained(&echoDuplex{})
	if result == nil {
		t.Fatal("expected non-nil chained duplex")
	}
	if len(order) != 3 || order[0] != "c" || order[2] != "a" {
		t.Fatalf("unexpected middleware order: %v", order)
	}
}

func TestManager_WithLoggerAndDefault(t *testing.T) {
	registry := provider.NewRegistry[provider.RequestResponse[string, string]]()
	selector := &provider.HealthCheckSelector[provider.RequestResponse[string, string]]{}
	mgr := provider.NewManager(registry, selector,
		provider.WithLogger[provider.RequestResponse[string, string]](slog.Default()),
	)

	registry.RegisterFactory("echo", func(_ map[string]any) (provider.RequestResponse[string, string], error) {
		return &echoProvider{name: "echo"}, nil
	})
	if err := mgr.InitializeWithContext(context.Background(), "echo", nil); err != nil {
		t.Fatalf("initialize error: %v", err)
	}

	mgr.SetDefault("echo")
	p, err := mgr.Get(context.Background())
	if err != nil {
		t.Fatalf("get error: %v", err)
	}
	if p.Name() != "echo" {
		t.Fatalf("expected echo provider, got %q", p.Name())
	}
}

func TestWithStreamResilience_NameAndIsAvailable(t *testing.T) {
	cfg := provider.ResilienceConfig{
		RateLimiter: &resilience.RateLimiterConfig{Name: "s", Rate: 100, Burst: 10},
	}
	wrapped := provider.WithStreamResilience[string, byte](&splitProvider{}, cfg)
	if wrapped.Name() != "split" {
		t.Fatalf("expected name split, got %q", wrapped.Name())
	}
	if !wrapped.IsAvailable(context.Background()) {
		t.Fatal("expected wrapped stream to be available")
	}
}

func TestAdaptSink_IsAvailableDelegates(t *testing.T) {
	sink := provider.AdaptSink[int, string](&collectSink{}, "adapted",
		func(_ context.Context, in int) (string, error) {
			return string(rune('0' + in)), nil
		},
	)
	if !sink.IsAvailable(context.Background()) {
		t.Fatal("expected adapted sink to be available")
	}
}

func TestTapSink_IsAvailableDelegates(t *testing.T) {
	var tapped []string
	sink := provider.TapSink[string](&collectSink{}, func(_ context.Context, in string) {
		tapped = append(tapped, in)
	})
	if !sink.IsAvailable(context.Background()) {
		t.Fatal("expected tap sink to be available")
	}
	if err := sink.Send(context.Background(), "x"); err != nil {
		t.Fatalf("send error: %v", err)
	}
	if len(tapped) != 1 || tapped[0] != "x" {
		t.Fatalf("expected tap to observe x, got %v", tapped)
	}
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
