package provider_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kbukum/gokit/provider"
)

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
func TestContextCancellation_Sink(t *testing.T) {
	t.Parallel()
	sink := provider.NewSinkFunc("blocking-sink", func(ctx context.Context, _ string) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
			return nil
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := sink.Send(ctx, "test")
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}
}
