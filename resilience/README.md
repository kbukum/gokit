# resilience

Resilience patterns: circuit breaker, retry with backoff, rate limiter, and bulkhead for concurrency control.

`Policy` applies its timeout to the whole execution: rate-limiter and bulkhead admission, circuit-breaker execution, retries, and the operation. `WithTimeout` chooses the earlier of the policy and caller deadlines; `WithTimeoutIfUnset` preserves an existing caller deadline. Work must cooperate with context cancellation.

Retry only when the operation is safe to repeat. `RetryConfig.MinimumDelay` supplies a server minimum; the retry owner waits at least that long or stops if the remaining budget cannot accommodate it. `ExecuteWithRetry` selects a call's retry configuration without resetting the policy's shared limiter, bulkhead, or circuit breaker.

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
| `CalculateBackoff()` / `CalculateJitteredBackoff()` / `BackoffCalculator` | Standalone exponential backoff with optional jitter |
| `Default*Config()` | Sensible default configurations for each pattern |

---

[⬅ Back to main README](../README.md)
