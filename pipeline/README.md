# pipeline

**Pull-based data pipeline with composable operators**

`pipeline` provides lazy, backpressure-aware data processing for Go. Pipelines pull values on demand — no work happens until you call `Collect`, `Drain`, or `ForEach`. This design naturally handles flow control without buffering or blocking.

## Features

- **Lazy evaluation** — operators compose but don't execute until pulled
- **Backpressure** — upstream producers only generate values when downstream consumers request them
- **Composable operators** — map, filter, batch, throttle, window, and more
- **Provider integration** — structurally compatible with `provider.Iterator[T]`
- **Context-aware** — all operations support cancellation and deadlines
- **Type-safe** — full generic support for strongly typed pipelines

## Install

```bash
go get github.com/kbukum/gokit@latest
```

Pipeline is part of the core module — no separate sub-module import needed.

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "github.com/kbukum/gokit/pipeline"
)

func main() {
    ctx := context.Background()
    
    // Create a pipeline from a slice
    src := pipeline.FromSlice([]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})
    
    // Double each value
    doubled := pipeline.Map(src, func(_ context.Context, n int) (int, error) {
        return n * 2, nil
    })
    
    // Keep only even numbers
    evens := pipeline.Filter(doubled, func(n int) bool {
        return n%2 == 0
    })
    
    // Collect results
    results, err := pipeline.Collect(ctx, evens)
    if err != nil {
        panic(err)
    }
    
    fmt.Println(results) // [2, 4, 6, 8, 10, 12, 14, 16, 18, 20]
}
```

## Operators

### Synchronous Operators

| Operator | Description |
|----------|-------------|
| `Map[T, O](p Pipeline[T], fn func(context.Context, T) (O, error)) Pipeline[O]` | Transform each value |
| `FlatMap[T, O](p Pipeline[T], fn func(context.Context, T) (Iterator[O], error)) Pipeline[O]` | Transform each value into multiple values |
| `Filter[T](p Pipeline[T], pred func(T) bool) Pipeline[T]` | Keep values matching predicate |
| `Tap[T](p Pipeline[T], fn func(context.Context, T) error) Pipeline[T]` | Side-effect without altering value (logging, metrics) |
| `TapEach[T](p Pipeline[[]T], fn func(context.Context, T) error) Pipeline[[]T]` | Per-element side-effect on slices |
| `FanOut[T, O](p Pipeline[T], fns ...func(context.Context, T) (O, error)) Pipeline[[]O]` | Apply multiple functions in parallel, collect as slice |
| `Reduce[T, O](p Pipeline[T], init O, fn func(O, T) O)` | Accumulate all values into one result |
| `Concat[T](pipelines ...Pipeline[T]) Pipeline[T]` | Join pipelines sequentially |

### Concurrent Operators

| Operator | Description |
|----------|-------------|
| `Buffer[T](p Pipeline[T], size int) Pipeline[T]` | Decouple producer/consumer with buffered channel |
| `Parallel[T, O](p Pipeline[T], workers int, fn func(context.Context, T) (O, error)) Pipeline[O]` | Concurrent Map with worker pool (order NOT preserved) |
| `Merge[T](pipelines ...Pipeline[T]) Pipeline[T]` | Combine pipelines concurrently (order NOT preserved) |

### Stream/Time-Aware Operators

| Operator | Description |
|----------|-------------|
| `Throttle[T](p Pipeline[T], interval time.Duration) Pipeline[T]` | Rate-limit values (drop values arriving faster than interval) |
| `Batch[T](p Pipeline[T], size int, timeout time.Duration) Pipeline[[]T]` | Collect N items or wait timeout, emit as slice |
| `Debounce[T](p Pipeline[T], quiet time.Duration) Pipeline[T]` | Wait for silence before emitting latest value |
| `TumblingWindow[T](p Pipeline[T], duration time.Duration) Pipeline[[]T]` | Non-overlapping fixed-duration windows |
| `SlidingWindow[T](p Pipeline[T], duration, slide time.Duration) Pipeline[[]T]` | Overlapping windows with configurable slide |

### Terminal Operators

| Operator | Description |
|----------|-------------|
| `Collect[T](ctx context.Context, p Pipeline[T]) ([]T, error)` | Pull all values into a slice |
| `Drain[T](p Pipeline[T], fn func(context.Context, T) error) Runnable` | Pull values and pass to function, discard results |
| `ForEach[T](ctx context.Context, p Pipeline[T], fn func(T) error) error` | Apply function to each value |

### Source Constructors

| Constructor | Description |
|-------------|-------------|
| `FromSlice[T](items []T) Pipeline[T]` | Create pipeline from slice |
| `From[T](it Iterator[T]) Pipeline[T]` | Create pipeline from any iterator (including `provider.Iterator[T]`) |
| `FromFunc[T](fn func(context.Context) (T, bool, error)) Pipeline[T]` | Create pipeline from generator function |

## Usage Examples

### Example 1: Data Transformation

```go
ctx := context.Background()

// Process a list of user IDs
userIDs := pipeline.FromSlice([]string{"u1", "u2", "u3", "u4", "u5"})

// Fetch user data
users := pipeline.Map(userIDs, func(ctx context.Context, id string) (*User, error) {
    return userService.GetByID(ctx, id)
})

// Filter active users
active := pipeline.Filter(users, func(u *User) bool {
    return u.Active
})

// Collect results
activeUsers, err := pipeline.Collect(ctx, active)
```

### Example 2: Batching & Throttling

```go
// Stream of events
events := pipeline.FromFunc(eventSource.Next)

