package worker

import (
	"context"
	"sync"
)

// FanOut sends the same input to N handlers concurrently. Returns when all complete. Events from all handlers are merged into the composite emit. Results are collected in the same order as handlers.
func FanOut[I, O any](name string, handlers ...Handler[I, O]) Handler[I, []O] {
	return HandlerFunc[I, []O](func(ctx context.Context, task I, emit func(Event[[]O])) error {
		var (
			wg      sync.WaitGroup
			mu      sync.Mutex
			errs    = make([]error, len(handlers))
			results = make([]O, len(handlers))
		)

		// safeEmit protects the caller's emit from concurrent access
		safeEmit := func(e Event[[]O]) {
			mu.Lock()
			defer mu.Unlock()
			emit(e)
		}

		wg.Add(len(handlers))
		for i, h := range handlers {
			go func(idx int, handler Handler[I, O]) {
				defer wg.Done()

				var lastResult O
				innerEmit := func(e Event[O]) {
					switch e.Type {
					case EventResult:
						lastResult = e.Data
					case EventPartial:
						lastResult = e.Data
						fallthrough
					case EventProgress, EventLog:
						fwdEvent := Event[[]O]{
							Type:      e.Type,
							WorkerID:  e.WorkerID,
							TaskID:    e.TaskID,
							Progress:  e.Progress,
							Metadata:  e.Metadata,
							Timestamp: e.Timestamp,
						}
						safeEmit(fwdEvent)
					default:
					}
				}

				err := handler.Handle(ctx, task, innerEmit)
				mu.Lock()
				errs[idx] = err
				results[idx] = lastResult
				mu.Unlock()
			}(i, h)
		}

		wg.Wait()

		for _, err := range errs {
			if err != nil {
				return err
			}
		}

		emit(resultEvent(results))
		return nil
	})
}

// MapReduceConfig configures a map-reduce handler.
type MapReduceConfig[I, O, R any] struct {
	Name     string
	Split    func(I) []O          // split input into sub-tasks
	Handler  Handler[O, R]        // process each sub-task
	Combine  func([]R) (R, error) // reduce partial results
	PoolSize int                  // concurrency for map phase (default: len(splits))
	Pool     *Pool[O, R]          // optional reusable pool; if nil, a temporary pool is created per call
}

// NewMapReduce creates a handler that splits input, processes sub-tasks concurrently via Handler, and combines results. If cfg.Pool is set, it is reused across invocations (caller manages its lifecycle). Otherwise a temporary pool is created and stopped per call.
func NewMapReduce[I, O, R any](cfg MapReduceConfig[I, O, R]) Handler[I, R] {
	return HandlerFunc[I, R](func(ctx context.Context, task I, emit func(Event[R])) error {
		subtasks := cfg.Split(task)

		pool := cfg.Pool
		ownPool := pool == nil
		if ownPool {
			poolSize := cfg.PoolSize
			if poolSize <= 0 {
				poolSize = len(subtasks)
			}
			pool = NewPool(cfg.Handler, PoolConfig{
				Name:        cfg.Name,
				Size:        poolSize,
				EventBuffer: 64,
			})
			// Ensure temporary pool is always stopped to prevent goroutine leaks.
			defer func() { _ = pool.Stop(ctx) }()
		}

		handles, err := pool.SubmitBatch(ctx, subtasks)
		if err != nil {
			return err
		}

		// Forward pool events in a synchronized goroutine. emitMu protects calls to emit from concurrent access.
		var emitMu sync.Mutex
		var eventWg sync.WaitGroup
		eventWg.Add(1)
		go func() {
			defer eventWg.Done()
			for e := range pool.Events() {
				fwd := Event[R]{
					Type:      e.Type,
					WorkerID:  e.WorkerID,
					TaskID:    e.TaskID,
					Progress:  e.Progress,
					Metadata:  e.Metadata,
					Timestamp: e.Timestamp,
				}
				emitMu.Lock()
				emit(fwd)
				emitMu.Unlock()
			}
		}()

		// Collect results
		partials := make([]R, len(handles))
		for i, h := range handles {
			result, herr := h.Result()
			if herr != nil {
				return herr
			}
			partials[i] = result
		}

		// Stop temporary pool and wait for event forwarding goroutine to drain
		if ownPool {
			_ = pool.Stop(ctx)
		}
		eventWg.Wait()

		combined, err := cfg.Combine(partials)
		if err != nil {
			return err
		}

		emit(resultEvent(combined))
		return nil
	})
}

// PipelineStage defines one step in a handler pipeline.
type PipelineStage struct {
	Name    string
	Handler Handler[any, any]
}

// NewPipeline chains handlers: output of stage N is input to stage N+1. Events from all stages are merged into the composite emit.
//
// Due to Go's generics limitations, pipeline stages use any internally with runtime type assertions. For compile-time safety, compose handlers manually or use dag with typed ports.
func NewPipeline[I, O any](name string, stages ...PipelineStage) Handler[I, O] {
	return HandlerFunc[I, O](func(ctx context.Context, task I, emit func(Event[O])) error {
		var current any = task

		for _, stage := range stages {
			var stageResult any

			stageEmit := func(e Event[any]) {
				if e.Type == EventResult {
					stageResult = e.Data
					return
				}
				// Forward non-result events
				fwd := Event[O]{
					Type:      e.Type,
					WorkerID:  e.WorkerID,
					TaskID:    e.TaskID,
					Progress:  e.Progress,
					Metadata:  e.Metadata,
					Timestamp: e.Timestamp,
				}
				emit(fwd)
			}

			if err := stage.Handler.Handle(ctx, current, stageEmit); err != nil {
				return err
			}

			if stageResult != nil {
				current = stageResult
			}
		}

		if final, ok := current.(O); ok {
			emit(resultEvent(final))
		}
		return nil
	})
}
