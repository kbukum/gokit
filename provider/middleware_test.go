package provider_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kbukum/gokit/logger"
	"github.com/kbukum/gokit/observability"
	"github.com/kbukum/gokit/provider"
)

// --- Chain tests ---

func TestChain_Empty(t *testing.T) {
	p := &echoProvider{name: "test"}
	chain := provider.Chain[string, string]()
	wrapped := chain(p)
	if wrapped.Name() != "test" {
		t.Fatalf("expected 'test', got %q", wrapped.Name())
	}
	result, err := wrapped.Execute(context.Background(), "hello")
	if err != nil || result != "echo:hello" {
		t.Fatalf("expected echo:hello, got %q, err %v", result, err)
	}
}

func TestChain_SingleMiddleware(t *testing.T) {
	p := &echoProvider{name: "test"}
	log := logger.NewDefault("test")
	wrapped := provider.Chain(
		provider.WithLogging[string, string](log),
	)(p)

	result, err := wrapped.Execute(context.Background(), "hello")
	if err != nil || result != "echo:hello" {
		t.Fatalf("expected echo:hello, got %q, err %v", result, err)
	}
}

func TestChain_Order(t *testing.T) {
	// Verify middlewares execute in order: first is outermost
	var order []string

	mw := func(tag string) provider.Middleware[string, string] {
		return func(inner provider.RequestResponse[string, string]) provider.RequestResponse[string, string] {
			return &orderTracker[string, string]{inner: inner, tag: tag, order: &order}
		}
	}

	p := &echoProvider{name: "test"}
	wrapped := provider.Chain(mw("A"), mw("B"), mw("C"))(p)

	_, err := wrapped.Execute(context.Background(), "x")
	if err != nil {
		t.Fatal(err)
	}

	// A is outermost, so A enters first, then B, then C
	if len(order) != 6 {
		t.Fatalf("expected 6 entries, got %v", order)
	}
	if order[0] != "A:before" || order[1] != "B:before" || order[2] != "C:before" {
		t.Errorf("expected [A:before B:before C:before ...], got %v", order[:3])
	}
	if order[3] != "C:after" || order[4] != "B:after" || order[5] != "A:after" {
		t.Errorf("expected [... C:after B:after A:after], got %v", order[3:])
	}
}

type orderTracker[I, O any] struct {
	inner provider.RequestResponse[I, O]
	tag   string
	order *[]string
}

func (o *orderTracker[I, O]) Name() string                         { return o.inner.Name() }
func (o *orderTracker[I, O]) IsAvailable(ctx context.Context) bool { return o.inner.IsAvailable(ctx) }
func (o *orderTracker[I, O]) Execute(ctx context.Context, input I) (O, error) {
	*o.order = append(*o.order, o.tag+":before")
	result, err := o.inner.Execute(ctx, input)
	*o.order = append(*o.order, o.tag+":after")
	return result, err
}

// --- WithLogging tests ---

func TestWithLogging_Success(t *testing.T) {
	p := &echoProvider{name: "log-test"}
	log := logger.NewDefault("test")
	wrapped := provider.WithLogging[string, string](log)(p)

	result, err := wrapped.Execute(context.Background(), "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "echo:hello" {
		t.Fatalf("expected echo:hello, got %q", result)
	}
	if wrapped.Name() != "log-test" {
		t.Fatalf("expected name 'log-test', got %q", wrapped.Name())
	}
}

func TestWithLogging_Error(t *testing.T) {
	p := &failingChatProvider2{}
	log := logger.NewDefault("test")
	wrapped := provider.WithLogging[string, string](log)(p)

	_, err := wrapped.Execute(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error")
	}
}

type failingChatProvider2 struct{}

func (p *failingChatProvider2) Name() string                       { return "fail" }
func (p *failingChatProvider2) IsAvailable(_ context.Context) bool { return true }
func (p *failingChatProvider2) Execute(_ context.Context, _ string) (string, error) {
	return "", errors.New("intentional failure")
}

// --- WithTracing tests ---