// Rate-limit to 10 events/sec
throttled := pipeline.Throttle(events, 100*time.Millisecond)

// Batch into groups of 50 or every 5 seconds
batched := pipeline.Batch(throttled, 50, 5*time.Second)

// Process each batch
pipeline.Drain(batched, func(ctx context.Context, batch []Event) error {
    return batchProcessor.Process(ctx, batch)
}).Run(ctx)
```

### Example 3: Provider Integration

```go
import (
    "github.com/kbukum/gokit/pipeline"
    "github.com/kbukum/gokit/provider"
)

// Assume audioSource implements provider.Iterator[AudioChunk]
src := pipeline.From(audioSource)

// Transcribe audio chunks
transcribed := pipeline.FlatMap(src, func(ctx context.Context, chunk AudioChunk) (Iterator[Segment], error) {
    return transcriber.Execute(ctx, chunk)
})

// Publish to Kafka as side-effect
tapped := pipeline.Tap(transcribed, func(ctx context.Context, seg Segment) error {
    return kafkaPublisher.Send(ctx, seg)
})

// Identify speakers
identified := pipeline.Map(tapped, func(ctx context.Context, seg Segment) (IdentifiedSegment, error) {
    return speakerID.Execute(ctx, seg)
})

// Drain to final sink
pipeline.Drain(identified, finalSink.Send).Run(ctx)
```

### Example 4: Windowing

```go
// Stream of metrics
metrics := pipeline.FromFunc(metricSource.Next)

// Create 1-minute tumbling windows
windows := pipeline.TumblingWindow(metrics, 1*time.Minute)

// Aggregate each window
aggregated := pipeline.Map(windows, func(ctx context.Context, window []Metric) (Summary, error) {
    return aggregator.Summarize(window), nil
})

// Store summaries
pipeline.Drain(aggregated, summaryStore.Save).Run(ctx)
```

### Example 5: FanOut (Parallel Processing)

```go
// Process data through multiple models concurrently
src := pipeline.FromSlice(audioChunks)

results := pipeline.FanOut(src,
    modelA.Execute, // Run in parallel
    modelB.Execute,
    modelC.Execute,
)

// Each value is now []Result containing outputs from all 3 models
pipeline.Drain(results, func(ctx context.Context, outputs []Result) error {
    return combiner.Merge(ctx, outputs)
}).Run(ctx)
```

### Example 6: Error Handling

```go
src := pipeline.FromSlice(items)

processed := pipeline.Map(src, func(ctx context.Context, item Item) (Result, error) {
    // Errors propagate and halt the pipeline
    if err := validate(item); err != nil {
        return Result{}, fmt.Errorf("validation failed: %w", err)
    }
    return process(item), nil
})

// Collect will return the first error encountered
results, err := pipeline.Collect(ctx, processed)
if err != nil {
    log.Error("pipeline failed", map[string]interface{}{"error": err})
    return
}
```

### Example 7: Parallel Processing with Workers

```go
// Process 100 items with 10 concurrent workers
src := pipeline.FromSlice(items)

// Order is NOT preserved with Parallel
processed := pipeline.Parallel(src, 10, func(ctx context.Context, item Item) (Result, error) {
    return heavyProcessing(ctx, item)
})

results, err := pipeline.Collect(ctx, processed)
```

## Design Philosophy

### Pull vs Push

**Pull-based** (this library):
- Downstream pulls from upstream
- Natural backpressure — producer only works when consumer is ready
- No buffering required
- Lazy evaluation — nothing happens until terminal operator runs

**Push-based** (channels, reactive streams):
- Upstream pushes to downstream
- Requires explicit backpressure mechanism
- Buffering often needed
- Eager evaluation — producers start immediately

### Structural Compatibility with Provider

The `Iterator[T]` interface is structurally identical to `provider.Iterator[T]`:

```go
type Iterator[T any] interface {
    Next(ctx context.Context) (T, bool, error)
    Close() error
}
```

This means provider streams plug directly into pipelines:

```go
// Provider iterator
audioIterator := audioProvider.Stream(ctx, input)

// Wrap in pipeline
p := pipeline.From(audioIterator)

// Apply operators
transcribed := pipeline.FlatMap(p, transcriber.Execute)
```

## Testing

Use `FromSlice` for deterministic test data:

```go
func TestPipeline(t *testing.T) {
    ctx := context.Background()
    src := pipeline.FromSlice([]int{1, 2, 3})
    
    doubled := pipeline.Map(src, func(_ context.Context, n int) (int, error) {
        return n * 2, nil
    })
    
    result, err := pipeline.Collect(ctx, doubled)
    require.NoError(t, err)
    assert.Equal(t, []int{2, 4, 6}, result)
}
```

## Performance Considerations

- **Lazy evaluation** means no work until terminal operator runs
- **Synchronous operators** (Map, Filter) add negligible overhead
- **Concurrent operators** (Parallel, Buffer, Merge) spawn goroutines — use when I/O or CPU-heavy work benefits from parallelism
- **Throttle/Batch/Debounce** use timers — appropriate for real-time/streaming scenarios, not batch processing
- **Context cancellation** stops the pipeline immediately at the next operator

## Related Packages

- **provider** — Provider pattern with Iterator interface (structurally compatible)
- **dag** — Dependency-ordered task orchestration with batch/stream modes
- **sse** — Server-sent events broadcasting (push-based, not pull-based)

## License

[MIT](../LICENSE) — Copyright (c) 2024 kbukum
