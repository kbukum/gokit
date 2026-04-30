package resilience

import (
	"context"
	"fmt"
	"time"
)

func Example_policy() {
	policy := NewPolicy().
		WithRateLimiter(RateLimiterConfig{Name: "example", Rate: 100, Burst: 1}).
		WithBulkhead(BulkheadConfig{Name: "example", MaxConcurrent: 1, MaxWait: time.Second}).
		WithCircuitBreaker(CircuitBreakerConfig{Name: "example", MaxFailures: 2, Timeout: time.Second, HalfOpenMaxCalls: 1}).
		WithTimeout(time.Second).
		WithRetry(RetryConfig{MaxAttempts: 2, InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond, Strategy: ConstantBackoff})

	attempts := 0
	result, err := Execute(context.Background(), policy, func(ctx context.Context) (string, error) {
		attempts++
		if attempts == 1 {
			return "", fmt.Errorf("transient failure")
		}
		return fmt.Sprintf("ok after %d attempts", attempts), nil
	})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(result)

	// Output: ok after 2 attempts
}
