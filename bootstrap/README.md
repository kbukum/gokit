# bootstrap

Application bootstrap framework with lifecycle hooks, component registration, and startup summary.

`App.DisplaySummary(ctx)` and `Summary.DisplaySummary(ctx, registry, container, logger)` pass the caller's context to health probes. Startup rollback preserves context values but detaches cancellation and applies the configured shutdown timeout, so a canceled startup cannot prevent cleanup.

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

// main reports run's error and exits non-zero; the App's own logger is already released.
func run(ctx context.Context) error {
    var cfg MyConfig
    if err := gkconfig.LoadConfig("my-service", &cfg); err != nil {
        return err
    }

    app, err := bootstrap.NewApp(&cfg)
    if err != nil {
        return err
    }
    app.OnConfigure(func(ctx context.Context, a *bootstrap.App[*MyConfig]) error {
        // Wire services, register routes, etc.
        return nil
    })

    return app.Run(ctx) // blocks until SIGINT/SIGTERM
}
```

## Quick Start — Task / CLI

Use `RunTask()` for CLI tools, batch jobs, and one-shot processes:

```go
func run(ctx context.Context) error {
    var cfg MyConfig
    if err := gkconfig.LoadConfig("my-tool", &cfg); err != nil {
        return err
    }

    app, err := bootstrap.NewApp(&cfg)
    if err != nil {
        return err
    }
    app.OnConfigure(func(ctx context.Context, a *bootstrap.App[*MyConfig]) error {
        // Wire dependencies
        return nil
    })

    return app.RunTask(ctx, func(ctx context.Context) error {
        return processData(ctx) // runs to completion, then shuts down
    })
}
```

Both modes share the same lifecycle:
Configure → OnBeforeStart → StartAll → OnAfterStart → ReadyCheck → OnReady → (execute) → Quiesce → OnBeforeStop → Drain and stop components → Close the DI container → OnAfterStop → Release the owned logger.

## Errors and ownership

Each outcome stays inspectable with `errors.As` and `errors.Is`:

- `*StartupError` reports the failed `Phase` and its `Cause`. `Rollback` holds the teardown failure, or nil. A component start failure arrives as a `*component.StartError` cause with its own registry rollback result.
- `*TaskError` from `RunTask` holds the task `Cause` and any `Shutdown` failure, so neither hides the other.
- `*ShutdownError` reports teardown failures on their own, including a logger that failed to release.

An `App` runs one lifecycle. After `Run`, `RunTask`, `Startup`, or `Shutdown`, a new start returns `ErrLifecycleUsed`. `Shutdown` runs teardown once; later calls return the same result.

`Shutdown` also stops a running lifecycle. During startup it cancels the startup context with `ErrShutdownRequested`, waits for the running phase to return, and lets startup roll back, so no resource starts after shutdown. A shutdown requested during the ready check skips the ready hooks, and a phase that returns its context error still reports `ErrShutdownRequested`. It wakes `Run`, cancels the `RunTask` task, and waits for the task to return before teardown, so task code never runs against released resources. If startup or the task does not return within the shutdown budget, `Shutdown` returns a `*ShutdownError`; teardown still runs when they return.

The App owns the logger it creates from config and releases it last, within the shutdown budget. A logger passed with `WithLogger` is borrowed and never closed. The App always closes its DI container, including one passed with `WithContainer`.

One process owns signals: `Run` and `RunTask` handle SIGINT and SIGTERM. To host several modules in one process, register their components on one App rather than nesting apps. `Startup` and `Shutdown` serve embedding and tests.

## Key Types & Functions

| Name | Description |
|------|-------------|
| `App[C]` | Generic application container with lifecycle management |
| `NewApp[C]()` | Create app from typed config (must satisfy `Config` interface) |
| `Run()` | Start long-running service with signal handling |
| `RunTask()` | Execute a finite task with signal-based cancellation |
| `OnConfigure()` / `OnBeforeStart()` / `OnAfterStart()` / `OnReady()` / `OnBeforeStop()` / `OnAfterStop()` | Lifecycle hooks |
| `RegisterComponent()` | Add a managed component |
| `WithLogger()` / `WithGracefulTimeout()` / `WithContainer()` | App options |
| `Summary` | Tracks and displays startup summary |

---

[⬅ Back to main README](../README.md)
