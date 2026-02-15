# provider

Generic provider registry, manager, and selection strategies for pluggable backends.

## Install

```bash
go get github.com/skillsenselab/gokit
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "github.com/skillsenselab/gokit/provider"
)

// MyProvider implements provider.Provider
type MyProvider struct{ name string }
func (p *MyProvider) Name() string                          { return p.name }
func (p *MyProvider) IsAvailable(ctx context.Context) bool  { return true }

func main() {
    registry := provider.NewRegistry[*MyProvider]()
    selector := &provider.PrioritySelector[*MyProvider]{}
    mgr := provider.NewManager(registry, selector)

    // Register a factory
    mgr.Register("primary", func(cfg map[string]any) (*MyProvider, error) {
        return &MyProvider{name: "primary"}, nil
    })
    mgr.Initialize("primary", nil)
    mgr.SetDefault("primary")

    // Get best available provider
    p, err := mgr.Get(context.Background())
    fmt.Println(p.Name(), err) // "primary" <nil>
}
```

## Key Types & Functions

| Name | Description |
|------|-------------|
| `Provider` | Interface: `Name()` + `IsAvailable()` |
| `Registry[T]` | Generic provider registry with factory support |
| `Manager[T]` | Manages provider lifecycle and selection |
| `Factory[T]` | Factory function type for creating providers |
| `Selector[T]` | Interface for provider selection strategy |
| `PrioritySelector` / `RoundRobinSelector` / `HealthCheckSelector` | Built-in selection strategies |

---

[⬅ Back to main README](../README.md)
