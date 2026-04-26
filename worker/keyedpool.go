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
	inflight map[K]*keyedEntry[O]
}

// keyedEntry tracks an in-flight submission. `ready` is closed once `handle`
// is populated (or `err` is set) so racing callers under the same key can
// wait without holding the global pool mutex during pool.Submit.
type keyedEntry[O any] struct {
	ready  chan struct{}
	handle *TaskHandle[O]
	err    error
}

// NewKeyedPool wraps an existing Pool with a keyed coalescer.
//
// The caller retains ownership of the underlying Pool — KeyedPool does not
// stop it. This allows multiple KeyedPools (or direct Submit calls) to share
// a single Pool when desirable.
func NewKeyedPool[K comparable, I, O any](pool *Pool[I, O]) *KeyedPool[K, I, O] {
	return &KeyedPool[K, I, O]{
		pool:     pool,
		inflight: make(map[K]*keyedEntry[O]),
	}
}

// SubmitOrAttach submits task under key. If a task is already in-flight for
// key, returns the existing handle with attached=true and does NOT start a
// new task. Otherwise the task is submitted to the underlying pool and the
// fresh handle is recorded under key until it completes.
//
// Cancellation semantics: canceling the returned handle (or the underlying
// task) cancels the single shared attempt for ALL attached observers — this
// is the expected behavior for coalesced work.
//
// Concurrency: the global mutex is released across pool.Submit (F-076 #64);
// racing callers for the same key reserve a sentinel entry and wait on its
// `ready` channel, so submissions for different keys never serialize on each
// other.
func (k *KeyedPool[K, I, O]) SubmitOrAttach(ctx context.Context, key K, task I) (handle *TaskHandle[O], attached bool, err error) {
	k.mu.Lock()
	if existing, ok := k.inflight[key]; ok {
		k.mu.Unlock()
		// Wait for the first submitter to publish (or fail). Honor ctx so a
		// stuck submission cannot pin this caller indefinitely.
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

	// Reserve the slot before releasing the lock so concurrent callers
	// observe an in-flight entry and attach instead of double-submitting.
	entry := &keyedEntry[O]{ready: make(chan struct{})}
	k.inflight[key] = entry
	k.mu.Unlock()

	h, submitErr := k.pool.Submit(ctx, task)
	if submitErr != nil {
		k.mu.Lock()
		// Defensive: only clear if our entry is still the current one.
		if cur, ok := k.inflight[key]; ok && cur == entry {
			delete(k.inflight, key)
		}
		k.mu.Unlock()
		entry.err = submitErr
		close(entry.ready)
		return nil, false, submitErr
	}

	entry.handle = h
	close(entry.ready)

	// Auto-evict on completion. Spawned once per real submission; we accept
	// the goroutine cost as the price of map cleanliness.
	go func() {
		<-h.Done()
		k.mu.Lock()
		// Defensive: only evict if the same entry is still recorded — a
		// retry submission after Done() but before this goroutine ran could
		// have replaced it.
		if cur, ok := k.inflight[key]; ok && cur == entry {
			delete(k.inflight, key)
		}
		k.mu.Unlock()
	}()

	return h, false, nil
}

// Get returns the in-flight handle for key, if any. The boolean is false
// when no task is observably in flight under that key — including the brief
// window where SubmitOrAttach has reserved an entry but pool.Submit has not
// yet published the handle. Callers that need to attach to a pending
// submission should use SubmitOrAttach, which waits on the entry's ready
// channel and observes the published handle.
func (k *KeyedPool[K, I, O]) Get(key K) (*TaskHandle[O], bool) {
	k.mu.Lock()
	entry, ok := k.inflight[key]
	k.mu.Unlock()
	if !ok {
		return nil, false
	}
	select {
	case <-entry.ready:
	default:
		return nil, false
	}
	if entry.err != nil || entry.handle == nil {
		return nil, false
	}
	return entry.handle, true
}

// Cancel cancels the in-flight task for key. Returns true when a task was
// found and cancellation was issued. The entry is left in place — eviction
// happens via the Done watcher once the task observes the cancellation and
// returns.
func (k *KeyedPool[K, I, O]) Cancel(key K) bool {
	h, ok := k.Get(key)
	if !ok {
		return false
	}
	h.Cancel()
	return true
}

// Active returns the number of currently in-flight tasks (including entries
// whose Submit is still in progress).
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
