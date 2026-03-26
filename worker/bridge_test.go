package worker_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/kbukum/gokit/provider"
	"github.com/kbukum/gokit/worker"
)

// mockProvider implements provider.RequestResponse for testing.
type mockProvider struct {
	name   string
	result string
}

func (m *mockProvider) Name() string                       { return m.name }
func (m *mockProvider) IsAvailable(_ context.Context) bool { return true }
func (m *mockProvider) Execute(_ context.Context, input string) (string, error) {
	return m.result + "-" + input, nil
}

// compile-time check
var _ provider.RequestResponse[string, string] = (*mockProvider)(nil)

func TestFromProvider(t *testing.T) {
	t.Parallel()

	p := &mockProvider{name: "test", result: "ok"}
	h := worker.FromProvider[string, string](p)

	var events []worker.Event[string]
	emit := func(e worker.Event[string]) { events = append(events, e) }

	err := h.Handle(context.Background(), "hello", emit)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != worker.EventResult {
		t.Fatalf("expected EventResult, got %v", events[0].Type)
	}
	if events[0].Data != "ok-hello" {
		t.Fatalf("expected 'ok-hello', got %q", events[0].Data)
	}
}

func TestAsProvider(t *testing.T) {
	t.Parallel()

	h := worker.HandlerFunc[string, string](func(
		ctx context.Context, task string, emit func(worker.Event[string]),
	) error {
		emit(worker.PartialEvent("partial"))
		return nil
	})

	p := worker.AsProvider(h, worker.AsProviderConfig{ProviderName: "test-handler"})

	if p.Name() != "test-handler" {
		t.Fatalf("expected name 'test-handler', got %q", p.Name())
	}

	if !p.IsAvailable(context.Background()) {
		t.Fatal("expected provider to be available")
	}

	result, err := p.Execute(context.Background(), "input")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Handler emits PartialEvent but no EventResult, so result is zero value
	if result != "" {
		t.Fatalf("expected empty result, got %q", result)
	}
}

func TestAsProviderWithResult(t *testing.T) {
	t.Parallel()

	h := worker.HandlerFunc[string, string](func(
		ctx context.Context, task string, emit func(worker.Event[string]),
	) error {
		// Simulate handler that produces result via emit
		emit(worker.Event[string]{Type: worker.EventResult, Data: "computed-" + task})
		return nil
	})

	p := worker.AsProvider(h, worker.AsProviderConfig{ProviderName: "result-handler"})
	result, err := p.Execute(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "computed-test" {
		t.Fatalf("expected 'computed-test', got %q", result)
	}
}

func TestFromProviderUnavailable(t *testing.T) {
	t.Parallel()

	p := &unavailableProvider{name: "offline"}
	h := worker.FromProvider[string, string](p)

	err := h.Handle(context.Background(), "hello", func(worker.Event[string]) {})
	if err == nil {
		t.Fatal("expected error from unavailable provider")
	}
}

type unavailableProvider struct {
	name string
}

func (m *unavailableProvider) Name() string                       { return m.name }
func (m *unavailableProvider) IsAvailable(_ context.Context) bool { return false }
func (m *unavailableProvider) Execute(_ context.Context, _ string) (string, error) {
	return "", nil
}

func TestAsProviderHandlerError(t *testing.T) {
	t.Parallel()

	h := worker.HandlerFunc[string, string](func(
		ctx context.Context, task string, emit func(worker.Event[string]),
	) error {
		return fmt.Errorf("handler failed")
	})

	p := worker.AsProvider(h, worker.AsProviderConfig{ProviderName: "error-handler"})
	_, err := p.Execute(context.Background(), "input")
	if err == nil {
		t.Fatal("expected error from handler")
	}
}
