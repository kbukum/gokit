// Package worker provides push-based task execution with real-time event
// streaming, worker pools, and supervision.
//
// The core abstraction is a Handler — a function that receives typed input,
// does work, and calls emit() to push events (progress, partial results, logs)
// back to the caller during execution. Context carries cancellation.
//
// # Handler
//
// The Handler interface is the unit of work:
//
//	h := worker.HandlerFunc[string, string](func(
//	    ctx context.Context, task string, emit func(worker.Event[string]),
//	) error {
//	    emit(worker.ProgressEvent[string](50, 100, "halfway"))
//	    return nil
//	})
//
// # Pool
//
// Pool manages N goroutines running the same handler with dispatch strategies,
// event aggregation, and graceful shutdown:
//
//	pool := worker.NewPool(h, worker.PoolConfig{Name: "example", Size: 4})
//	handle, _ := pool.Submit(ctx, "hello")
//	for event := range handle.Events() {
//	    fmt.Println(event.Type, event.Data)
//	}
//
// # Middleware
//
// Middleware[I, O] wraps a Handler with cross-cutting behavior. Chain composes
// multiple middlewares (same pattern as provider.Middleware):
//
//	wrapped := worker.Chain(
//	    worker.WithTimeout[In, Out](30 * time.Second),
//	    worker.WithRecovery[In, Out](),
//	)(myHandler)
//
// # Composition
//
// Handlers compose via FanOut (same input to N handlers), NewMapReduce
// (split → process → combine), and NewPipeline (sequential chaining).
//
// # Provider Integration
//
// FromProvider bridges a provider.RequestResponse into a Handler.
// AsProvider bridges a Handler back into a provider.RequestResponse.
// NewSubprocessHandler bridges process.Run() into a Handler with line streaming.
package worker
