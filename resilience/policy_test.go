package resilience

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"testing/quick"
	"time"
)

func TestExecutePolicy_OrderAndRetry(t *testing.T) {
	t.Parallel()

	var orderMu sync.Mutex
	order := make([]string, 0, 8)
	appendOrder := func(label string) {
		orderMu.Lock()
		defer orderMu.Unlock()
		order = append(order, label)
	}

	attempts := atomic.Int32{}
	policy := NewPolicy().
		WithRateLimiter(RateLimiterConfig{Name: "rl", Rate: 1000, Burst: 1}).
		WithBulkhead(BulkheadConfig{Name: "bh", MaxConcurrent: 1, MaxWait: time.Second, OnAcquire: func(string) { appendOrder("bulkhead") }}).
		WithCircuitBreaker(CircuitBreakerConfig{Name: "cb", MaxFailures: 5, Timeout: time.Second}).
		WithTimeout(200 * time.Millisecond).
		WithRetry(RetryConfig{MaxAttempts: 2, InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond, Strategy: ConstantBackoff, OnRetry: func(attempt int, err error, backoff time.Duration) { appendOrder("retry") }})

	result, err := Execute(context.Background(), policy, func(ctx context.Context) (string, error) {
		if attempts.Add(1) == 1 {
			appendOrder("fn")
			return "", errors.New("transient")
		}
		appendOrder("fn")
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ok" {
		t.Fatalf("result = %q, want ok", result)
	}

	want := []string{"bulkhead", "fn", "retry", "fn"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
}

func TestExecutePolicy_TimeoutWrapsRetryBudget(t *testing.T) {
	t.Parallel()

	attempts := atomic.Int32{}
	policy := NewPolicy().
		WithTimeout(40 * time.Millisecond).
		WithRetry(RetryConfig{MaxAttempts: 5, InitialBackoff: 20 * time.Millisecond, MaxBackoff: 20 * time.Millisecond, Strategy: ConstantBackoff})

	_, err := Execute(context.Background(), policy, func(ctx context.Context) (string, error) {
		attempts.Add(1)
		<-ctx.Done()
		return "", ctx.Err()
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
	if attempts.Load() != 1 {
		t.Fatalf("timeout should bound retries, attempts = %d", attempts.Load())
	}
}

func TestExecutePolicy_ReusesCircuitBreakerAcrossCalls(t *testing.T) {
	t.Parallel()

	policy := NewPolicy().WithCircuitBreaker(CircuitBreakerConfig{
		Name:             "cb",
		MaxFailures:      1,
		Timeout:          time.Hour,
		HalfOpenMaxCalls: 1,
	})

	_, err := Execute(context.Background(), policy, func(ctx context.Context) (string, error) {
		return "", errors.New("boom")
	})
	if err == nil {
		t.Fatal("expected first call to fail")
	}

	_, err = Execute(context.Background(), policy, func(ctx context.Context) (string, error) {
		return "ok", nil
	})
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected circuit open, got %v", err)
	}
}

func TestCalculateBackoff_Strategies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		strategy BackoffStrategy
		want     []time.Duration
	}{
		{name: "exponential", strategy: ExponentialBackoff, want: []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond}},
		{name: "constant", strategy: ConstantBackoff, want: []time.Duration{100 * time.Millisecond, 100 * time.Millisecond, 100 * time.Millisecond}},
		{name: "linear", strategy: LinearBackoff, want: []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 300 * time.Millisecond}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := RetryConfig{InitialBackoff: 100 * time.Millisecond, MaxBackoff: time.Second, Strategy: tt.strategy, BackoffFactor: 2, Jitter: 0}
			for attempt, want := range tt.want {
				got := calculateBackoff(attempt+1, cfg)
				if got != want {
					t.Fatalf("attempt %d: got %v, want %v", attempt+1, got, want)
				}
			}
		})
	}
}

func TestRetryBackoffBoundaries_Property(t *testing.T) {
	t.Parallel()

	strategies := []BackoffStrategy{ExponentialBackoff, ConstantBackoff, LinearBackoff}
	for _, strategy := range strategies {
		t.Run(strategyName(strategy), func(t *testing.T) {
			t.Parallel()
			property := func(a uint8, b uint8) bool {
				cfg := RetryConfig{InitialBackoff: 25 * time.Millisecond, MaxBackoff: 250 * time.Millisecond, Strategy: strategy, BackoffFactor: 2, Jitter: 0}
				attemptA := int(a%20) + 1
				attemptB := attemptA + int(b%10)
				backoffA := calculateBackoff(attemptA, cfg)
				backoffB := calculateBackoff(attemptB, cfg)
				return backoffA > 0 && backoffA <= cfg.MaxBackoff && backoffB > 0 && backoffB <= cfg.MaxBackoff && backoffB >= backoffA
			}
			if err := quick.Check(property, nil); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRateLimiterBurst_Property(t *testing.T) {
	t.Parallel()

	property := func(raw uint8) bool {
		burst := int(raw%8) + 1
		rl := NewRateLimiter(RateLimiterConfig{Name: "burst", Rate: 1, Burst: burst})
		for range burst {
			if !rl.Allow() {
				return false
			}
		}
		return !rl.Allow()
	}
	if err := quick.Check(property, nil); err != nil {
		t.Fatal(err)
	}
}

func strategyName(strategy BackoffStrategy) string {
	switch strategy {
	case ConstantBackoff:
		return "constant"
	case LinearBackoff:
		return "linear"
	default:
		return "exponential"
	}
}
