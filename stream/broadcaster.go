package stream

import (
	"context"
	"sync"
)

// DefaultBroadcastBuffer is the per-subscriber buffer used when no buffer size is configured. A subscriber lagging more than this many unconsumed events drops the overflow rather than stalling the broadcaster.
const DefaultBroadcastBuffer = 64

// Broadcaster is a bounded, cancellable fan-out source: it turns "observe a backend" into a bounded stream of typed events delivered to many independent subscribers. Each subscriber owns a private bounded channel — a subscriber that falls further behind than the buffer loses interim events (backpressure by drop) but never blocks the broadcaster or its peers. This is the canonical owner for the "watch a source → typed change stream" shape that recurs across config reloads, service discovery, cache invalidation, and secret rotation.
//
// Share a Broadcaster by pointer; every holder observes the same subscriber set. It is safe for concurrent use. NewBroadcaster is the canonical constructor, but the zero value is also usable: it lazily initializes on first use with the default buffer.
type Broadcaster[T any] struct {
	mu     sync.Mutex
	subs   []*subscriber[T]
	buffer int
	done   chan struct{}
	closed bool
}

type subscriber[T any] struct {
	ch     chan T
	closed bool
}

type broadcasterConfig struct {
	buffer int
}

// BroadcasterOption configures a Broadcaster at construction time.
type BroadcasterOption func(*broadcasterConfig)

// WithBroadcastBuffer sets the per-subscriber buffer size. Values below 1 are clamped to 1 so every subscriber can hold at least one in-flight event.
func WithBroadcastBuffer(size int) BroadcasterOption {
	return func(c *broadcasterConfig) { c.buffer = size }
}

// NewBroadcaster creates a Broadcaster with the given options. Without WithBroadcastBuffer it uses DefaultBroadcastBuffer.
func NewBroadcaster[T any](opts ...BroadcasterOption) *Broadcaster[T] {
	cfg := broadcasterConfig{buffer: DefaultBroadcastBuffer}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.buffer < 1 {
		cfg.buffer = 1
	}
	return &Broadcaster[T]{buffer: cfg.buffer, done: make(chan struct{})}
}

// Buffer returns the effective per-subscriber buffer size.
func (b *Broadcaster[T]) Buffer() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ensureInit()
	return b.buffer
}

// ensureInit lazily initializes the fields a zero-value Broadcaster leaves unset, so it is safe to use without NewBroadcaster (mirroring the zero-value readiness of sync primitives). Callers must hold b.mu.
func (b *Broadcaster[T]) ensureInit() {
	if b.done == nil {
		b.done = make(chan struct{})
	}
	if b.buffer < 1 {
		b.buffer = DefaultBroadcastBuffer
	}
}

// SubscriberCount returns the number of currently live subscribers.
func (b *Broadcaster[T]) SubscriberCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subs)
}

// Subscribe registers a new subscriber and returns its receive-only event channel. The channel is closed — terminating any range over it — when ctx is canceled or the Broadcaster is closed. Subscribing with an already-canceled context, or to a closed Broadcaster, returns an already-closed channel without registering a subscriber or spawning a watcher goroutine.
func (b *Broadcaster[T]) Subscribe(ctx context.Context) <-chan T {
	if ctx.Err() != nil {
		ch := make(chan T)
		close(ch)
		return ch
	}

	b.mu.Lock()
	b.ensureInit()
	if b.closed {
		b.mu.Unlock()
		ch := make(chan T)
		close(ch)
		return ch
	}
	ch := make(chan T, b.buffer)
	sub := &subscriber[T]{ch: ch}
	b.subs = append(b.subs, sub)
	done := b.done
	b.mu.Unlock()

	go func() {
		select {
		case <-ctx.Done():
		case <-done:
		}
		b.remove(sub)
	}()

	return ch
}

// Broadcast delivers item to every live subscriber. Delivery to a full subscriber is dropped rather than blocked, so a slow subscriber never stalls the broadcaster or its peers. Broadcasting after Close is a no-op.
func (b *Broadcaster[T]) Broadcast(item T) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	for _, sub := range b.subs {
		select {
		case sub.ch <- item:
		default: // subscriber buffer full: drop the overflow event
		}
	}
}

// Close terminates the Broadcaster: every subscriber channel is closed and all subscriber goroutines are released. It is idempotent, and further Broadcast calls become no-ops.
func (b *Broadcaster[T]) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.ensureInit()
	b.closed = true
	close(b.done)
	for _, sub := range b.subs {
		if !sub.closed {
			sub.closed = true
			close(sub.ch)
		}
	}
	b.subs = nil
}

// remove unregisters sub and closes its channel exactly once. Broadcast and remove both hold b.mu, so a channel is never sent to after it is closed.
func (b *Broadcaster[T]) remove(sub *subscriber[T]) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if sub.closed {
		return
	}
	sub.closed = true
	for i, s := range b.subs {
		if s != sub {
			continue
		}
		copy(b.subs[i:], b.subs[i+1:])
		last := len(b.subs) - 1
		b.subs[last] = nil
		b.subs = b.subs[:last]
		break
	}
	close(sub.ch)
}
