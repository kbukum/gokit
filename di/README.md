# di

Small,
type-keyed dependency injection container with eager / singleton / transient registration modes,
closeable lifecycle, and context-based cycle detection. The public API is generic
and typed end-to-end — there is no untyped registration or lookup.

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

    "github.com/kbukum/gokit/di"
)

type Store struct{ dsn string }

func main() {
    ctx := context.Background()
    c := di.NewContainer()
    defer func() { _ = c.Close(ctx) }()

    // Pre-built value (eager).
    _ = di.Register(c, "Hello, World!", di.WithName("greeting"))

    // Lazily constructed, cached (singleton).
    _ = di.RegisterSingleton(c, func(ctx context.Context) (*Store, error) {
        return &Store{dsn: "postgres://..."}, nil
    })

    // Fresh instance per resolve (transient).
    _ = di.RegisterTransient(c, func(ctx context.Context) (int, error) { return 7, nil })

    // Container-owned resource: its disposer runs on Close, in reverse order.
    _ = di.RegisterSingletonCloseable(c,
        func(ctx context.Context) (*Store, error) { return &Store{dsn: "postgres://..."}, nil },
        func(ctx context.Context, s *Store) error { return nil /* s.Close() */ },
        di.WithName("owned-store"))

    // Type-safe resolve (optionally by name).
    greeting, _ := di.Resolve[string](ctx, c, di.WithName("greeting"))
    fmt.Println(greeting) // "Hello, World!"

    // Must resolve (panics on error; startup/tests only).
    store := di.MustResolve[*Store](ctx, c)
    fmt.Println(store.dsn)
}
```

Constructor injection is the only wiring pattern:
a factory receives the resolution `context.Context`
and calls `Resolve` with it for each dependency it needs.
The active resolution chain travels in that context, so circular dependencies are detected
and returned as an error, and a canceled context aborts resolution.

Resource cleanup is opt-in: only values registered with `RegisterCloseable`
or `RegisterSingletonCloseable` are released by `Container.Close`,
which runs their disposers in reverse order of construction and joins any errors.
Plain `Register` values and unresolved singletons are never closed by the container —
the caller owns them.

## Key Types & Functions

| Name | Description |
|------|-------------|
| `Container` / `NewContainer()` | Type-keyed container |
| `Register[T]()` | Register a pre-built value (eager) |
| `RegisterSingleton[T]()` | Register a factory invoked once and cached |
| `RegisterTransient[T]()` | Register a factory invoked on every resolve |
| `RegisterCloseable[T]()` / `RegisterSingletonCloseable[T]()` | Register a container-owned resource with a disposer |
| `Resolve[T]()` / `MustResolve[T]()` / `TryResolve[T]()` | Typed resolution (context-threaded) |
| `WithName(string)` | Qualify a registration/lookup with a name |
| `Registrations()` | Introspect registrations for diagnostics/summaries |
| `Mode` / `RegistrationInfo` | Introspection types |

---

[⬅ Back to main README](../README.md)
