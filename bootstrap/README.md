# bootstrap

Application bootstrap framework with lifecycle hooks, component registration, and startup summary.

## Install

```bash
go get github.com/kbukum/gokit
```

## Quick Start — Server

Use `Run()` for long-running services that block until a shutdown signal:

```go
import (
    "context"
    gkconfig "github.com/kbukum/gokit/config"
    "github.com/kbukum/gokit/bootstrap"
)

func main() {
    var cfg MyConfig
    gkconfig.LoadConfig("my-service", &cfg)

    app, _ := bootstrap.NewApp(&cfg)
    app.OnConfigure(func(ctx context.Context, a *bootstrap.App[*MyConfig]) error {
        // Wire services, register routes, etc.
        return nil
    })

    app.Run(context.Background()) // blocks until SIGINT/SIGTERM
}
```

## Quick Start — Task / CLI

Use `RunTask()` for CLI tools, batch jobs, and one-shot processes:

```go
func main() {
    var cfg MyConfig
    gkconfig.LoadConfig("my-tool", &cfg)

    app, _ := bootstrap.NewApp(&cfg)
    app.OnConfigure(func(ctx context.Context, a *bootstrap.App[*MyConfig]) error {
        // Wire dependencies
        return nil
    })

    app.RunTask(context.Background(), func(ctx context.Context) error {
        return processData(ctx) // runs to completion, then shuts down
    })
}
```

Both modes share the same lifecycle: Initialize → OnStart → Configure → ReadyCheck → OnReady → (execute) → OnStop → Shutdown.

## Key Types & Functions

| Name | Description |
|------|-------------|
| `App[C]` | Generic application container with lifecycle management |
| `NewApp[C]()` | Create app from typed config (must satisfy `Config` interface) |
| `Run()` | Start long-running service with signal handling |
| `RunTask()` | Execute a finite task with signal-based cancellation |
| `OnConfigure()` / `OnStart()` / `OnReady()` / `OnStop()` | Lifecycle hooks |
| `RegisterComponent()` | Add a managed component |
| `WithLogger()` / `WithGracefulTimeout()` / `WithContainer()` | App options |
| `Summary` | Tracks and displays startup summary |

---

[⬅ Back to main README](../README.md)
