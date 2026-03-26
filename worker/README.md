# worker

**Push-based task execution with real-time event streaming, worker pools, and supervision**

`worker` provides a generic `Handler[I, O]` abstraction for executing tasks that emit events during execution — progress updates, partial results, logs — with built-in pooling, dispatch strategies, supervision, middleware, and composition patterns. Designed for use cases where callers need real-time visibility into task execution: file downloaders, CLI subprocess orchestration, parallel data processing, and long-running background jobs.

## Features

- **Handler[I, O]** — single generic interface for all task execution
- **Push-based events** — handlers call `emit()` for progress, partial results, and logs during execution
- **Worker pool** — fixed-size goroutine pool with per-task handles, cancellation, and graceful shutdown
- **Dispatch strategies** — round-robin and least-loaded worker selection
- **Supervision** — panic tracking, health monitoring, backoff, and configurable restart policies
- **Middleware** — composable cross-cutting concerns (timeout, recovery) using the same `Chain` pattern as `provider`
- **Composition** — FanOut, MapReduce, and Pipeline for combining handlers
- **Provider bridges** — `FromProvider` / `AsProvider` for interop with `provider.RequestResponse`
- **Subprocess handler** — wraps `process.Command` with line-by-line stdout/stderr streaming
- **Lock-free hot path** — atomic stats, no mutex on Submit/dispatch/runWorker

## Install

```bash
go get github.com/kbukum/gokit@latest
```

Worker is part of the core module — no separate sub-module import needed.

## Quick Start

```go
package main

import (
    "context"
    "fmt"

    "github.com/kbukum/gokit/worker"
)

func main() {
    ctx := context.Background()

    // Define a handler
    h := worker.HandlerFunc[string, string](func(
        ctx context.Context, task string, emit func(worker.Event[string]),
    ) error {
        emit(worker.ProgressEvent[string](1, 2, "processing"))
        return nil
    })

    // Create a pool
    pool := worker.NewPool(h, worker.PoolConfig{Name: "demo", Size: 4})
    defer pool.Stop(ctx)

    // Submit a task
    handle, err := pool.Submit(ctx, "hello")
    if err != nil {
        panic(err)
    }

    // Consume events
    for event := range handle.Events() {
        fmt.Printf("%s: %s\n", event.Type, event.Progress)
    }

    // Get final result
    result, err := handle.Result()
    fmt.Println(result, err)
}
```

## Key Types & Functions

| Name | Description |
|------|-------------|
| `Handler[I, O]` | Interface: `Handle(ctx, task, emit) error` — unit of work |
| `HandlerFunc[I, O]` | Adapter to use ordinary functions as Handlers |
| `Event[O]` | Typed event emitted during execution (progress, partial, log, result, error) |
| `Progress` | Quantitative progress: current, total, percent, message |
| `TaskHandle[O]` | Tracks a submitted task — `Events()`, `Result()`, `Cancel()`, `Done()` |
| `Pool[I, O]` | Fixed-size worker pool with dispatch, events, and graceful shutdown |
| `PoolConfig` | Pool configuration: size, queue, dispatch strategy, supervisor |
| `PoolStats` | Pool utilization snapshot: active, idle, queued, total, failed |
| `DispatchStrategy` | `RoundRobin` or `LeastLoaded` worker selection |
| `SupervisorConfig` | Panic tracking, restart policy, backoff, health interval |
| `RestartPolicy` | `RestartNever`, `RestartOnFailure`, `RestartAlways` |
| `Middleware[I, O]` | `func(Handler[I, O]) Handler[I, O]` — wraps handlers |
| `Chain` | Composes multiple middlewares: `Chain(a, b, c)(handler)` = `a(b(c(handler)))` |
| `WithTimeout` | Middleware: enforces a deadline on each Handle call |
| `WithRecovery` | Middleware: recovers panics and converts to errors |
| `FanOut` | Sends same input to N handlers concurrently, collects results |
| `NewMapReduce` | Split → process concurrently → combine results |
| `NewPipeline` | Sequential handler chaining: output of stage N → input of stage N+1 |
| `FromProvider` | Bridges `provider.RequestResponse` → `Handler` |
| `AsProvider` | Bridges `Handler` → `provider.RequestResponse` |
| `NewSubprocessHandler` | Wraps `process.Command` as a Handler with line-by-line streaming |

