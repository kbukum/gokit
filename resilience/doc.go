// Package resilience provides patterns for building fault-tolerant systems: circuit breaker, retry with backoff, rate limiting and concurrency limits.
//
// A [Policy] combines them for one dependency. Its order, from outside in, is timeout, rate limiter, bulkhead, circuit breaker, retry, then the operation:
//
//	policy := resilience.NewPolicy().
//		WithTimeout(5 * time.Second).
//		WithBulkhead(resilience.BulkheadConfig{MaxConcurrent: 10}).
//		WithCircuitBreaker(resilience.DefaultCircuitBreakerConfig("billing")).
//		WithRetry(resilience.DefaultRetryConfig())
//	invoice, err := resilience.Execute(ctx, policy, func(ctx context.Context) (Invoice, error) {
//		return billing.Get(ctx, id)
//	})
//
// # Choosing a concurrency limit
//
//   - [Bulkhead] (or [Policy.WithBulkhead]) caps concurrent calls to one dependency. A caller waits in a bounded queue for a slot, so use it where waiting briefly is better than failing.
//   - [Policy.Acquire] holds the policy's timeout, bulkhead slot and breaker admission for a long-lived operation such as a stream, until its finish function is called.
//   - [ConcurrencyLimit] never waits. It caps work overall and per key (for example per tenant), so one key cannot take every slot; a caller that gets no slot should refuse the work, typically with a busy or rate-limit error.
//   - [KeyedRateLimiter] and [KeyedComponent] limit how often each key may act, not how many of its calls run at once.
package resilience
