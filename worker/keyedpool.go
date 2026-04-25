package worker

import (
	"context"
	"sync"
)

// KeyedPool wraps a Pool with singleflight-style coalescing on a caller-defined
// key K. A second submission for an in-flight key returns the existing
// TaskHandle instead of starting a new task — useful for "no duplicate work"
// scenarios such as image pulls, cache warmups, or per-resource background
// jobs where concurrent callers must observe the same outcome.
//
// Each key may have at most one in-flight task; the entry is automatically
// evicted when the underlying TaskHandle finishes.
//
// KeyedPool is safe for concurrent use.
type KeyedPool[K comparable, I, O any] struct {
	pool *Pool[I, O]

	mu       sync.Mutex
	inflight map[K]*TaskHandle[O]
}

// NewKeyedPool wraps an existing Pool with a keyed coalescer.
//
// The caller retains ownership of the underlying Pool — KeyedPool does not
// stop it. This allows multiple KeyedPools (or direct Submit calls) to share
// a single Pool when desirable.
func NewKeyedPool[K comparable, I, O any](pool *Pool[I, O]) *KeyedPool[K, I, O] {
	return &KeyedPool[K, I, O]{
		pool:     pool,
		inflight: make(map[K]*TaskHandle[O]),
	}
}

// SubmitOrAttach submits task under key. If a task is already in-flight for
// key, returns the existing handle with attached=true and does NOT start a
// new task. Otherwise the task is submitted to the underlying pool and the
// fresh handle is recorded under key until it completes.
//
// Cancellation semantics: cancelling the returned handle (or the underlying
// task) cancels the single shared attempt for ALL attached observers — this
// is the expected behaviour for coalesced work.
func (k *KeyedPool[K, I, O]) SubmitOrAttach(ctx context.Context, key K, task I) (handle *TaskHandle[O], attached bool, err error) {
	k.mu.Lock()
	if existing, ok := k.inflight[key]; ok {
		k.mu.Unlock()
		return existing, true, nil
	}
	// Reserve the slot before submitting so a racing caller sees it. We hold
	// the lock through Submit to make "first submitter wins" deterministic.
	h, submitErr := k.pool.Submit(ctx, task)
	if submitErr != nil {
		k.mu.Unlock()
		return nil, false, submitErr
	}
	k.inflight[key] = h
	k.mu.Unlock()

	// Auto-evict on completion. Spawned once per real submission; we accept
	// the goroutine cost as the price of map cleanliness.
	go func() {
		<-h.Done()
		k.mu.Lock()
		// Defensive: only evict if the same handle is still recorded — a
		// retry submission after Done() but before this goroutine ran could
		// have replaced it.
		if cur, ok := k.inflight[key]; ok && cur == h {
			delete(k.inflight, key)
		}
		k.mu.Unlock()
	}()

	return h, false, nil
}

// Get returns the in-flight handle for key, if any. The boolean is false when
// no task is currently running under that key.
func (k *KeyedPool[K, I, O]) Get(key K) (*TaskHandle[O], bool) {
	k.mu.Lock()
	defer k.mu.Unlock()
	h, ok := k.inflight[key]
	return h, ok
}

// Cancel cancels the in-flight task for key. Returns true when a task was
// found and cancellation was issued. The entry is left in place — eviction
// happens via the Done watcher once the task observes the cancellation and
// returns.
func (k *KeyedPool[K, I, O]) Cancel(key K) bool {
	k.mu.Lock()
	h, ok := k.inflight[key]
	k.mu.Unlock()
	if !ok {
		return false
	}
	h.Cancel()
	return true
}

// Active returns the number of currently in-flight tasks.
func (k *KeyedPool[K, I, O]) Active() int {
	k.mu.Lock()
	defer k.mu.Unlock()
	return len(k.inflight)
}

// Keys returns a snapshot of currently in-flight keys.
func (k *KeyedPool[K, I, O]) Keys() []K {
	k.mu.Lock()
	defer k.mu.Unlock()
	out := make([]K, 0, len(k.inflight))
	for key := range k.inflight {
		out = append(out, key)
	}
	return out
}
