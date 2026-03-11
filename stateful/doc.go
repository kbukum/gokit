// Package stateful provides push-based stateful accumulation with configurable triggers,
// bounded buffers, and pluggable storage backends.
//
// # Core Concepts
//
// Accumulator - Collects values of type V and flushes them based on configurable triggers
// (time, size, custom). Supports bounded FIFO buffers, keep-alive TTL, rate limiting,
// and type-aware measurement.
//
// Manager - Manages multiple named accumulators for multi-tenant use cases (per session,
// per user, per stream, etc.).
//
// Store - Pluggable storage backend interface. Built-in implementations: memory (fast, local)
// and Redis (durable, distributed). Users can implement custom stores for any backend
// (Postgres, DynamoDB, filesystem, etc.).
//
// # Use Cases
//
//   - Real-time event stream accumulation
//   - Audio chunk buffering with processing triggers
//   - AI context window management (token-based accumulation)
//   - Log aggregation and batch writes
//   - Metric buffering and time-series aggregation
//
// # Pattern: Push-Based Accumulation
//
// Unlike pipeline (pull-based Iterator), stateful uses push-based Append pattern.
// The producer pushes values, and the accumulator decides when to flush based on triggers.
//
// # Basic Example
//
//	// Create memory-backed accumulator
//	acc := stateful.NewAccumulator(
//	    stateful.NewMemoryStore[Event](),
//	    stateful.Config[Event]{
//	        MinSize:  100,              // Process at minimum 100 events
//	        MaxSize:  1000,             // Cap at 1000 (FIFO eviction)
//	        TTL:      30 * time.Minute, // Expire after 30min inactivity
//	        KeepAlive: true,            // Reset TTL on each append
//	        Triggers: []stateful.Trigger[Event]{
//	            stateful.TimeTrigger[Event](10 * time.Second), // OR every 10s
//	            stateful.SizeTrigger[Event](100),              // OR at 100 events
//	        },
//	        OnFlush: func(ctx context.Context, events []Event) error {
//	            return processEvents(ctx, events)
//	        },
//	    },
//	)
//
//	// Append events
//	acc.Append(ctx, event)
//	// Auto-flushes when triggers fire
//
// # Multi-Tenant Example
//
//	// Manage accumulators per user
//	mgr := stateful.NewManager(
//	    func(userID string) *stateful.Accumulator[LogEntry] {
//	        return stateful.NewAccumulator(
//	            stateful.NewRedisStore[LogEntry](redis, "logs:"+userID),
//	            config,
//	        )
//	    },
//	    30 * time.Minute,
//	)
//
//	// Append to user's accumulator
//	mgr.Append(ctx, userID, logEntry)
//
// # Composing with Pipeline
//
// stateful and pipeline are complementary patterns that compose naturally:
//
//	// Pipeline feeds Accumulator
//	pipeline.FromSlice(events).
//	    Map(transform).
//	    ForEach(func(ctx context.Context, e Event) error {
//	        return accumulator.Append(ctx, e)
//	    })
//
//	// Accumulator flushes to Pipeline
//	acc.OnFlush = func(ctx context.Context, events []Event) error {
//	    return pipeline.FromSlice(events).
//	        Filter(validate).
//	        Sink(kafkaPublish)
//	}
//
// # When to Use
//
// Use stateful when:
//   - Producer controls the rate (push-based)
//   - Need persistence between restarts (Redis, DB)
//   - Real-time event streams (Kafka, websockets)
//   - Bounded accumulation with triggers
//
// Use pipeline when:
//   - Consumer controls the rate (pull-based)
//   - Batch processing (files, DB records)
//   - ETL pipelines
//   - Stateless transformations
package stateful
