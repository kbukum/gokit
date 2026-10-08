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
Configure → Modules → OnBeforeStart → StartAll → OnAfterStart → ReadyCheck → OnReady → (execute) → Quiesce → OnBeforeStop → Drain and stop components → Close the DI container → OnAfterStop → Release the owned logger.

## Modules

A module is one named capability. It declares the typed ports it provides and needs, and registers its handlers and components through a `ModuleContext`. The command decides where each module runs: add modules to one App to run them together, or split them across services and let client modules stand in for the modules that run elsewhere. Module code stays the same either way.

```go
// A port is identified by this variable, not by its name; declare it once next to its interface.
var StorePort = bootstrap.NewPort[Store]("store")

type storeModule struct{}

func (storeModule) Spec() bootstrap.ModuleSpec {
    return bootstrap.ModuleSpec{Name: "store", Provides: []bootstrap.PortRef{StorePort.Ref()}}
}

func (storeModule) Register(ctx context.Context, mc *bootstrap.ModuleContext) error {
    return bootstrap.Provide(mc, StorePort, newStore())
}

type apiModule struct{}

func (apiModule) Spec() bootstrap.ModuleSpec {
    return bootstrap.ModuleSpec{
        Name:      "api",
        Needs:     []bootstrap.PortRef{StorePort.Ref()},
        Listeners: []string{"public"},
    }
}

func (apiModule) Register(ctx context.Context, mc *bootstrap.ModuleContext) error {
    store, err := bootstrap.Need(mc, StorePort)
    if err != nil {
        return err
    }
    return mc.Handle("public", "/items", newItemsHandler(store))
}
```

The command composes them. A listener is any `bootstrap.Listener`, such as gokit's `*server.Component`:

```go
public := server.NewComponent(srv, server.WithName("public-http"))

// One process runs both modules.
err := errors.Join(
    app.Listen("public", public),
    app.Use(storeModule{}, apiModule{}),
)

// Or the API runs alone, and the store's client module stands in for the store.
err := errors.Join(
    app.Listen("public", public),
    app.Use(apiModule{}, store.ClientModule(cfg.StoreURL)),
)
```

Splitting a module out of a process does not touch the modules that need it. Each module package exports a client module next to the module itself: it has its own name, provides the same ports through a remote client, and registers the client's component if it owns connections, so the client starts before the modules that use it. `bootstrap.ValueModule(name, port, value)` provides a port with a value the command already holds, such as a test double.

`app.CheckModules()` runs the startup wiring check without registering or starting anything, so a test can prove a service composition is complete without its infrastructure.

How it behaves:

- A port is identified by its `*Port` value: two `NewPort` calls are different ports even with the same name. Port types must be interfaces.
- Listeners are HTTP only. A `Listener` is a component that quiesces and drains, with `Handle(pattern, handler) error` for routes and `Fallback(handler) error`, and it must drain as `component.DrainIngress`. A module lists the listeners it mounts on in `ModuleSpec.Listeners` and can only `Handle` routes on those. An invalid or conflicting pattern is returned as `ErrRouteConflict`.
- `ModuleContext.Fallback(listener, handler)` sets the handler for requests no route matches, such as a single-page app; a listener has one fallback, and a second is `ErrRouteConflict`. The App installs it after every module registers and answers 404 under the first path segment of every module route on that listener (for `POST /auth/login`, everything under `/auth`), so unknown API paths never reach the fallback. It compares the unescaped first segment, so an escaped slash such as `/auth%2Fx` is reserved too, and a segment that fails to decode never reaches the fallback.
- Startup checks the whole module set before any module registers. Missing ports, duplicate providers (such as a module and its client module in one App), missing listeners, every dependency cycle, listener component names that are empty, shared or already registered, and invalid declarations come back together as one `*ModuleError` inside a `*StartupError` with phase `modules`. A module whose `Register` returns without providing a declared port is reported after it registers.
- Modules register in dependency order, keeping `Use` order where they are independent. A module can only `Provide` and `Need` ports in its own spec, and it must provide every port it declares.
- The modules phase runs after `OnConfigure`. Components start in this order: those registered before `Run` or in `OnConfigure`, then module components in dependency order, then listener components in `Listen` order, then any registered in `OnBeforeStart`.
- On shutdown, listeners stop accepting requests and drain in-flight ones before module workers drain, then components stop in reverse within each component phase.
- `Register` must not block or do network I/O. Components it adds do their I/O in `Start`.
- `Use` and `Listen` only record declarations. They work before the lifecycle starts and in `OnConfigure`, and return `ErrLifecycleUsed` once the modules phase begins.
- The startup summary lists each module with its ports and their providers, routes and components.

### Testing modules

Package `bootstrap/testutil` runs modules in tests the way a service runs them:

```go
func TestAPIModule(t *testing.T) {
    app := testutil.NewApp(t)                         // quiet App; logs and summary discarded
    public := testutil.Listen(t, app, "public")       // loopback HTTP listener
    store := testutil.Capture(t, app, StorePort)      // reads a provided port after start
    if err := app.Use(apiModule{}, bootstrap.ValueModule("store", StorePort, fakeStore{})); err != nil {
        t.Fatal(err)
    }
    testutil.Start(t, app)                            // shuts down when the test ends
    resp, err := http.Get(public.URL() + "/items")
    // ...
    _ = store()
}
```

`Start` works with any `*bootstrap.App[C]`, so a test can also start a command's real composition. Startup is bounded by `testutil.DefaultStartBudget` (30 seconds) or `testutil.WithStartBudget(d)`; a hook that outlasts it is canceled with `testutil.ErrStartBudget` and startup rolls back. The budget does not cancel an app that started. `Capture` is an ordinary module that needs the port, so a missing provider fails startup with the same `*ModuleError` a service reports.

