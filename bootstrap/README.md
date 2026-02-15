# bootstrap

Application bootstrap framework with lifecycle hooks, component registration, and startup summary.

## Install

```bash
go get github.com/skillsenselab/gokit
```

## Quick Start

```go
package main

import (
    "context"
    "github.com/skillsenselab/gokit/bootstrap"
    "github.com/skillsenselab/gokit/logger"
    "time"
)

func main() {
    app := bootstrap.NewApp("my-service", "1.0.0",
        bootstrap.WithLogger(logger.NewDefault("my-service")),
        bootstrap.WithGracefulTimeout(10*time.Second),
    )

    app.OnConfigure(func(ctx context.Context, a *bootstrap.App) error {
        // Register components, configure DI, etc.
        return nil
    })

    app.OnStart(func(ctx context.Context) error {
        // Runs after all components start
        return nil
    })

    app.OnStop(func(ctx context.Context) error {
        // Cleanup on shutdown
        return nil
    })

    if err := app.Run(context.Background()); err != nil {
        panic(err)
    }
}
```

## Key Types & Functions

| Name | Description |
|------|-------------|
| `App` | Application container with lifecycle management |
| `NewApp()` | Create app with name, version, and options |
| `OnConfigure()` / `OnStart()` / `OnReady()` / `OnStop()` | Lifecycle hooks |
| `RegisterComponent()` | Add a managed component |
| `Run()` | Start app with signal handling and graceful shutdown |
| `WithLogger()` / `WithGracefulTimeout()` / `WithContainer()` | App options |
| `Summary` | Tracks and displays startup summary |

---

[⬅ Back to main README](../README.md)
