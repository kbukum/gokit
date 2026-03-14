# provider

Generic provider registry, manager, and selection strategies for pluggable backends.

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
    "github.com/kbukum/gokit/provider"
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
| `Meta` | Open-ended metadata annotations (`map[string]any`) |
| `WithMeta[I,O]` / `GetMeta[I,O]` | Attach and retrieve metadata on providers |
| `Connector[T]` | Lazy-init client lifecycle with double-check locking |
| `Call[C,R]` | Execute a function using a connector's client |
| `FanOutStream[I,O]` | Fan-out one stream input to N streams, merge results |
| `WindowedStream[I,O,R]` | Buffer stream items into windows, process as batch |
| `DrainIterator[T]` | Drain remaining items on close for graceful shutdown |

## Provider Metadata

Attach open-ended metadata to any provider for cost-aware ordering,
scheduling, and observability:

```go
import "github.com/kbukum/gokit/provider"

// Annotate providers with any dimensions that matter
annotated := provider.WithMeta(myProvider, provider.Meta{
    "cost":       0.05,
    "latency_ms": 200,
    "reliability": 0.999,
    "requires":   "gpu",
})

// Retrieve metadata (e.g., in DAG ordering strategies)
meta := provider.GetMeta(annotated)
cost, _ := meta.Float("cost")
latency, _ := meta.Duration("latency_ms")
```

Meta is consumed by `dag.OrderByCost()`, `dag.OrderByLatency()`, and
`dag.WeightedScore()` for intelligent node scheduling. Also available
for Sink and Stream providers via `WithSinkMeta` and `WithStreamMeta`.

## Stream Composition

Utilities for composing and processing streams:

```go
// Fan-out: send same input to multiple streams, merge all results
merged := provider.FanOutStream("multi", stream1, stream2, stream3)
iter, _ := merged.Execute(ctx, input)

// Windowed: buffer items and process in batches
batched := provider.WindowedStream("batch", videoStream, 30, frameBatchProcessor)
iter, _ := batched.Execute(ctx, videoInput) // yields one result per 30-frame window

// Drain: gracefully drain remaining items on close
drain := provider.DrainIterator(iter, 100) // drain up to 100 items on Close
```

---

[⬅ Back to main README](../README.md)
