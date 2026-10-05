# resilience

Resilience patterns: circuit breaker, retry with backoff, rate limiter, and bulkhead for concurrency control.

`Policy` applies its timeout to the whole execution: rate-limiter and bulkhead admission, circuit-breaker execution, retries, and the operation. `WithTimeout` chooses the earlier of the policy and caller deadlines; `WithTimeoutIfUnset` preserves an existing caller deadline. Work must cooperate with context cancellation.

Retry only when the operation is safe to repeat. `RetryConfig.MinimumDelay` supplies a server minimum; the retry owner waits at least that long or stops if the remaining budget cannot accommodate it. `ExecuteWithRetry` selects a call's retry configuration without resetting the policy's shared limiter, bulkhead, or circuit breaker.

## Long-lived operations

`Policy.Acquire(ctx)` returns a call context and an idempotent `finish(error) error` function. Unlike `Execute`, it keeps the timeout, bulkhead slot and circuit-breaker admission alive after acquisition returns. It uses the same policy state as unary execution and never retries.

Use the returned context for the whole operation. After closing the operation's resources, pass its terminal outcome to `finish` and use the returned error. This order matters: cancellation signals cleanup, but capacity stays occupied until cleanup has finished. Every successful acquisition requires a finish call, including cancellation, timeout, decoder failure and early close. Normalize a successful protocol completion to nil; an early close is cancellation, not evidence of successful completion.

`finish(nil)` explicitly reports success. If cancellation or timeout stopped the operation, report its context error; the completion function cannot infer protocol success from context state. This keeps successful unary callback results authoritative even if a deadline races with their return. Explicit upstream and cleanup errors are preserved. Repeated or concurrent calls return the first outcome and release resources once. Canceled calls are neutral to breaker health and return half-open probe capacity; deadlines and other failures count against the breaker. Admission rejection does not count as an upstream failure. Results from before a breaker transition or reset cannot change the current generation. Unary execution shares these rules; a panicking callback still propagates its panic without certifying success or leaking admission.

Set a policy timeout or supply a caller deadline when a finite budget is required. A nil policy adds no timeout. No reader, watcher or cleanup goroutine is started by `Acquire`; the transport owns I/O cancellation and resource teardown. See `ExamplePolicy_Acquire` for the ownership sequence.

## Install

```bash
go get github.com/kbukum/gokit
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "net/http"
    "github.com/kbukum/gokit/resilience"
)

func main() {
    ctx := context.Background()

    // Retry with exponential backoff
    body, err := resilience.Retry(ctx, resilience.DefaultRetryConfig(), func() (string, error) {
        resp, err := http.Get("https://api.example.com/data")
        if err != nil {
            return "", err
        }
        defer resp.Body.Close()
        return "ok", nil
    })

    // Circuit breaker
    cb := resilience.NewCircuitBreaker(resilience.DefaultCircuitBreakerConfig("my-api"))
    err = cb.Execute(func() error {
        _, err := http.Get("https://api.example.com/health")
        return err
    })
    fmt.Println(cb.State()) // StateClosed, StateOpen, or StateHalfOpen

    // Rate limiter
    rl := resilience.NewRateLimiter(resilience.DefaultRateLimiterConfig("api"))
    if rl.Allow() {
        fmt.Println("request allowed")
    }
}
```

## Key Types & Functions

| Name | Description |
|------|-------------|
| `Retry[T]()` / `RetryFunc()` / `RetryWithBackoff[T]()` | Generic retry with exponential backoff |
| `CircuitBreaker` | Circuit breaker with closed/open/half-open states |
| `RateLimiter` | Token bucket rate limiter |
| `Bulkhead` | Concurrency limiter with semaphore pattern |
| `Policy.Acquire()` | Whole-operation admission with a call context and one terminal completion |
| `CalculateBackoff()` / `CalculateJitteredBackoff()` / `BackoffCalculator` | Standalone exponential backoff with optional jitter |
| `Default*Config()` | Sensible default configurations for each pattern |

---

[⬅ Back to main README](../README.md)
