# resilience

Resilience patterns: circuit breaker, retry with backoff, rate limiter, and bulkhead for concurrency control.

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
| `Default*Config()` | Sensible default configurations for each pattern |

---

[⬅ Back to main README](../README.md)
