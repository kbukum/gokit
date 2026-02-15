// Package resilience provides patterns for building fault-tolerant systems.
//
// This package includes:
//   - CircuitBreaker: Prevents cascading failures by failing fast
//   - Retry: Retries failed operations with exponential backoff
//   - Bulkhead: Limits concurrent access to isolate failures
//   - RateLimiter: Controls request rate with token bucket algorithm
//
// These patterns can be combined for comprehensive resilience:
//
//	// Example: HTTP client with all patterns
//	cb := resilience.NewCircuitBreaker(resilience.DefaultCircuitBreakerConfig("http"))
//	bh := resilience.NewBulkhead(resilience.BulkheadConfig{MaxConcurrent: 10})
//	rl := resilience.NewRateLimiter(resilience.RateLimiterConfig{Rate: 100, Burst: 20})
//
//	err := cb.Execute(func() error {
//	    return bh.Execute(ctx, func() error {
//	        return rl.ExecuteWait(ctx, func() error {
//	            return httpClient.Do(req)
//	        })
//	    })
//	})
package resilience
