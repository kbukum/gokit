package provider_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/kbukum/gokit/provider"
)

func TestLargePayload_FanOutSink(t *testing.T) {
	t.Parallel()
	largeInput := strings.Repeat("x", 1<<16)

	var mu sync.Mutex
	var received []string

	sink1 := provider.NewSinkFunc("s1", func(_ context.Context, s string) error {
		mu.Lock()
		received = append(received, s)
		mu.Unlock()
		return nil
	})
	sink2 := provider.NewSinkFunc("s2", func(_ context.Context, s string) error {
		mu.Lock()
		received = append(received, s)
		mu.Unlock()
		return nil
	})

	fan := provider.FanOutSink("fan", sink1, sink2)
	err := fan.Send(context.Background(), largeInput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(received))
	}
	for _, r := range received {
		if len(r) != len(largeInput) {
			t.Fatalf("payload corrupted: expected len %d, got len %d", len(largeInput), len(r))
		}
	}
}
