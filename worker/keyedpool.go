package worker

import (
	"context"
	"sync"
)

// KeyedPool wraps a Pool with singleflight-style coalescing on a caller-defined key K:
// at most one in-flight task per key.
// Concurrent submissions for the same key attach to the running task and observe the same outcome.
//
// Typical use cases: cache warmups, image pulls,
// per-resource background jobs where duplicate work is wasteful or incorrect.
//
// # State model
//
// An entry exists in the inflight map for `key` if and only if work is in flight under that key.
// "In flight" spans three phases:
//
//  1. Reserved — a caller has won the race to submit but pool.Submit has not
//     yet returned a TaskHandle.
//  2. Running  — the TaskHandle is published; the underlying pool is
//     executing the task.
//  3. Done watcher — the task has finished but the eviction goroutine has not
//     yet run. Brief.
//
// All public methods agree on this invariant. Get blocks through phase 1
// so it never lies about state; Cancel works in any phase.
//
// KeyedPool is safe for concurrent use.
type KeyedPool[K comparable, I, O any] struct {
	pool *Pool[I, O]

	mu       sync.Mutex
	inflight map[K]*keyedEntry[O]
}

// keyedEntry is the canonical record of in-flight work under a key.
//
// `ready` is closed exactly once when either `handle` is published or `err` is set.
// The close acts as the happens-before barrier for both fields,
// so they may be read without holding KeyedPool.mu after `<-ready` returns.
//
// `cancelSubmit` cancels the context passed to pool.Submit.
// Calling it during phase 1 unblocks Submit with ctx.Err(); calling it later is a no-op.
// It is held alongside (not instead of) the eventual handle's Cancel
// so a single Cancel call works in every phase.
type keyedEntry[O any] struct {
	ready        chan struct{}
	cancelSubmit context.CancelFunc
	handle       *TaskHandle[O]
	err          error
}

// NewKeyedPool wraps an existing Pool with a keyed coalescer.
//
// The caller retains ownership of the underlying Pool — KeyedPool does not stop it.
// Multiple KeyedPools (or direct Submit calls) may share a single Pool when desirable.
func NewKeyedPool[K comparable, I, O any](pool *Pool[I, O]) *KeyedPool[K, I, O] {
	return &KeyedPool[K, I, O]{
		pool:     pool,
		inflight: make(map[K]*keyedEntry[O]),
	}
}

// SubmitOrAttach submits task under key,
// or attaches to an existing in-flight submission for the same key. Returns the shared TaskHandle
// and attached=true when a prior submission was found.
//
// Cancellation:
// canceling the returned handle (or any caller's submission ctx via Cancel) terminates the single shared attempt for ALL attached observers
// — the documented semantic for coalesced work.
//
// Concurrency: KeyedPool.mu is released across pool.Submit, so submissions
// for different keys never serialize on each other (F-076 #64). Same-key
// racers wait on the entry's `ready` channel, ctx-aware.
func (k *KeyedPool[K, I, O]) SubmitOrAttach(ctx context.Context, key K, task I) (*TaskHandle[O], bool, error) {
	k.mu.Lock()
	if existing, ok := k.inflight[key]; ok {
		k.mu.Unlock()
		select {
		case <-existing.ready:
		case <-ctx.Done():
			return nil, false, ctx.Err()
		}
		if existing.err != nil {
			return nil, false, existing.err
		}
		return existing.handle, true, nil
	}

	// Reserve under the lock so concurrent same-key callers attach instead of double-submitting.
	// We derive a cancellable ctx so Cancel(key) can interrupt Submit during phase 1.
	submitCtx, cancelSubmit := context.WithCancel(ctx)
	entry := &keyedEntry[O]{
		ready:        make(chan struct{}),
		cancelSubmit: cancelSubmit,
	}
	k.inflight[key] = entry
	k.mu.Unlock()

	h, submitErr := k.pool.Submit(submitCtx, task)
	if submitErr != nil {
		// Phase 1 failure: clear the entry first, then publish the error.
		// New callers post-clear see no entry;
		// callers that captured the entry pointer pre-clear wait on ready and observe the error.
		k.mu.Lock()
		if cur, ok := k.inflight[key]; ok && cur == entry {
			delete(k.inflight, key)
		}
		k.mu.Unlock()
		entry.err = submitErr
		close(entry.ready)
		cancelSubmit() // free ctx tree
		return nil, false, submitErr
	}

	entry.handle = h
	close(entry.ready)

	// Auto-evict on completion. Spawned once per real submission;
	// the goroutine cost is the price of map cleanliness. cancelSubmit is invoked here too
	// so the derived ctx is released even on natural completion.
	go func() {
		<-h.Done()
		k.mu.Lock()
		if cur, ok := k.inflight[key]; ok && cur == entry {
			delete(k.inflight, key)
		}
		k.mu.Unlock()
		cancelSubmit()
	}()

	return h, false, nil
}

// Get returns the in-flight handle for key.
// The boolean is false when no work is in flight under key.
// The error is non-nil when an in-flight submission failed before publishing a handle (phase 1 failure).
//
// Get blocks through the reservation window: if an entry exists but the handle is not
// yet published, Get waits on the entry until the handle materializes (or the submission fails),
// honoring ctx.
func (k *KeyedPool[K, I, O]) Get(ctx context.Context, key K) (*TaskHandle[O], bool, error) {
	k.mu.Lock()
	entry, ok := k.inflight[key]
	k.mu.Unlock()
	if !ok {
		return nil, false, nil
	}
	select {
	case <-entry.ready:
	case <-ctx.Done():
		return nil, false, ctx.Err()
	}
	if entry.err != nil {
		return nil, false, entry.err
	}
	return entry.handle, true, nil
}

// Cancel cancels in-flight work under key. Returns true when an entry was found.
// Cancel is non-blocking and safe in any phase:
//
//   - Phase 1 (reservation): cancels the Submit ctx, causing Submit to
//     return ctx.Err(); SubmitOrAttach publishes the error and removes the
//     entry.
//   - Phase 2 (running): cancels the published TaskHandle. The eviction
//     goroutine removes the entry once the task observes cancellation.
//   - Phase 3 (done-watcher window): both the cancelSubmit and handle.Cancel
//     calls are idempotent no-ops.
//
// Concurrent Cancel calls under the same key are safe and idempotent.
func (k *KeyedPool[K, I, O]) Cancel(key K) bool {
	k.mu.Lock()
	entry, ok := k.inflight[key]
	k.mu.Unlock()
	if !ok {
		return false
	}
	// Always cancel the Submit ctx — no-op if Submit already returned.
	entry.cancelSubmit()
	// If the handle is already published, cancel it directly.
	// select-default keeps Cancel non-blocking during phase 1;
	// the cancelSubmit call above is sufficient to terminate that phase.
	select {
	case <-entry.ready:
		if entry.handle != nil {
			entry.handle.Cancel()
		}
	default:
	}
	return true
}

// Active returns the number of in-flight entries (all phases).
func (k *KeyedPool[K, I, O]) Active() int {
	k.mu.Lock()
	defer k.mu.Unlock()
	return len(k.inflight)
}

// Keys returns a snapshot of in-flight keys (all phases). Order is unspecified.
func (k *KeyedPool[K, I, O]) Keys() []K {
	k.mu.Lock()
	defer k.mu.Unlock()
	out := make([]K, 0, len(k.inflight))
	for key := range k.inflight {
		out = append(out, key)
	}
	return out
}
