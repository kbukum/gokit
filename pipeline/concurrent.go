package pipeline

import (
	"context"
	"sync"
)

// Buffer adds a buffered channel between pipeline stages.
// This decouples the production rate from the consumption rate.
func Buffer[T any](p *Pipeline[T], size int) *Pipeline[T] {
	if size <= 0 {
		size = 1
	}
	return &Pipeline[T]{
		create: func(ctx context.Context) Iterator[T] {
			source := p.create(ctx)
			bufCtx, cancel := context.WithCancel(ctx)
			ch := make(chan result[T], size)

			go func() {
				defer close(ch)
				for {
					val, ok, err := source.Next(bufCtx)
					if err != nil {
						select {
						case ch <- result[T]{err: err}:
						case <-bufCtx.Done():
						}
						return
					}
					if !ok {
						return
					}
					select {
					case ch <- result[T]{val: val, ok: true}:
					case <-bufCtx.Done():
						return
					}
				}
			}()

			return &channelIter[T]{
				ch: ch,
				closer: func() error {
					cancel()
					return source.Close()
				},
			}
		},
	}
}

// Parallel applies fn to each value concurrently with up to n workers.
// Order is NOT preserved. Use Map for ordered processing.
func Parallel[I, O any](p *Pipeline[I], n int, fn func(context.Context, I) (O, error)) *Pipeline[O] {
	if n <= 0 {
		n = 1
	}
	return &Pipeline[O]{
		create: func(ctx context.Context) Iterator[O] {
			source := p.create(ctx)
			workerCtx, cancel := context.WithCancel(ctx)
			out := make(chan result[O], n)
			in := make(chan I, n)

			var wg sync.WaitGroup

			// Producer: pull from source into input channel
			go func() {
				defer close(in)
				for {
					val, ok, err := source.Next(workerCtx)
					if err != nil {
						select {
						case out <- result[O]{err: err}:
						case <-workerCtx.Done():
						}
						return
					}
					if !ok {
						return
					}
					select {
					case in <- val:
					case <-workerCtx.Done():
						return
					}
				}
			}()

			// Workers: process input and write to output
			for range n {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for val := range in {
						o, err := fn(workerCtx, val)
						if err != nil {
							select {
							case out <- result[O]{err: err}:
							case <-workerCtx.Done():
							}
							cancel()
							return
						}
						select {
						case out <- result[O]{val: o, ok: true}:
						case <-workerCtx.Done():
							return
						}
					}
				}()
			}

			go func() {
				wg.Wait()
				close(out)
			}()

			return &channelIter[O]{
				ch: out,
				closer: func() error {
					cancel()
					return source.Close()
				},
			}
		},
	}
}

// Merge combines multiple pipelines concurrently.
// Values are yielded as they become available from any source.
// Order is NOT preserved.
func Merge[T any](pipelines ...*Pipeline[T]) *Pipeline[T] {
	return &Pipeline[T]{
		create: func(ctx context.Context) Iterator[T] {
			mergeCtx, cancel := context.WithCancel(ctx)
			ch := make(chan result[T], len(pipelines))
			var wg sync.WaitGroup
			iters := make([]Iterator[T], len(pipelines))

			for i, p := range pipelines {
				iters[i] = p.create(mergeCtx)
				wg.Add(1)
				go func(iter Iterator[T]) {
					defer wg.Done()
					for {
						val, ok, err := iter.Next(mergeCtx)
						if err != nil {
							select {
							case ch <- result[T]{err: err}:
							case <-mergeCtx.Done():
							}
							return
						}
						if !ok {
							return
						}
						select {
						case ch <- result[T]{val: val, ok: true}:
						case <-mergeCtx.Done():
							return
						}
					}
				}(iters[i])
			}

			go func() {
				wg.Wait()
				close(ch)
			}()

			return &channelIter[T]{
				ch: ch,
				closer: func() error {
					cancel()
					var firstErr error
					for _, iter := range iters {
						if err := iter.Close(); err != nil && firstErr == nil {
							firstErr = err
						}
					}
					return firstErr
				},
			}
		},
	}
}
