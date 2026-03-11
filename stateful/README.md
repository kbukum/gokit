# gokit/stateful

Push-based stateful accumulation with configurable triggers, bounded buffers, and pluggable storage.

## Features

- ✅ **Push-based pattern** - Append values, auto-flush on triggers
- ✅ **Configurable triggers** - Time, size, count, custom (ANY/ALL logic)
- ✅ **Bounded FIFO buffers** - Min/Max with automatic eviction
- ✅ **Rate limiting** - MinInterval prevents too-frequent flushes
- ✅ **Keep-alive TTL** - Sliding window expiration (reset on activity)
- ✅ **Type-aware measurement** - Count, bytes, tokens, custom
- ✅ **Pluggable storage** - Memory (built-in), Redis, or custom
- ✅ **Multi-tenant** - Manager handles multiple accumulators
- ✅ **Thread-safe** - Concurrent append operations
- ✅ **Zero dependencies** - Core uses only stdlib

## Installation

```bash
go get github.com/kbukum/gokit/stateful@latest
```

## Quick Start

```go
import "github.com/kbukum/gokit/stateful"

// Create memory-backed accumulator
acc := stateful.NewAccumulator(
    stateful.NewMemoryStore[Event](),
    stateful.Config[Event]{
        MinSize:  100,              // Process at min 100 events
        MaxSize:  1000,             // Cap at 1000 (FIFO eviction)
        TTL:      30 * time.Minute, // Expire after 30min inactivity
        KeepAlive: true,            // Reset TTL on each append
        Triggers: []stateful.Trigger[Event]{
            stateful.TimeTrigger[Event](10 * time.Second), // OR every 10s
            stateful.SizeTrigger[Event](100),              // OR at 100 events
        },
        OnFlush: func(ctx context.Context, events []Event) error {
            return processEvents(ctx, events)
        },
    },
)

// Append events
acc.Append(ctx, event)
// Auto-flushes when triggers fire
```

## Use Cases

- **Real-time event streams** - Accumulate events, flush periodically
- **Audio buffering** - Collect audio chunks, process when ready
- **AI context windows** - Token-based accumulation for LLMs
- **Log aggregation** - Batch writes to reduce I/O
- **Metric buffering** - Time-series aggregation

## Multi-Tenant Example

```go
// Manage accumulators per user
mgr := stateful.NewManager(
    func(userID string) *stateful.Accumulator[LogEntry] {
        return stateful.NewAccumulator(
            stateful.NewRedisStore[LogEntry](redis, "logs:"+userID),
            config,
        )
    },
    30 * time.Minute,
)

// Append to user's accumulator
mgr.Append(ctx, userID, logEntry)
```

## Custom Measurer Example

```go
// Character-based accumulation (for transcripts)
charMeasurer := stateful.CustomMeasurer(func(ctx context.Context, items []Transcript) int {
    total := 0
    for _, t := range items {
        total += len(t.Text)
    }
    return total
})

acc := stateful.NewAccumulator(
    store,
    stateful.Config[Transcript]{
        MinSize: 1500,  // Process at 1500 chars
        MaxSize: 4000,  // Cap at 4000 chars (FIFO)
        Triggers: []stateful.Trigger[Transcript]{
            stateful.SizeTrigger[Transcript](1500),
        },
        OnFlush: analyzeTranscripts,
    },
    stateful.WithMeasurer(charMeasurer),
)
```

## Custom Store Implementation

Implement the `Store[V]` interface for any backend:

```go
type PostgresStore[V any] struct {
    db *sql.DB
}

func (s *PostgresStore[V]) Append(ctx context.Context, value V) error {
    // Your Postgres logic
}

// ... implement other Store methods

// Use it!
acc := stateful.NewAccumulator(&PostgresStore[Event]{db: myDB}, config)
```

## Composing with Pipeline

`stateful` and `pipeline` are complementary patterns:

```go
// Pipeline feeds Accumulator
pipeline.FromSlice(events).
    Map(transform).
    ForEach(func(ctx context.Context, e Event) error {
        return accumulator.Append(ctx, e)
    })

// Accumulator flushes to Pipeline
acc.OnFlush = func(ctx context.Context, events []Event) error {
    return pipeline.FromSlice(events).
        Filter(validate).
        Sink(kafkaPublish)
}
```

## When to Use

**Use stateful when:**
- Producer controls rate (push-based)
- Need persistence (Redis, DB)
- Real-time event streams
- Bounded accumulation with triggers

**Use pipeline when:**
- Consumer controls rate (pull-based)
- Batch processing (files, DB records)
- ETL pipelines
- Stateless transformations

## API Overview

### Core Types

- `Accumulator[V]` - Accumulates values with auto-flush
- `Manager[K, V]` - Multi-tenant accumulator management
- `Store[V]` - Storage interface (pluggable)
- `Config[V]` - Accumulator configuration
- `Trigger[V]` - When to flush
- `Measurer[V]` - How to measure

### Built-in Stores

- `MemoryStore[V]` - Fast, in-memory (not durable)
- `RedisStore[V]` - Durable, distributed (TODO)

### Built-in Measurers

- `CountMeasurer[V]()` - Count items (default)
- `ByteSizeMeasurer()` - Sum byte sizes
- `CustomMeasurer(fn)` - Custom logic

### Built-in Triggers

- `TimeTrigger[V](duration)` - Time since last flush
- `SizeTrigger[V](threshold)` - Size >= threshold
- `CustomTrigger[V](name, fn)` - Custom logic

## License

MIT

## Contributing

Contributions welcome! This is part of the [gokit](https://github.com/kbukum/gokit) framework.
