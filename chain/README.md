# chain

Typed, sequential chain execution: a statically typed sequence of steps where each step consumes the previous step's output and produces the next step's input type. Supports per-step progress reporting, cancellation at step boundaries, and automatic reverse-order cleanup of completed steps when a later step fails or the chain is canceled.

Mirrors `rskit-chain` (Rust) and `pykit-chain` (Python) — the same typed `Step` / builder / cleanup-on-failure semantics, expressed idiomatically in Go.

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
    "strconv"

    "github.com/kbukum/gokit/chain"
)

func main() {
    parse := chain.StepFunc("parse", func(_ chain.StepContext, in string) (int, error) {
        return strconv.Atoi(in)
    })
    double := chain.StepFunc("double", func(sctx chain.StepContext, n int) (int, error) {
        sctx.Progress(100, "doubled")
        return n * 2, nil
    })

    c := chain.Then(chain.Then(chain.New[string](), parse), double).Build()

    out, err := c.Execute(context.Background(), "21", nil)
    if err != nil {
        panic(err)
    }
    fmt.Println(out) // 42
}
```

Because Go methods cannot introduce new type parameters, steps are appended with the package-level `Then` function rather than a fluent method, so each `Then` can transform the output type.

## Key Types & Functions

| Name | Description |
|------|-------------|
| `New[T]()` | Start a builder whose input and current output types are both `T` |
| `Then(b, step)` | Append a `Step[M, N]`, transforming the output type from `M` to `N` |
| `Builder.Build()` | Produce a `*Chain[I, O]` |
| `Chain.Execute(ctx, input, progress)` | Run the chain, returning the final typed output |
| `Chain.Len()` / `Chain.IsEmpty()` | Number of steps / emptiness |
| `NewStep(id, name, fn)` / `StepFunc(id, fn)` | Construct a typed `Step[I, O]` |
| `Step.WithCleanup(fn)` | Register reverse-order cleanup run on later failure/cancellation |
| `StepContext` | Per-step context: `Context()`, `Err()`, `Progress(percent, message)` |
| `ChainProgressFn` / `StepProgress` / `StepStatus` | Chain-level progress callback and payload |

---

[⬅ Back to main README](../README.md)
