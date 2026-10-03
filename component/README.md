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
| `Health` | Health status with name, status, message |
| `Registry` | Manages component lifecycle with deterministic ordering |
| `BaseLazyComponent` | Thread-safe lazy initialization wrapper |
| `NewRegistry()` | Create component registry |
| `StartAll()` / `StopAll()` / `HealthAll()` | Batch lifecycle operations |

## Coordinated shutdown

Start order remains registration order. Shutdown first calls each optional `Quiescer`, then drains `DrainIngress` before `DrainWorkers`. No clients or databases close until those drains return. `Stop` then releases `PhaseResources`, `PhaseTelemetry`, and `PhaseAdmin`, reversing registration order within each phase.

Use `RegisterInPhase` for telemetry and admin resources, or let a component declare `ShutdownPhase()`. Most components use the default resource phase. `bootstrap.App` places DI-container cleanup between resource components and telemetry. `Registry.Shutdown` exposes the same boundary to hosts that manage their own container.

Shutdown uses one bounded context and gives each operation a share of the remaining time. Quiescers must be prompt and idempotent; drain/stop methods must honor their context and release owned goroutines. No detached timeout wrapper can safely kill code that ignores cancellation. `StopAllDetailed` reports quiesce, drain, and stop errors for each component.

---

[⬅ Back to main README](../README.md)
