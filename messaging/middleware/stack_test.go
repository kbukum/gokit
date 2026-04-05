package middleware_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/kbukum/gokit/messaging"
	"github.com/kbukum/gokit/messaging/middleware"
)

func TestStackBuildNoMiddleware(t *testing.T) {
	var called atomic.Bool
	base := func(ctx context.Context, msg messaging.Message) error {
		called.Store(true)
		return nil
	}

	handler := middleware.NewStack(base).Build()
	if err := handler(context.Background(), messaging.Message{}); err != nil {
		t.Fatal(err)
	}
	if !called.Load() {
		t.Error("base handler not called")
	}
}

func TestStackBuildWithMetrics(t *testing.T) {
	var called atomic.Bool
	base := func(ctx context.Context, msg messaging.Message) error {
		called.Store(true)
		return nil
	}

	handler := middleware.NewStack(base).
		WithMetrics("test-topic", "test-group").
		Build()

	if err := handler(context.Background(), messaging.Message{}); err != nil {
		t.Fatal(err)
	}
	if !called.Load() {
		t.Error("base handler not called through metrics middleware")
	}
}

func TestStackBuildMultiple(t *testing.T) {
	var called atomic.Bool
	base := func(ctx context.Context, msg messaging.Message) error {
		called.Store(true)
		return nil
	}

	// Build with metrics + dedup (no retry, no tracing, no CB — just test composition)
	handler := middleware.NewStack(base).
		WithMetrics("topic", "group").
		WithDedup(middleware.DedupConfig{}).
		Build()

	if err := handler(context.Background(), messaging.Message{Key: "k1"}); err != nil {
		t.Fatal(err)
	}
	if !called.Load() {
		t.Error("base handler not called through stacked middleware")
	}
}
