// Package pipeline provides composable, pull-based data pipeline operators.
//
// Pipelines are lazy — no work happens until values are pulled via Collect,
// Drain, or ForEach. Each stage pulls from the previous stage on demand,
// providing natural backpressure without explicit flow control.
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
// # Usage
//
//	src := pipeline.FromSlice([]int{1, 2, 3, 4, 5})
//	doubled := pipeline.Map(src, func(_ context.Context, n int) (int, error) {
//	    return n * 2, nil
//	})
//	evens := pipeline.Filter(doubled, func(n int) bool { return n%2 == 0 })
//	results, _ := pipeline.Collect(ctx, evens)
//
// With providers:
//
//	src := pipeline.From(audioSource)
//	transcribed := pipeline.FlatMap(src, transcriber.Execute)
//	tapped := pipeline.Tap(transcribed, kafkaPublish.Send)
//	identified := pipeline.Map(tapped, speakerID.Execute)
//	pipeline.Drain(identified, finalSink.Send).Run(ctx)
package pipeline
