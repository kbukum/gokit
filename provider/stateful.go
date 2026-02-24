package provider

import (
	"context"
	"time"
)

// Stateful wraps a RequestResponse provider with automatic state
// load/save around each Execute call.
//
// Before Execute: loads state from store, calls inject to enrich input.
// After Execute: calls extract to derive updated state from output, saves to store.
//
// If the store returns nil (first call for a key), inject receives nil —
// the consumer's inject function handles initialization.
//
// Stateful implements RequestResponse[I, O] so it composes with
// WithResilience and other middleware.
type Stateful[I, O, C any] struct {
	inner   RequestResponse[I, O]
	store   ContextStore[C]
	keyFunc func(I) string
	inject  func(I, *C) I
	extract func(I, O) *C
	ttl     time.Duration
}

// StatefulConfig holds configuration for creating a Stateful wrapper.
type StatefulConfig[I, O, C any] struct {
	// Inner is the wrapped provider.
	Inner RequestResponse[I, O]
	// Store provides state persistence.
	Store ContextStore[C]
	// KeyFunc derives the state key from input.
	KeyFunc func(I) string
	// Inject enriches input with loaded state. Receives nil state on first call.
	Inject func(I, *C) I
	// Extract derives updated state from input and output for persistence.
	Extract func(I, O) *C
	// TTL is the time-to-live for persisted state. Zero means no expiration.
	TTL time.Duration
}

// NewStateful creates a Stateful wrapper from configuration.
func NewStateful[I, O, C any](cfg StatefulConfig[I, O, C]) *Stateful[I, O, C] {
	return &Stateful[I, O, C]{
		inner:   cfg.Inner,
		store:   cfg.Store,
		keyFunc: cfg.KeyFunc,
		inject:  cfg.Inject,
		extract: cfg.Extract,
		ttl:     cfg.TTL,
	}
}

// Name delegates to the inner provider.
func (s *Stateful[I, O, C]) Name() string { return s.inner.Name() }

// IsAvailable delegates to the inner provider.
func (s *Stateful[I, O, C]) IsAvailable(ctx context.Context) bool { return s.inner.IsAvailable(ctx) }

// Execute loads state, injects it into input, executes inner, extracts state, and saves.
func (s *Stateful[I, O, C]) Execute(ctx context.Context, input I) (O, error) {
	var zero O

	// Derive the state key
	key := s.keyFunc(input)

	// Load existing state (nil on first call)
	state, err := s.store.Load(ctx, key)
	if err != nil {
		return zero, err
	}

	// Inject state into input
	enriched := s.inject(input, state)

	// Execute the inner provider
	output, err := s.inner.Execute(ctx, enriched)
	if err != nil {
		return zero, err
	}

	// Extract updated state from output
	newState := s.extract(enriched, output)

	// Save state (if extract returned non-nil)
	if newState != nil {
		if err := s.store.Save(ctx, key, newState, s.ttl); err != nil {
			return zero, err
		}
	}

	return output, nil
}

// compile-time interface check
var _ RequestResponse[any, any] = (*Stateful[any, any, any])(nil)
