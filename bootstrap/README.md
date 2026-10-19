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

A one-shot command can use `RunPortTask` to consume a module port through the normal lifecycle. A missing provider fails startup before the task runs.

Pass `WithoutStartupSummary()` to `NewApp` to suppress a task's startup banner without suppressing lifecycle logs.

Both modes share the same lifecycle:
Configure → Modules → OnBeforeStart → StartAll → OnAfterStart → ReadyCheck → OnReady → (execute) → Quiesce → OnBeforeStop → Drain and stop components → Close the DI container → OnAfterStop → Release the owned logger.

## Choosing modules or the DI container

The App offers two ways to wire dependencies. Use one per dependency, not both.

- **Modules and ports** are the default for anything a command composes: a capability with handlers, components or listeners, or one that may run local or remote behind the same interface. Wiring is typed, checked before anything starts, and reported all at once as a `ModuleError`.
- **`App.Container`** (the `di` container) suits small apps and lazily built values that no module provides, such as a value resolved by name inside `OnConfigure`. It has no startup wiring check, and modules cannot see it.

When a value is needed across modules, give it a port. Keep the container for app-local values that never cross a module boundary.

## Modules

A module declares the typed ports it provides and needs, then registers handlers and components through `ModuleContext`. Commands compose local modules or remote client modules behind the same ports.

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
    app.RegisterListener("public", public),
    app.Use(storeModule{}, apiModule{}),
)

