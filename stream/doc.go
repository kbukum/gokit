// Package stream provides composable,
// pull-based data stream operators plus a bounded push fan-out source.
//
// # Canonical shape
//
// gokit converges on the rskit-stream operator vocabulary (map/filter/fan_out/window/batch/parallel/merge/partition/…)
// but keeps an idiomatic Go pull-iterator model for transformation pipelines: pipelines are lazy —
// no work happens until values are pulled via Collect, Drain, or ForEach,
// and each stage pulls from the previous stage on demand. Pull gives natural,
// allocation-free backpressure without an explicit flow-control protocol.
//
// The genuinely push-shaped concern — fanning one source out to many independent observers —
// is served by Broadcaster, mirroring rskit's Broadcaster<T>.
// Every subscriber owns a private bounded channel;
// a subscriber that lags beyond its buffer drops overflow (backpressure by drop)
// but never blocks the broadcaster or its peers.
//
// # Bounded buffers
//
// No operator buffers without bound. Buffer clamps size <= 0 to 1;
// Broadcaster clamps its per-subscriber buffer to at least 1;
// the time/size-aware operators (Batch, TumblingWindow, SlidingWindow) emit
// and release each group as it completes.
// Concurrent operators (Parallel, Merge, Buffer) run owned goroutines bounded by ctx cancellation
// and closed via the iterator's Close.
//
// The Iterator interface is structurally compatible with provider.Iterator[T],
// so provider streams plug directly into pipelines.
//
// # Operators
//
// Synchronous (single-goroutine):
//
//   - Map: transform each value
//   - FlatMap: transform each value into multiple values
//   - Filter: keep values matching a predicate
//   - Tap: side-effect without altering the value (logging, metrics, mid-pipeline publish)
//   - TapEach: per-element side-effect on []T (e.g., after FanOut)
//   - FanOut: apply multiple functions in parallel, collect results as []O
//   - Reduce: accumulate all values into one result
//   - Concat: join pipelines sequentially
//
// Concurrent (multi-goroutine):
//
//   - Buffer: decouple producer/consumer with a buffered channel
//   - Parallel: concurrent Map with a worker pool (order NOT preserved)
//   - Merge: combine multiple pipelines concurrently (order NOT preserved)
//
// Stream/time-aware:
//
//   - Throttle: rate-limit values (drop values arriving faster than interval)
//   - Batch: collect N items or wait timeout, emit as slice
//   - Debounce: wait for silence before emitting the latest value
//   - TumblingWindow: non-overlapping fixed-duration windows
//   - SlidingWindow: overlapping windows with configurable slide
//
// Push fan-out:
//
//   - Broadcaster: bounded, cancellable one-to-many event fan-out (drop overflow)
//
// # Usage
//
//	src := stream.FromSlice([]int{1, 2, 3, 4, 5})
//	doubled := stream.Map(src, func(_ context.Context, n int) (int, error) {
//	    return n * 2, nil
//	})
//	evens := stream.Filter(doubled, func(n int) bool { return n%2 == 0 })
//	results, _ := stream.Collect(ctx, evens)
//
// With providers:
//
//	src := stream.From(audioSource)
//	transcribed := stream.FlatMap(src, transcriber.Execute)
//	tapped := stream.Tap(transcribed, kafkaPublish.Send)
//	identified := stream.Map(tapped, speakerID.Execute)
//	stream.Drain(identified, finalSink.Send).Run(ctx)
package stream
