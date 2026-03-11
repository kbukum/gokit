package stateful

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Accumulator accumulates values of type V with configurable flushing.
// Thread-safe for concurrent Append operations.
type Accumulator[V any] struct {
	store     Store[V]
	config    Config[V]
	measurer  Measurer[V]
	created   time.Time
	lastFlush time.Time
	mu        sync.RWMutex
}

// NewAccumulator creates a new accumulator with the given store and configuration.
// If no Measurer is provided in options, CountMeasurer is used by default.
func NewAccumulator[V any](store Store[V], config Config[V], opts ...Option[V]) *Accumulator[V] {
	acc := &Accumulator[V]{
		store:    store,
		config:   config,
		measurer: CountMeasurer[V](), // default
		created:  time.Now(),
	}

	// Apply options
	for _, opt := range opts {
		opt(acc)
	}

	return acc
}

// Option is a functional option for configuring an Accumulator.
type Option[V any] func(*Accumulator[V])

// WithMeasurer sets a custom measurer for the accumulator.
func WithMeasurer[V any](m Measurer[V]) Option[V] {
	return func(acc *Accumulator[V]) {
		acc.measurer = m
	}
}

// Append adds a value to the accumulator. If MaxSize is configured and exceeded,
// oldest values are evicted (FIFO). After appending, triggers are checked and
// the accumulator may flush automatically.
//
// If KeepAlive is enabled, this resets the TTL.
func (a *Accumulator[V]) Append(ctx context.Context, value V) error {
	// Handle keep-alive
	if a.config.KeepAlive {
		if err := a.store.Touch(ctx); err != nil {
			return fmt.Errorf("accumulator touch: %w", err)
		}
	}

	// Append with FIFO if MaxSize configured
	if a.config.MaxSize > 0 {
		evicted, err := a.store.AppendFIFO(ctx, value, a.config.MaxSize)
		if err != nil {
			return fmt.Errorf("accumulator append fifo: %w", err)
		}
		if len(evicted) > 0 && a.config.OnEvict != nil {
			a.config.OnEvict(ctx, evicted)
		}
	} else {
		if err := a.store.Append(ctx, value); err != nil {
			return fmt.Errorf("accumulator append: %w", err)
		}
	}

	// Check triggers and flush if needed
	return a.checkAndFlush(ctx)
}

// Flush manually flushes the accumulator, returning all values.
// This bypasses trigger checks and rate limiting.
func (a *Accumulator[V]) Flush(ctx context.Context) ([]V, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	values, err := a.store.Flush(ctx)
	if err != nil {
		return nil, fmt.Errorf("accumulator flush: %w", err)
	}

	a.lastFlush = time.Now()

	if len(values) > 0 && a.config.OnFlush != nil {
		if err := a.config.OnFlush(ctx, values); err != nil {
			// Return values but also return the error
			return values, fmt.Errorf("accumulator flush handler: %w", err)
		}
	}

	return values, nil
}

// Size returns the current number of values in the accumulator.
func (a *Accumulator[V]) Size(ctx context.Context) (int, error) {
	return a.store.Size(ctx)
}

// Measure returns the current measurement of accumulated values using the configured Measurer.
func (a *Accumulator[V]) Measure(ctx context.Context) (int, error) {
	values, err := a.store.Get(ctx)
	if err != nil {
		return 0, fmt.Errorf("accumulator measure: %w", err)
	}
	return a.measurer.Measure(ctx, values), nil
}

// IsExpired returns true if the accumulator has expired based on TTL and KeepAlive settings.
func (a *Accumulator[V]) IsExpired(ctx context.Context) bool {
	if a.config.TTL == 0 {
		return false // Never expires
	}

	// If KeepAlive is false, use absolute TTL from creation
	if !a.config.KeepAlive {
		return time.Since(a.created) > a.config.TTL
	}

	// KeepAlive enabled - check last activity
	lastActivity, err := a.store.LastActivity(ctx)
	if err != nil {
		if a.config.OnError != nil {
			a.config.OnError(err)
		}
		return false
	}

	if lastActivity.IsZero() {
		// Never active - use creation time
		return time.Since(a.created) > a.config.TTL
	}

	return time.Since(lastActivity) > a.config.TTL
}

// Touch updates the last activity time. Only useful when KeepAlive is enabled.
func (a *Accumulator[V]) Touch(ctx context.Context) error {
	return a.store.Touch(ctx)
}

// Close releases resources held by the accumulator.
func (a *Accumulator[V]) Close() error {
	return a.store.Close()
}

// checkAndFlush checks triggers and flushes if conditions are met.
// Respects MinInterval rate limiting.
func (a *Accumulator[V]) checkAndFlush(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Check rate limiting
	if a.config.MinInterval > 0 && !a.lastFlush.IsZero() {
		if time.Since(a.lastFlush) < a.config.MinInterval {
			return nil // Too soon
		}
	}

	// Check if minimum size requirement met
	if a.config.MinSize > 0 {
		values, err := a.store.Get(ctx)
		if err != nil {
			return fmt.Errorf("accumulator check flush get: %w", err)
		}
		measured := a.measurer.Measure(ctx, values)
		if measured < a.config.MinSize {
			return nil // Not enough yet
		}
	}

	// Check triggers
	shouldFlush := a.evaluateTriggers(ctx)
	if !shouldFlush {
		return nil
	}

	// Flush
	values, err := a.store.Flush(ctx)
	if err != nil {
		return fmt.Errorf("accumulator auto-flush: %w", err)
	}

	a.lastFlush = time.Now()

	if len(values) > 0 && a.config.OnFlush != nil {
		if err := a.config.OnFlush(ctx, values); err != nil {
			return fmt.Errorf("accumulator flush handler: %w", err)
		}
	}

	return nil
}

// evaluateTriggers evaluates all triggers according to TriggerMode.
// Caller must hold mu.Lock.
func (a *Accumulator[V]) evaluateTriggers(ctx context.Context) bool {
	if len(a.config.Triggers) == 0 {
		return false
	}

	if a.config.TriggerMode == TriggerAll {
		// ALL triggers must fire (AND logic)
		for _, trigger := range a.config.Triggers {
			if !trigger.ShouldFlush(ctx, a) {
				return false
			}
		}
		return true
	}

	// ANY trigger fires (OR logic) - default
	for _, trigger := range a.config.Triggers {
		if trigger.ShouldFlush(ctx, a) {
			return true
		}
	}
	return false
}
