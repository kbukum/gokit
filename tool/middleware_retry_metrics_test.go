package tool_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kbukum/gokit/tool"
)

// --- WithRetry tests ---

func TestWithRetry_SuccessOnFirstTry(t *testing.T) {
	fn := tool.FromFunc("always_ok", "Always succeeds",
		func(ctx context.Context, in struct{}) (string, error) {
			return "ok", nil
		})
	callable := fn.AsCallable()

	retried := tool.Apply(callable, tool.WithRetry(tool.RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   1 * time.Millisecond,
	}))

	ctx := tool.Background()
	result, err := retried.Call(ctx, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result == nil {
		t.Fatal("expected result")
	}
}

func TestWithRetry_SuccessAfterRetries(t *testing.T) {
	callCount := 0
	fn := tool.FromFunc("retry_me", "Fails twice then succeeds",
		func(ctx context.Context, in struct{}) (string, error) {
			callCount++
			if callCount < 3 {
				return "", fmt.Errorf("attempt %d failed", callCount)
			}
			return "success", nil
		})
	callable := fn.AsCallable()

	retried := tool.Apply(callable, tool.WithRetry(tool.RetryConfig{
		MaxAttempts: 5,
		BaseDelay:   1 * time.Millisecond,
		MaxDelay:    5 * time.Millisecond,
	}))

	ctx := tool.Background()
	result, err := retried.Call(ctx, nil)
	if err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	if result == nil {
		t.Fatal("expected result")
	}
	if callCount != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
}

func TestWithRetry_AllAttemptsFail(t *testing.T) {
	fn := tool.FromFunc("always_fail", "Always fails",
		func(ctx context.Context, in struct{}) (string, error) {
			return "", fmt.Errorf("permanent error")
		})
	callable := fn.AsCallable()

	retried := tool.Apply(callable, tool.WithRetry(tool.RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   1 * time.Millisecond,
	}))

	ctx := tool.Background()
	_, err := retried.Call(ctx, nil)
	if err == nil {
		t.Fatal("expected error after all attempts")
	}
}

