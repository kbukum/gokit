package resilience

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
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

func TestPolicyJSONRoundTrip(t *testing.T) {
	t.Parallel()

	// A fully-populated policy, including a Retry block carrying non-serializable
	// callbacks, must marshal without error (func fields are json:"-") and decode
	// back to the same serializable shape via snake_case keys.
	in := &Policy{
		Retry: &RetryConfig{
			MaxAttempts:    4,
			InitialBackoff: 50 * time.Millisecond,
			MaxBackoff:     2 * time.Second,
			Strategy:       LinearBackoff,
			BackoffFactor:  1.5,
			Jitter:         0.2,
			Rand:           func() float64 { return 0 },
			RetryIf:        func(error) bool { return true },
			OnRetry:        func(int, error, time.Duration) {},
		},
		CircuitBreaker: &CircuitBreakerConfig{Name: "cb", MaxFailures: 5, Timeout: time.Second, OnStateChange: func(string, State, State) {}},
		RateLimiter:    &RateLimiterConfig{Name: "rl", Rate: 10, Burst: 2, OnLimit: func(string) {}},
		Bulkhead:       &BulkheadConfig{Name: "bh", MaxConcurrent: 3, MaxWait: time.Second, OnReject: func(string) {}},
		Timeout:        5 * time.Second,
	}

	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal policy: %v", err)
	}
	if !strings.Contains(string(data), `"max_attempts"`) || !strings.Contains(string(data), `"circuit_breaker"`) {
		t.Fatalf("expected snake_case keys, got %s", data)
	}
	if strings.Contains(string(data), `"strategy":0`) {
		t.Fatalf("strategy should encode as text, got %s", data)
	}

	var out Policy
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("Unmarshal policy: %v", err)
	}
	if out.Retry == nil || out.Retry.MaxAttempts != 4 || out.Retry.Strategy != LinearBackoff {
		t.Fatalf("retry did not round-trip: %+v", out.Retry)
	}
	if out.CircuitBreaker == nil || out.CircuitBreaker.MaxFailures != 5 {
		t.Fatalf("circuit breaker did not round-trip: %+v", out.CircuitBreaker)
	}
	if out.Timeout != 5*time.Second {
		t.Fatalf("timeout did not round-trip: %v", out.Timeout)
	}
}
