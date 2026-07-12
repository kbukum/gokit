# hook

A lightweight, generic, observe-only event system. Register handlers for arbitrary event
types; handlers run sequentially in registration order. Non-fatal errors are aggregated and
surfaced through the canonical `on_error` event — only errors wrapping `ErrFatalHook` abort
dispatch. The package is domain-agnostic: applications define their own event types by
implementing the `Event` interface.

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

    "github.com/kbukum/gokit/hook"
)

type userCreated struct{ ID string }

func (userCreated) Type() hook.EventType { return "user_created" }

func main() {
    reg := hook.NewRegistry()

    unsubscribe := reg.On("user_created", func(ctx context.Context, e hook.Event) error {
        fmt.Printf("event: %s\n", e.Type())
        return nil
    })
    defer unsubscribe()

    _ = reg.Emit(context.Background(), userCreated{ID: "u1"})
}
```

## Key Types & Functions

| Name | Description |
|------|-------------|
| `NewRegistry()` | Create a handler registry |
| `Registry.On(type, handler)` | Register a handler; returns an unsubscribe func |
| `Registry.Emit(ctx, event)` | Dispatch an event to registered handlers |
| `Registry.HasHandlers(type)` / `Clear(types...)` | Introspect / remove handlers |
| `Event` | Interface implemented by application event types (`Type()`) |
| `EventType` | String key identifying an event type |
| `Handler` | `func(ctx, event) error` handler signature |
| `ErrorEvent` / `ErrFatalHook` | Canonical error event and fatal-abort sentinel |

---

[⬅ Back to main README](../README.md)
