# component

Lifecycle-managed components with ordered startup/shutdown, health checks, and lazy initialization.

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
    "github.com/kbukum/gokit/component"
)

func main() {
    registry := component.NewRegistry()

    // Create a lazy component
    comp := component.NewBaseLazyComponent("cache", func(ctx context.Context) error {
        fmt.Println("initializing cache...")
        return nil
    }).WithHealthCheck(func(ctx context.Context) error {
        return nil // health check logic
    }).WithCloser(func() error {
        fmt.Println("closing cache")
        return nil
    })

    registry.Register(comp)

    ctx := context.Background()
    registry.StartAll(ctx)  // starts in registration order
    defer registry.StopAll(ctx)  // stops in reverse order

    // Check health
    for _, h := range registry.HealthAll(ctx) {
        fmt.Printf("%s: %s\n", h.Name, h.Status)
    }
}
```

## Key Types & Functions

| Name | Description |
|------|-------------|
| `Component` | Interface: `Name()`, `Start()`, `Stop()`, `Health()` |
| `ComponentHealth` | Health status with name, status, message |
| `Registry` | Manages component lifecycle with deterministic ordering |
| `BaseLazyComponent` | Thread-safe lazy initialization wrapper |
| `NewRegistry()` | Create component registry |
| `StartAll()` / `StopAll()` / `HealthAll()` | Batch lifecycle operations |

---

[⬅ Back to main README](../README.md)
