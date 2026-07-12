# chain

Sequential chain execution: run a series of operations where each step receives the output
of the previous one, with per-step progress reporting, cancellation at step boundaries, and
automatic cleanup of completed steps when a later step fails.

Mirrors `rskit-chain` (Rust) and `pykit-chain` (Python).

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

    "github.com/kbukum/gokit/chain"
)

type doubleOp struct{ chain.BaseOperation }

func (doubleOp) ID() string   { return "double" }
func (doubleOp) Name() string { return "double" }
func (doubleOp) Execute(ctx context.Context, input any, progress chain.ProgressFn) (any, error) {
    n := input.(int)
    progress(100, "doubled")
    return n * 2, nil
}

func main() {
    exec := chain.NewBuilder().
        Step(doubleOp{}).
        Step(doubleOp{}).
        Build()

    result, err := exec.Execute(context.Background(), 3, nil)
    if err != nil {
        panic(err)
    }
    fmt.Println(result.FinalOutput) // 12
}
```

## Key Types & Functions

| Name | Description |
|------|-------------|
| `NewBuilder()` | Start assembling a chain |
| `Builder.Step(op)` | Append an `Operation` to the chain |
| `Builder.WithConfig()` / `CleanupOnFailure()` / `StopOnFailure()` | Configure execution behavior |
| `Builder.Build()` | Produce an `*Executor` |
| `Executor.Execute(ctx, input, progress)` | Run the chain, returning a `*ChainResult` |
| `Operation` | Interface: `ID()`, `Name()`, `Execute()`, `Cleanup()` |
| `BaseOperation` | Embeddable no-op defaults for optional `Operation` methods |
| `ChainProgressFn` / `ProgressFn` | Per-chain and per-step progress callbacks |
| `StepResult` / `ChainResult` / `StepStatus` | Per-step and overall execution results |

---

[⬅ Back to main README](../README.md)