func TestWithRetry_ShouldRetryFilter(t *testing.T) {
	callCount := 0
	fn := tool.FromFunc("selective_retry", "Only retries certain errors",
		func(ctx context.Context, in struct{}) (string, error) {
			callCount++
			return "", fmt.Errorf("non-retryable error")
		})
	callable := fn.AsCallable()

	retried := tool.Apply(callable, tool.WithRetry(tool.RetryConfig{
		MaxAttempts: 5,
		BaseDelay:   1 * time.Millisecond,
		ShouldRetry: func(err error) bool {
			return false // never retry
		},
	}))

	ctx := tool.Background()
	_, err := retried.Call(ctx, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if callCount != 1 {
		t.Errorf("expected exactly 1 call (no retry), got %d", callCount)
	}
}

func TestWithRetry_DefaultConfig(t *testing.T) {
	cfg := tool.RetryConfig{}
	// WithRetry applies defaults internally
	fn := tool.FromFunc("test", "test",
		func(ctx context.Context, in struct{}) (string, error) {
			return "ok", nil
		})
	callable := fn.AsCallable()

	retried := tool.Apply(callable, tool.WithRetry(cfg))

	ctx := tool.Background()
	result, err := retried.Call(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result")
	}
}

// --- WithMetrics tests ---

func TestWithMetrics_RecordsCalls(t *testing.T) {
	metrics := &tool.InMemoryMetrics{}

	fn := tool.FromFunc("counter_tool", "Counts calls",
		func(ctx context.Context, in struct{}) (string, error) {
			return "ok", nil
		})
	callable := fn.AsCallable()

	metered := tool.Apply(callable, tool.WithMetrics(metrics))

	ctx := tool.Background()
	for range 3 {
		_, err := metered.Call(ctx, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	if metrics.CallCount("counter_tool") != 3 {
		t.Errorf("expected 3 calls, got %d", metrics.CallCount("counter_tool"))
	}
	if metrics.ErrorCount("counter_tool") != 0 {
		t.Errorf("expected 0 errors, got %d", metrics.ErrorCount("counter_tool"))
	}
}

func TestWithMetrics_RecordsErrors(t *testing.T) {
	metrics := &tool.InMemoryMetrics{}

	fn := tool.FromFunc("error_tool", "Always errors",
		func(ctx context.Context, in struct{}) (string, error) {
			return "", fmt.Errorf("oops")
		})
	callable := fn.AsCallable()

	metered := tool.Apply(callable, tool.WithMetrics(metrics))

	ctx := tool.Background()
	_, _ = metered.Call(ctx, nil)

	if metrics.CallCount("error_tool") != 1 {
		t.Errorf("expected 1 call, got %d", metrics.CallCount("error_tool"))
	}
	if metrics.ErrorCount("error_tool") != 1 {
		t.Errorf("expected 1 error, got %d", metrics.ErrorCount("error_tool"))
	}
}

func TestWithMetrics_RecordsDuration(t *testing.T) {
	metrics := &tool.InMemoryMetrics{}

	fn := tool.FromFunc("slow_tool", "Takes time",
		func(ctx context.Context, in struct{}) (string, error) {
			time.Sleep(10 * time.Millisecond)
			return "done", nil
		})
	callable := fn.AsCallable()

	metered := tool.Apply(callable, tool.WithMetrics(metrics))

	ctx := tool.Background()
	_, err := metered.Call(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries := metrics.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Duration < 10*time.Millisecond {
		t.Errorf("expected duration >= 10ms, got %v", entries[0].Duration)
	}
}

func TestWithMetrics_MultipleTools(t *testing.T) {
	metrics := &tool.InMemoryMetrics{}

	fn1 := tool.FromFunc("tool_a", "A",
		func(ctx context.Context, in struct{}) (string, error) {
			return "a", nil
		})
	fn2 := tool.FromFunc("tool_b", "B",
		func(ctx context.Context, in struct{}) (string, error) {
			return "b", nil
		})

	metered1 := tool.Apply(fn1.AsCallable(), tool.WithMetrics(metrics))
	metered2 := tool.Apply(fn2.AsCallable(), tool.WithMetrics(metrics))

	ctx := tool.Background()
	_, _ = metered1.Call(ctx, nil)
	_, _ = metered1.Call(ctx, nil)
	_, _ = metered2.Call(ctx, nil)

	if metrics.CallCount("tool_a") != 2 {
		t.Errorf("tool_a: expected 2 calls, got %d", metrics.CallCount("tool_a"))
	}
	if metrics.CallCount("tool_b") != 1 {
		t.Errorf("tool_b: expected 1 call, got %d", metrics.CallCount("tool_b"))
	}
}

// --- Middleware chain tests ---

func TestChain_RetryWithMetrics(t *testing.T) {
	metrics := &tool.InMemoryMetrics{}
	callCount := 0

	fn := tool.FromFunc("chained_tool", "Retried and metered",
		func(ctx context.Context, in struct{}) (string, error) {
			callCount++
			if callCount < 2 {
				return "", fmt.Errorf("fail once")
			}
			return "ok", nil
		})
	callable := fn.AsCallable()

	// Metrics wraps retry, so metrics sees the final result (1 call from metrics POV)
	chained := tool.Apply(callable,
		tool.WithMetrics(metrics),
		tool.WithRetry(tool.RetryConfig{
			MaxAttempts: 3,
			BaseDelay:   1 * time.Millisecond,
		}),
	)

	ctx := tool.Background()
	result, err := chained.Call(ctx, nil)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if result == nil {
		t.Fatal("expected result")
	}
	if callCount != 2 {
		t.Errorf("expected 2 actual calls, got %d", callCount)
	}
	// Metrics sees 1 call (retry is inner, metrics is outer)
	if metrics.CallCount("chained_tool") != 1 {
		t.Errorf("expected 1 metric call, got %d", metrics.CallCount("chained_tool"))
	}
}
