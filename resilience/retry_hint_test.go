package resilience

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRetryHonorsMinimumBeyondBackoffCap(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := DefaultRetryConfig()
	cfg.MaxBackoff = time.Millisecond
	cfg.MinimumDelay = func(error) (time.Duration, error) { return time.Hour, nil }
	var observed time.Duration
	cfg.OnRetry = func(_ int, _ error, delay time.Duration) {
		observed = delay
		cancel()
	}
	_, err := Retry(ctx, cfg, func() (int, error) { return 0, errors.New("transient") })
	if observed != time.Hour || !errors.Is(err, context.Canceled) {
		t.Fatalf("delay=%s, error=%v", observed, err)
	}
}

func TestRetryHintOutsideBudgetDoesNotWait(t *testing.T) {
	t.Parallel()
	cfg := DefaultRetryConfig()
	cfg.MaxElapsedTime = time.Second
	cfg.MinimumDelay = func(error) (time.Duration, error) { return time.Hour, nil }
	calls := 0
	_, err := Retry(context.Background(), cfg, func() (int, error) {
		calls++
		return 0, errors.New("transient")
	})
	if calls != 1 || !errors.Is(err, ErrMaxRetriesExceeded) {
		t.Fatalf("calls=%d, error=%v", calls, err)
	}
}

func TestRetryHintFailureIsSurfaced(t *testing.T) {
	t.Parallel()
	want := errors.New("invalid remote retry delay")
	cfg := DefaultRetryConfig()
	cfg.MinimumDelay = func(error) (time.Duration, error) { return 0, want }
	_, err := Retry(context.Background(), cfg, func() (int, error) { return 0, errors.New("transient") })
	if !errors.Is(err, want) {
		t.Fatalf("lost hint error: %v", err)
	}
}

func TestPerCallRetrySelectionPreservesBreakerState(t *testing.T) {
	t.Parallel()
	policy := NewPolicy().WithCircuitBreaker(CircuitBreakerConfig{
		Name: "shared", MaxFailures: 1, Timeout: time.Hour,
	})
	calls := 0
	invoke := func(context.Context) (int, error) {
		calls++
		return 0, errors.New("failed")
	}
	if _, err := ExecuteWithRetry(context.Background(), policy, nil, invoke); err == nil {
		t.Fatal("expected first failure")
	}
	if _, err := ExecuteWithRetry(context.Background(), policy, nil, invoke); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("breaker state reset across calls: %v", err)
	}
	if calls != 1 {
		t.Fatalf("open breaker admitted %d calls", calls)
	}
}
