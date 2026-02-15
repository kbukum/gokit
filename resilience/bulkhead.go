package resilience

import (
	"context"
	"errors"
	"time"
)

// Common bulkhead errors.
var (
	ErrBulkheadFull    = errors.New("bulkhead is full")
	ErrBulkheadTimeout = errors.New("bulkhead wait timeout")
)

// BulkheadConfig configures a bulkhead.
type BulkheadConfig struct {
	// Name identifies this bulkhead for metrics/logging.
	Name string
	// MaxConcurrent is the maximum number of concurrent calls.
	MaxConcurrent int
	// MaxWait is how long to wait for a slot. 0 means fail immediately.
	MaxWait time.Duration
	// OnReject is called when a request is rejected.
	OnReject func(name string)
	// OnAcquire is called when a slot is acquired.
	OnAcquire func(name string)
	// OnRelease is called when a slot is released.
	OnRelease func(name string)
}

// DefaultBulkheadConfig returns sensible defaults.
func DefaultBulkheadConfig(name string) BulkheadConfig {
	return BulkheadConfig{
		Name:          name,
		MaxConcurrent: 10,
		MaxWait:       0, // Fail immediately if full
	}
}

// Bulkhead implements the bulkhead pattern for concurrency limiting.
// It isolates components to prevent cascading failures.
type Bulkhead struct {
	config BulkheadConfig
	sem    chan struct{}
}

// NewBulkhead creates a new bulkhead.
func NewBulkhead(config BulkheadConfig) *Bulkhead {
	if config.MaxConcurrent <= 0 {
		config.MaxConcurrent = 10
	}

	return &Bulkhead{
		config: config,
		sem:    make(chan struct{}, config.MaxConcurrent),
	}
}

// Execute runs the given function within the bulkhead.
// Returns ErrBulkheadFull or ErrBulkheadTimeout if no slot is available.
func (b *Bulkhead) Execute(ctx context.Context, fn func() error) error {
	if err := b.acquire(ctx); err != nil {
		if b.config.OnReject != nil {
			b.config.OnReject(b.config.Name)
		}
		return err
	}

	if b.config.OnAcquire != nil {
		b.config.OnAcquire(b.config.Name)
	}

	defer func() {
		b.release()
		if b.config.OnRelease != nil {
			b.config.OnRelease(b.config.Name)
		}
	}()

	return fn()
}

// ExecuteWithResult runs a function that returns a value.
func ExecuteWithResult[T any](b *Bulkhead, ctx context.Context, fn func() (T, error)) (T, error) {
	var result T
	err := b.Execute(ctx, func() error {
		var fnErr error
		result, fnErr = fn()
		return fnErr
	})
	return result, err
}

// acquire tries to acquire a slot in the bulkhead.
func (b *Bulkhead) acquire(ctx context.Context) error {
	// Try immediate acquire
	select {
	case b.sem <- struct{}{}:
		return nil
	default:
	}

	// If no wait configured, fail immediately
	if b.config.MaxWait <= 0 {
		return ErrBulkheadFull
	}

	// Wait with timeout
	timer := time.NewTimer(b.config.MaxWait)
	defer timer.Stop()

	select {
	case b.sem <- struct{}{}:
		return nil
	case <-timer.C:
		return ErrBulkheadTimeout
	case <-ctx.Done():
		return ctx.Err()
	}
}

// release releases a slot back to the bulkhead.
func (b *Bulkhead) release() {
	<-b.sem
}

// Available returns the number of available slots.
func (b *Bulkhead) Available() int {
	return b.config.MaxConcurrent - len(b.sem)
}

// InUse returns the number of slots currently in use.
func (b *Bulkhead) InUse() int {
	return len(b.sem)
}

// MaxConcurrent returns the maximum concurrent calls allowed.
func (b *Bulkhead) MaxConcurrent() int {
	return b.config.MaxConcurrent
}
