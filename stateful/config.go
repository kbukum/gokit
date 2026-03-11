package stateful

import (
	"context"
	"time"
)

// Config defines accumulator behavior.
type Config[V any] struct {
	// TTL is the time-to-live before expiration. Zero means never expire.
	// If KeepAlive is true, TTL resets on each Append/Touch.
	// If KeepAlive is false, TTL is absolute from creation/last flush.
	TTL time.Duration

	// KeepAlive enables sliding window TTL. When true, TTL resets on each
	// Append or Touch operation. When false, TTL is absolute.
	KeepAlive bool

	// MinSize is the minimum size (measured by Measurer) to trigger flush.
	// Zero means no minimum. Used by SizeTrigger.
	MinSize int

	// MaxSize is the maximum size (measured by Measurer) before FIFO eviction.
	// Zero means unbounded (no eviction). When exceeded, oldest values are evicted
	// and OnEvict callback is called.
	MaxSize int

	// MinInterval is the minimum time between flushes. Prevents too-frequent
	// processing even if triggers fire. Zero means no rate limiting.
	MinInterval time.Duration

	// Triggers define when to flush. Can be time-based, size-based, or custom.
	// Multiple triggers are evaluated according to TriggerMode (ANY or ALL).
	Triggers []Trigger[V]

	// TriggerMode defines how multiple triggers are combined.
	// TriggerAny (default): flush if ANY trigger fires (OR logic)
	// TriggerAll: flush only if ALL triggers fire (AND logic)
	TriggerMode TriggerMode

	// OnFlush is called when the accumulator flushes values.
	// Receives the flushed values and should return nil on success.
	OnFlush FlushHandler[V]

	// OnEvict is called when values are evicted due to MaxSize FIFO.
	// Optional - can be nil.
	OnEvict EvictHandler[V]

	// OnExpire is called when the accumulator expires due to TTL.
	// Receives the key/ID if managed by Manager. Optional - can be nil.
	OnExpire ExpireHandler

	// OnError is called when an error occurs during background operations
	// (trigger checks, expiration checks). Optional - can be nil.
	// If nil, errors are silently ignored.
	OnError ErrorHandler
}

// TriggerMode defines how multiple triggers are evaluated.
type TriggerMode int

const (
	// TriggerAny flushes if ANY trigger fires (OR logic). This is the default.
	TriggerAny TriggerMode = iota

	// TriggerAll flushes only if ALL triggers fire (AND logic).
	TriggerAll
)

// FlushHandler is called when values are flushed.
type FlushHandler[V any] func(ctx context.Context, values []V) error

// EvictHandler is called when values are evicted due to FIFO.
type EvictHandler[V any] func(ctx context.Context, evicted []V)

// ExpireHandler is called when an accumulator expires.
type ExpireHandler func(ctx context.Context, key string)

// ErrorHandler is called when background errors occur.
type ErrorHandler func(err error)