// Or the API runs alone, and the store's client module stands in for the store.
err := errors.Join(
    app.RegisterListener("public", public),
    app.Use(apiModule{}, store.ClientModule(cfg.StoreURL)),
)
```

Client modules provide the same ports and register owned connections before their consumers start. `ValueModule(name, port, value)` supplies an existing value, such as a test double.

`app.CheckModules()` runs the startup wiring check without registering or starting anything, so a test can prove a service composition is complete without its infrastructure.

How it behaves:

- A port is identified by its `*Port` value: two `NewPort` calls are different ports even with the same name. Port types must be interfaces.
- Listeners are HTTP only. A `Listener` is a component that quiesces and drains, with `Handle(pattern, handler) error` for routes and `Fallback(handler) error`, and it must drain as `component.DrainIngress`. A module lists the listeners it mounts on in `ModuleSpec.Listeners` and can only `Handle` routes on those. Patterns follow `http.ServeMux`; the unqualified `/` is reserved for fallback dispatch. An invalid or conflicting pattern is returned as `ErrRouteConflict`. Direct listener `Fallback` calls replace the previous handler, while the module layer rejects competing fallback owners.
- `ModuleContext.Fallback` allows one fallback per listener. Duplicate owners return `ErrRouteConflict`. Dispatch reserves every module route's first path segment, including escaped slashes and malformed escapes, so unknown API paths return 404 instead of reaching an SPA.
- Startup checks the whole module set before any module registers. Missing ports, duplicate providers (such as a module and its client module in one App), missing listeners, every dependency cycle, listener component names that are empty, shared or already registered, and invalid declarations come back together as one `*ModuleError` inside a `*StartupError` with phase `modules`. A module whose `Register` returns without providing a declared port is reported after it registers.
- Modules register in dependency order, keeping `Use` order where they are independent. A module can only `Provide` and `Need` ports in its own spec, and it must provide every port it declares.
- The modules phase runs after `OnConfigure`. Components start in this order: those registered before `Run` or in `OnConfigure`, then module components in dependency order, then listener components in `RegisterListener` order, then any registered in `OnBeforeStart`.
- On shutdown, listeners stop accepting requests and drain in-flight ones before module workers drain, then components stop in reverse within each component phase.
- `Register` must not block or do network I/O. Components it adds do their I/O in `Start`.
- `Use` and `RegisterListener` only record declarations. They work before the lifecycle starts and in `OnConfigure`, and return `ErrLifecycleUsed` once the modules phase begins.
- The startup summary shows service identity and a tree of module ports, routes, and components.

### Testing modules

Package `bootstrap/testutil` runs modules in tests the way a service runs them:

```go
func TestAPIModule(t *testing.T) {
    app := testutil.NewApp(t)                         // quiet App; logs and summary discarded
    public := testutil.RegisterListener(t, app, "public")       // loopback HTTP listener
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

The loopback listener supports HTTP/1 and h2c, including production Connect unary and streaming clients. `testutil.AssertListenerRoutes(t, listener, handler)` checks a fresh listener against the shared route/fallback contract without starting it.

`testutil.AssertRemoteShape` checks that a port's methods take a `context.Context` first, return an `error` last, and pass no channels, functions, unsafe pointers, interfaces or complex numbers at any depth (pointers, slices, maps and exported struct fields are inspected; types that encode themselves are accepted as is). This validates shape, not serialization or behavioral compatibility. `testutil.Contract` runs one behavior suite against the in-process implementation and the client module's client, so a module can move between services without changing behavior:

```go
func TestStorePortContract(t *testing.T) {
    testutil.AssertRemoteShape(t, StorePort)
    testutil.Contract(t, StorePort, storeSuite,
        testutil.Implementation[Store]{Name: "local", New: func(t *testing.T) Store { return newStore() }},
        testutil.Implementation[Store]{Name: "http", New: newStoreClientOverTestApp},
    )
}
```

## Errors and ownership

Each outcome stays inspectable with `errors.As` and `errors.Is`:

- `*StartupError` reports the failed `Phase` and its `Cause`. `Rollback` holds the teardown failure, or nil. A component start failure arrives as a `*component.StartError` cause with its own registry rollback result.
- `*TaskError` from `RunTask` holds the task `Cause` and any `Shutdown` failure, so neither hides the other.
- `*ShutdownError` reports teardown failures on their own, including a logger that failed to release.

An `App` runs one lifecycle. After `Run`, `RunTask`, `Startup`, or `Shutdown`, a new start returns `ErrLifecycleUsed`. `Shutdown` runs teardown once; later calls return the same result.

Cancellation stops startup after the active phase and returns `StartupError` with the context and phase causes. Later phases and tasks do not run.

`Shutdown` cancels startup with `ErrShutdownRequested` and waits for rollback, or cancels a running task and waits before releasing dependencies. Shutdown during readiness skips ready hooks. If active work exceeds the shutdown budget, `ShutdownError` reports the timeout; teardown still runs when that work returns.

The App owns the logger it creates from config and releases it last, within the shutdown budget. A logger passed with `WithLogger` is borrowed and never closed. The App always closes its DI container, including one passed with `WithContainer`.

## Admin listener

`WithAdmin(AdminConfig{Host, AllInterfaces, Port, Pprof, HealthTimeout})` adds an App-owned diagnostics listener named `admin`. It serves `GET /livez`, `GET /readyz`, `GET /metrics` (`observability.RuntimeMetricsHandler` unless `WithAdminMetrics` injects a handler) and, with `Pprof`, `/debug/pprof/`. It serves without TLS, so the host must be a loopback or private IP address; the default is loopback, and port zero picks an ephemeral port. `AllInterfaces` binds every interface instead, for platforms such as Kubernetes that probe the pod address inside an isolated network namespace; the host must then be empty or unspecified (`0.0.0.0`, `::`), and `Pprof` is refused. `App.AdminAddr()` reports the bound address.

`/readyz` returns a `Readiness` body. It reports `starting` (503) until startup completes and `draining` (503) from the moment shutdown begins. In between it checks component health within `HealthTimeout` (2 seconds by default): any unhealthy component makes it `not_ready` (503), any degraded one `degraded` (200), otherwise `ready` (200). `App.ReadyCheck` applies the same rule and returns `ErrNotReady` only for `not_ready`; startup logs a warning for degraded or not-ready components and continues. The listener registers before every other component in the admin shutdown phase, so it starts first and stops last, and it does not quiesce: probes and metrics answer throughout the drain.

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
| `Use()` / `RegisterListener()` / `CheckModules()` | Compose modules and named listeners; check wiring without starting |
| `Provide()` / `Need()` | Fill or read a declared port inside `Register` |
| `ValueModule()` | A module that provides one port with a value, such as a test double |
| `RunPortTask()` | Run a finite task against the value a module provides for a port |
| `Listener` | HTTP listener component that modules mount routes and a fallback on |
| `ModuleError` / `ModuleProblem` | Every wiring problem found at startup |
| `testutil.NewApp()` / `Start()` / `RegisterListener()` / `Capture()` | Run modules in tests |
| `testutil.ValidateRemoteShape()` / `testutil.Contract()` | Check a port's remote shape; run one suite against every implementation |
| `WithLogger()` / `WithGracefulTimeout()` / `WithContainer()` / `WithAdmin()` / `WithAdminMetrics()` | App options |
| `AdminConfig` / `Readiness` / `AdminAddr()` | Admin listener configuration, `/readyz` body and bound address |
| `Summary` | Tracks and displays startup summary |

---

[⬅ Back to main README](../README.md)
