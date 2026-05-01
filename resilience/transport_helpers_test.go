package resilience

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAwait_ReturnsResult(t *testing.T) {
	t.Parallel()

	got, err := Await(context.Background(), time.Second, func(context.Context) (string, error) {
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("Await() error = %v", err)
	}
	if got != "ok" {
		t.Fatalf("Await() = %q, want ok", got)
	}
}

func TestAwait_TimesOut(t *testing.T) {
	t.Parallel()

	_, err := Await(context.Background(), 10*time.Millisecond, func(ctx context.Context) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Await() error = %v, want context deadline exceeded", err)
	}
}

func TestAwait_PropagatesCallerCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Await(ctx, time.Second, func(callCtx context.Context) (string, error) {
		<-callCtx.Done()
		return "", callCtx.Err()
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Await() error = %v, want context canceled", err)
	}
}

func TestPolicy_WithTimeoutIfUnset(t *testing.T) {
	t.Parallel()

	policy := NewPolicy().WithTimeoutIfUnset(20 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := Execute(ctx, policy, func(callCtx context.Context) (struct{}, error) {
		deadline, ok := callCtx.Deadline()
		if !ok {
			t.Fatal("expected inherited deadline")
		}
		if time.Until(deadline) < 500*time.Millisecond {
			t.Fatalf("timeout override occurred: remaining=%v", time.Until(deadline))
		}
		return struct{}{}, nil
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestKeyedRateLimiter_Allow(t *testing.T) {
	t.Parallel()

	rl := NewKeyedRateLimiter(KeyedRateLimiterConfig{})
	t.Cleanup(rl.Stop)
	now := time.Now()
	rl.nowFunc = func() time.Time { return now }

	first := rl.Allow("tenant-a", 2, time.Minute)
	second := rl.Allow("tenant-a", 2, time.Minute)
	third := rl.Allow("tenant-a", 2, time.Minute)

	if !first.Allowed || !second.Allowed {
		t.Fatal("expected initial requests to pass")
	}
	if third.Allowed {
		t.Fatal("expected third request to be limited")
	}
	if third.RetryAfter <= 0 {
		t.Fatalf("expected retry-after > 0, got %v", third.RetryAfter)
	}
}

func TestKeyedRateLimiter_CleansUpWhileIdle(t *testing.T) {
	t.Parallel()

	rl := NewKeyedRateLimiter(KeyedRateLimiterConfig{
		CleanupInterval: 10 * time.Millisecond,
		BucketTTL:       10 * time.Millisecond,
	})
	t.Cleanup(rl.Stop)

	rl.Allow("tenant-a", 1, time.Minute)

	deadline := time.Now().Add(250 * time.Millisecond)
	for time.Now().Before(deadline) {
		rl.mu.Lock()
		bucketCount := len(rl.buckets)
		rl.mu.Unlock()
		if bucketCount == 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}

	t.Fatal("expected idle janitor to remove stale buckets")
}
