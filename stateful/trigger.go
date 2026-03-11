package stateful

import (
	"context"
	"time"
)

// Trigger defines when an accumulator should flush.
// Multiple triggers can be combined using TriggerMode (ANY or ALL).
type Trigger[V any] interface {
	// ShouldFlush returns true if this trigger condition is met.
	ShouldFlush(ctx context.Context, acc *Accumulator[V]) bool

	// Name returns a human-readable name for this trigger (for logging/debugging).
	Name() string
}

// TimeTrigger creates a trigger that fires after the specified duration since last flush.
func TimeTrigger[V any](d time.Duration) Trigger[V] {
	return &timeTrigger[V]{duration: d}
}

type timeTrigger[V any] struct {
	duration time.Duration
}

func (t *timeTrigger[V]) Name() string {
	return "time:" + t.duration.String()
}

func (t *timeTrigger[V]) ShouldFlush(ctx context.Context, acc *Accumulator[V]) bool {
	// Don't lock here - caller already holds lock
	if acc.lastFlush.IsZero() {
		// Never flushed - use creation time
		return time.Since(acc.created) >= t.duration
	}
	return time.Since(acc.lastFlush) >= t.duration
}

// SizeTrigger creates a trigger that fires when measured size >= threshold.
// Uses the accumulator's configured Measurer.
func SizeTrigger[V any](threshold int) Trigger[V] {
	return &sizeTrigger[V]{threshold: threshold}
}

type sizeTrigger[V any] struct {
	threshold int
}

func (t *sizeTrigger[V]) Name() string {
	return "size:>=" + string(rune(t.threshold))
}

func (t *sizeTrigger[V]) ShouldFlush(ctx context.Context, acc *Accumulator[V]) bool {
	size, err := acc.Measure(ctx)
	if err != nil {
		if acc.config.OnError != nil {
			acc.config.OnError(err)
		}
		return false
	}
	return size >= t.threshold
}

// CustomTrigger creates a trigger with custom logic.
func CustomTrigger[V any](name string, fn func(context.Context, *Accumulator[V]) bool) Trigger[V] {
	return &customTrigger[V]{
		name: name,
		fn:   fn,
	}
}

type customTrigger[V any] struct {
	name string
	fn   func(context.Context, *Accumulator[V]) bool
}

func (t *customTrigger[V]) Name() string {
	return "custom:" + t.name
}

func (t *customTrigger[V]) ShouldFlush(ctx context.Context, acc *Accumulator[V]) bool {
	return t.fn(ctx, acc)
}