## Usage Examples

### Example 1: File Downloader with Progress

```go
type DownloadInput struct {
    URL string
}
type DownloadOutput struct {
    Bytes []byte
}

downloader := worker.HandlerFunc[DownloadInput, DownloadOutput](func(
    ctx context.Context, task DownloadInput, emit func(worker.Event[DownloadOutput]),
) error {
    // Report progress during download
    for i := range 10 {
        emit(worker.ProgressEvent[DownloadOutput](
            int64(i+1), 10, fmt.Sprintf("chunk %d/10", i+1),
        ))
        // ... download chunk ...
    }
    return nil
})

pool := worker.NewPool(downloader, worker.PoolConfig{
    Name: "downloader",
    Size: 8,
})
defer pool.Stop(ctx)

handle, _ := pool.Submit(ctx, DownloadInput{URL: "https://example.com/file"})
for event := range handle.Events() {
    if event.Progress != nil {
        fmt.Printf("%.0f%%\n", event.Progress.Percent*100)
    }
}
```

### Example 2: Middleware Composition

```go
// Wrap handler with timeout and panic recovery
safe := worker.Chain(
    worker.WithTimeout[string, string](30 * time.Second),
    worker.WithRecovery[string, string](),
)(myHandler)

pool := worker.NewPool(safe, worker.PoolConfig{Name: "safe", Size: 4})
```

### Example 3: Supervised Pool

```go
pool := worker.NewPool(handler, worker.PoolConfig{
    Name: "supervised",
    Size: 8,
    Supervisor: &worker.SupervisorConfig{
        RestartPolicy:  worker.RestartOnFailure,
        MaxRestarts:    5,
        BackoffBase:    time.Second,
        HealthInterval: 30 * time.Second,
    },
})
```

When a task panics, the worker goroutine survives (panics are caught per-task).
The supervisor tracks per-worker panic counts and marks workers unhealthy after
exceeding `MaxRestarts`. Unhealthy workers are skipped by the dispatcher.

### Example 4: MapReduce — Parallel Processing with Combine

```go
mr := worker.NewMapReduce(worker.MapReduceConfig[string, string, int]{
    Name: "word-count",
    Split: func(doc string) []string {
        return strings.Split(doc, "\n") // split by line
    },
    Handler: lineCounter, // Handler[string, int]
    Combine: func(counts []int) (int, error) {
        total := 0
        for _, c := range counts {
            total += c
        }
        return total, nil
    },
    PoolSize: 4,
})

// Use as a regular handler
err := mr.Handle(ctx, document, emit)
```

### Example 5: FanOut — Same Input to Multiple Handlers

```go
// Send same audio to multiple transcription engines
multi := worker.FanOut("multi-transcribe", engineA, engineB, engineC)

// Result type is []TranscriptionResult
var results []TranscriptionResult
err := multi.Handle(ctx, audioInput, func(e worker.Event[[]TranscriptionResult]) {
    if e.Type == worker.EventResult {
        results = e.Data
    }
})
```

### Example 6: Pipeline — Sequential Stages

```go
pipeline := worker.NewPipeline[RawAudio, Text]("transcribe-pipeline",
    worker.PipelineStage{Name: "decode", Handler: audioDecoder},
    worker.PipelineStage{Name: "transcribe", Handler: transcriber},
    worker.PipelineStage{Name: "format", Handler: formatter},
)

err := pipeline.Handle(ctx, rawAudio, emit)
```

### Example 7: Subprocess with Line Streaming

```go
h := worker.NewSubprocessHandler(worker.SubprocessConfig{
    Command: process.Command{
        Binary:      "ffmpeg",
        Dir:         "/tmp",
        GracePeriod: 10 * time.Second,
    },
})

handle, _ := pool.Submit(ctx, worker.SubprocessInput{
    Args: []string{"-i", "input.mp4", "-f", "wav", "output.wav"},
})

// Each stdout/stderr line arrives as an EventPartial
for event := range handle.Events() {
    if event.Type == worker.EventPartial {
        fmt.Printf("[%s] %s\n", event.Data.Stream, event.Data.Line)
    }
}
```