`testutil.AssertRemoteSafe` checks that a port's methods take a `context.Context` first, return an `error` last, and pass no channels, functions, unsafe pointers or interfaces at any depth (pointers, slices, maps and exported struct fields are inspected; types that encode themselves are accepted as is). `testutil.Contract` runs one behavior suite against the in-process implementation and the client module's client, so a module can move between services without changing behavior:

```go
func TestStorePortContract(t *testing.T) {
    testutil.AssertRemoteSafe(t, StorePort)
    testutil.Contract(t, StorePort, storeSuite,
        testutil.Impl[Store]{Name: "local", New: func(t *testing.T) Store { return newStore() }},
        testutil.Impl[Store]{Name: "http", New: newStoreClientOverTestApp},
    )
}
```

## Errors and ownership

Each outcome stays inspectable with `errors.As` and `errors.Is`:

- `*StartupError` reports the failed `Phase` and its `Cause`. `Rollback` holds the teardown failure, or nil. A component start failure arrives as a `*component.StartError` cause with its own registry rollback result.
- `*TaskError` from `RunTask` holds the task `Cause` and any `Shutdown` failure, so neither hides the other.
- `*ShutdownError` reports teardown failures on their own, including a logger that failed to release.

An `App` runs one lifecycle. After `Run`, `RunTask`, `Startup`, or `Shutdown`, a new start returns `ErrLifecycleUsed`. `Shutdown` runs teardown once; later calls return the same result.

If the caller's context ends during startup, startup stops after the running phase and rolls back, returning a `*StartupError` that wraps the context's cause, alongside the phase's own error when it returned one; a later phase or the `RunTask` task never runs on a canceled context. `Shutdown` also stops a running lifecycle. During startup it cancels the startup context with `ErrShutdownRequested`, waits for the running phase to return, and lets startup roll back, so no resource starts after shutdown. A shutdown requested during the ready check skips the ready hooks, and a phase that returns its context error still reports `ErrShutdownRequested`. It wakes `Run`, cancels the `RunTask` task, and waits for the task to return before teardown, so task code never runs against released resources. If startup or the task does not return within the shutdown budget, `Shutdown` returns a `*ShutdownError`; teardown still runs when they return.

The App owns the logger it creates from config and releases it last, within the shutdown budget. A logger passed with `WithLogger` is borrowed and never closed. The App always closes its DI container, including one passed with `WithContainer`.

## Admin listener

`WithAdmin(AdminConfig{Host, AllInterfaces, Port, Pprof, HealthTimeout, Metrics})` adds an App-owned diagnostics listener named `admin`. It serves `GET /livez`, `GET /readyz`, `GET /metrics` (`observability.RuntimeMetricsHandler` unless `Metrics` is set) and, with `Pprof`, `/debug/pprof/`. It serves without TLS, so the host must be a loopback or private IP address; the default is loopback, and port zero picks an ephemeral port. `AllInterfaces` binds every interface instead, for platforms such as Kubernetes that probe the pod address inside an isolated network namespace; the host must then be empty or unspecified (`0.0.0.0`, `::`), and `Pprof` is refused. `App.AdminAddr()` reports the bound address.

`/readyz` returns a `Readiness` body. It reports `starting` (503) until startup completes and `draining` (503) from the moment shutdown begins. In between it checks component health within `HealthTimeout` (2 seconds by default): any unhealthy component makes it `not_ready` (503), any degraded one `degraded` (200), otherwise `ready` (200). The listener registers before every other component in the admin shutdown phase, so it starts first and stops last, and it does not quiesce: probes and metrics answer throughout the drain.

One process owns signals: `Run` and `RunTask` handle SIGINT and SIGTERM. To host several modules in one process, add them to one App with `Use` rather than nesting apps. `Startup` and `Shutdown` serve embedding and tests.

## Key Types & Functions

| Name | Description |
|------|-------------|
| `App[C]` | Generic application container with lifecycle management |
| `NewApp[C]()` | Create app from typed config (must satisfy `Config` interface) |
| `Run()` | Start long-running service with signal handling |
| `RunTask()` | Execute a finite task with signal-based cancellation |
| `OnConfigure()` / `OnBeforeStart()` / `OnAfterStart()` / `OnReady()` / `OnBeforeStop()` / `OnAfterStop()` | Lifecycle hooks |
| `RegisterComponent()` | Add a managed component |
| `Module` / `ModuleSpec` / `ModuleContext` | A named capability, its declared ports, and its wiring handle |
| `Port[T]` / `NewPort()` / `PortRef` | Typed interface ports, identified by value |
| `Use()` / `Listen()` / `CheckModules()` | Compose modules and named listeners; check wiring without starting |
| `Provide()` / `Need()` | Fill or read a declared port inside `Register` |
| `ValueModule()` | A module that provides one port with a value, such as a test double |
| `Listener` | HTTP listener component that modules mount routes and a fallback on |
| `ModuleError` / `ModuleProblem` | Every wiring problem found at startup |
| `testutil.NewApp()` / `Start()` / `Listen()` / `Capture()` | Run modules in tests |
| `testutil.RemoteSafe()` / `testutil.Contract()` | Check a port's remote shape; run one suite against every implementation |
| `WithLogger()` / `WithGracefulTimeout()` / `WithContainer()` / `WithAdmin()` | App options |
| `AdminConfig` / `Readiness` / `AdminAddr()` | Admin listener configuration, `/readyz` body and bound address |
| `Summary` | Tracks and displays startup summary |

---

[⬅ Back to main README](../README.md)
