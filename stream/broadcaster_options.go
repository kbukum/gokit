package stream

// broadcasterConfig holds the resolved construction-time settings for a Broadcaster.
type broadcasterConfig struct {
	buffer int
	onDrop func()
}

// BroadcasterOption configures a Broadcaster at construction time.
type BroadcasterOption func(*broadcasterConfig)

// WithBroadcastBuffer sets the per-subscriber buffer size. Values below 1 are clamped to 1
// so every subscriber can hold at least one in-flight event.
func WithBroadcastBuffer(size int) BroadcasterOption {
	return func(c *broadcasterConfig) { c.buffer = size }
}

// WithBroadcastOnDrop registers a hook invoked once for every event dropped because a
// subscriber's buffer was full. It observes backpressure-by-drop without changing it:
// delivery, buffering, and ordering are unaffected.
//
// The hook is called synchronously while the Broadcaster holds its internal lock, so it
// must be cheap, must not block, and must not re-enter the Broadcaster (Broadcast,
// Subscribe, Close, DroppedCount) — doing so deadlocks. Bridge it to a counter or metric
// on the consumer side rather than performing work inside it.
func WithBroadcastOnDrop(hook func()) BroadcasterOption {
	return func(c *broadcasterConfig) { c.onDrop = hook }
}