### Example 8: Provider Bridge

```go
// Wrap a provider.RequestResponse as a worker Handler
handler := worker.FromProvider(myProvider)

// Use in a pool
pool := worker.NewPool(handler, worker.PoolConfig{Name: "bridged", Size: 4})

// Or expose a Handler as a provider.RequestResponse
prov := worker.AsProvider(myHandler, worker.AsProviderConfig{
    ProviderName: "my-worker",
})
result, err := prov.Execute(ctx, input)
```

### Example 9: Batch Submission

```go
handles, err := pool.SubmitBatch(ctx, []string{"task1", "task2", "task3"})
if err != nil {
    // All previously submitted tasks are canceled on error
    log.Fatal(err)
}

for _, h := range handles {
    result, err := h.Result()
    fmt.Println(result, err)
}
```

### Example 10: Pool-Level Event Monitoring

```go
pool := worker.NewPool(handler, worker.PoolConfig{
    Name:        "monitored",
    Size:        4,
    EventBuffer: 128,
})

// Monitor all events across all workers
go func() {
    for event := range pool.Events() {
        log.Printf("[%s] worker=%s task=%s type=%s",
            pool.Stats().Active, event.WorkerID, event.TaskID, event.Type)
    }
}()
```

## Architecture

### Push vs Pull

| Aspect | worker (push) | pipeline (pull) |
|--------|--------------|----------------|
| Direction | Handler pushes events to caller via `emit()` | Downstream pulls from upstream via `Next()` |
| Backpressure | Bounded event channels; drops if full | Natural — producer waits for consumer |
| Use case | Long tasks with progress, subprocess streaming | Data transformation, batch processing |
| Lifecycle | Task-scoped with explicit pool management | Lazy evaluation, runs on terminal operator |

Use **worker** when you need real-time visibility into task execution. Use **pipeline** for composable data transformations with backpressure.

### Handler ↔ Provider ↔ Process

```
provider.RequestResponse[I, O]
    ↕  FromProvider / AsProvider
worker.Handler[I, O]
    ↑  NewSubprocessHandler
process.Command → line-by-line streaming
```

- **provider** = one-in-one-out, synchronous completion
- **worker** = one-in-many-events, push-based streaming during execution
- **process** = subprocess execution with full output capture

`SubprocessConfig` wraps `process.Command` directly — shared type, no field duplication.

## Testing

Use `HandlerFunc` for deterministic test handlers:

```go
func TestMyWorker(t *testing.T) {
    h := worker.HandlerFunc[int, int](func(
        ctx context.Context, n int, emit func(worker.Event[int]),
    ) error {
        emit(worker.ProgressEvent[int](1, 1, "done"))
        return nil
    })

    pool := worker.NewPool(h, worker.PoolConfig{Name: "test", Size: 2})
    defer pool.Stop(context.Background())

    handle, err := pool.Submit(context.Background(), 42)
    if err != nil {
        t.Fatal(err)
    }
    if _, err := handle.Result(); err != nil {
        t.Fatal(err)
    }
}
```

## Performance Considerations

- **Lock-free hot path** — `stopped` is `atomic.Bool`, worker stats use `atomic.Int32`. No mutex on Submit, dispatch, or runWorker
- **Non-blocking event forwarding** — pool-level events use `select/default` to avoid blocking workers; per-task events are buffered
- **Timer management** — backoff uses `time.NewTimer` + `Stop()` (no `time.After` leaks)
- **Context.AfterFunc** — ties task context to pool context for zero-overhead cancellation propagation (Go 1.21+)
- **Atomic stats** — `PoolStats` reads use no locks, only atomic loads

## Related Packages

- **provider** — Generic provider framework with RequestResponse, Stream, Sink patterns
- **pipeline** — Pull-based data pipeline with composable operators
- **process** — Subprocess execution with context cancellation and signal handling
- **dag** — DAG execution engine for dependency-ordered orchestration

---

[⬅ Back to main README](../README.md)
