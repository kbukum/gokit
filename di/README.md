# di

Dependency injection container with eager/lazy registration, singletons, circuit breakers, and generics.

## Install

```bash
go get github.com/kbukum/gokit
```

## Quick Start

```go
package main

import (
    "fmt"
    "github.com/kbukum/gokit/di"
)

func main() {
    c := di.NewContainer()

    // Register a singleton
    c.RegisterSingleton("greeting", "Hello, World!")

    // Register a lazy constructor
    c.RegisterLazy("service", func() (string, error) {
        return "initialized on first resolve", nil
    })

    // Type-safe resolve
    val, err := di.Resolve[string](c, "greeting")
    if err != nil {
        panic(err)
    }
    fmt.Println(val) // "Hello, World!"

    // Must resolve (panics on error)
    svc := di.MustResolve[string](c, "service")
    fmt.Println(svc)

    c.Close()
}
```

## Key Types & Functions

| Name | Description |
|------|-------------|
| `Container` | DI container interface |
| `NewContainer()` / `NewSimpleContainer()` | Container constructors |
| `Register()` / `RegisterLazy()` / `RegisterEager()` | Registration modes |
| `RegisterSingleton()` | Register a pre-built instance |
| `Resolve[T]()` / `MustResolve[T]()` / `TryResolve[T]()` | Type-safe generic resolution |
| `WithRetryPolicy()` / `WithCircuitBreaker()` | Lazy registration options |
| `CircuitBreaker` / `CircuitBreakerConfig` | Built-in circuit breaker for lazy deps |

---

[⬅ Back to main README](../README.md)