func TestWithTracing_Success(t *testing.T) {
	p := &echoProvider{name: "trace-test"}
	wrapped := provider.WithTracing[string, string]("my-service")(p)

	result, err := wrapped.Execute(context.Background(), "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "echo:hello" {
		t.Fatalf("expected echo:hello, got %q", result)
	}
	if wrapped.Name() != "trace-test" {
		t.Fatalf("expected name 'trace-test', got %q", wrapped.Name())
	}
}

func TestWithTracing_Error(t *testing.T) {
	p := &failingChatProvider2{}
	wrapped := provider.WithTracing[string, string]("my-service")(p)

	_, err := wrapped.Execute(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- WithLogging delegation tests ---

func TestWithLogging_DelegatesIsAvailable(t *testing.T) {
	p := &echoProvider{name: "avail-test"}
	log := logger.NewDefault("test")
	wrapped := provider.WithLogging[string, string](log)(p)

	if !wrapped.IsAvailable(context.Background()) {
		t.Fatal("expected IsAvailable to delegate to inner provider")
	}
}

// --- WithMetrics tests ---

func TestWithMetrics_Success(t *testing.T) {
	p := &echoProvider{name: "metrics-test"}
	meter := observability.Meter("test")
	metrics, err := observability.NewMetrics(meter)
	if err != nil {
		t.Fatalf("failed to create metrics: %v", err)
	}

	wrapped := provider.WithMetrics[string, string](metrics)(p)

	result, err := wrapped.Execute(context.Background(), "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "echo:hello" {
		t.Fatalf("expected echo:hello, got %q", result)
	}
	if wrapped.Name() != "metrics-test" {
		t.Fatalf("expected name 'metrics-test', got %q", wrapped.Name())
	}
}

func TestWithMetrics_Error(t *testing.T) {
	p := &failingChatProvider2{}
	meter := observability.Meter("test")
	metrics, err := observability.NewMetrics(meter)
	if err != nil {
		t.Fatalf("failed to create metrics: %v", err)
	}

	wrapped := provider.WithMetrics[string, string](metrics)(p)

	_, err = wrapped.Execute(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestWithMetrics_DelegatesIsAvailable(t *testing.T) {
	p := &echoProvider{name: "avail-test"}
	meter := observability.Meter("test")
	metrics, err := observability.NewMetrics(meter)
	if err != nil {
		t.Fatalf("failed to create metrics: %v", err)
	}

	wrapped := provider.WithMetrics[string, string](metrics)(p)

	if !wrapped.IsAvailable(context.Background()) {
		t.Fatal("expected IsAvailable to delegate to inner provider")
	}
}

// --- WithTracing delegation tests ---

func TestWithTracing_DelegatesIsAvailable(t *testing.T) {
	p := &echoProvider{name: "avail-test"}
	wrapped := provider.WithTracing[string, string]("svc")(p)

	if !wrapped.IsAvailable(context.Background()) {
		t.Fatal("expected IsAvailable to delegate to inner provider")
	}
}

// --- Composition test: Chain + Resilience + Stateful ---

func TestChain_WithResilienceAndLogging(t *testing.T) {
	p := &echoProvider{name: "composed"}
	log := logger.NewDefault("test")

	wrapped := provider.Chain(
		provider.WithLogging[string, string](log),
		provider.WithTracing[string, string]("test-svc"),
	)(p)

	// Further wrap with resilience (which is also a RequestResponse)
	resilient := provider.WithResilience(wrapped, provider.ResilienceConfig{})

	result, err := resilient.Execute(context.Background(), "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "echo:hello" {
		t.Fatalf("expected echo:hello, got %q", result)
	}
}

// --- Full composition: Chain + Metrics + Logging + Tracing ---

func TestChain_AllMiddlewares(t *testing.T) {
	p := &echoProvider{name: "full-stack"}
	log := logger.NewDefault("test")
	meter := observability.Meter("test")
	metrics, err := observability.NewMetrics(meter)
	if err != nil {
		t.Fatalf("failed to create metrics: %v", err)
	}

	wrapped := provider.Chain(
		provider.WithLogging[string, string](log),
		provider.WithMetrics[string, string](metrics),
		provider.WithTracing[string, string]("test-svc"),
	)(p)

	result, err := wrapped.Execute(context.Background(), "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "echo:hello" {
		t.Fatalf("expected echo:hello, got %q", result)
	}
}
