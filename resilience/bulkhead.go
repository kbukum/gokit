package resilience

import (
	"context"
	"sync/atomic"
	"time"

	apperr "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/util"
)

// Common bulkhead errors. These are typed AppErrors so callers can branch on
// the error code, while errors.Is still matches the sentinels. Capacity
// rejection is backpressure, so both classify as rate-limited/429 (matching the
// cross-kit contract) rather than service-unavailable.
var (
	ErrBulkheadFull    = apperr.New(apperr.ErrCodeRateLimited, "bulkhead is full")
	ErrBulkheadTimeout = apperr.New(apperr.ErrCodeRateLimited, "bulkhead wait timeout")
)

// BulkheadConfig configures a bulkhead.
type BulkheadConfig struct {
	// Name identifies this bulkhead for metrics/logging.
	Name string `json:"name,omitempty" yaml:"name" mapstructure:"name"`
	// MaxConcurrent is the maximum number of concurrent calls.
	MaxConcurrent int `json:"max_concurrent,omitempty" yaml:"max_concurrent" mapstructure:"max_concurrent"`
	// MaxWait is how long to wait for a slot. 0 means fail immediately.
	MaxWait time.Duration `json:"max_wait,omitempty" yaml:"max_wait" mapstructure:"max_wait"`
	// MaxQueue bounds waiting callers. Zero disables waiting, even when MaxWait is positive.
	MaxQueue int `json:"max_queue,omitempty" yaml:"max_queue" mapstructure:"max_queue"`
	// Clock injects elapsed-time scheduling; nil uses the monotonic runtime clock.
	Clock util.TimerClock `json:"-" yaml:"-" mapstructure:"-"`
	// OnReject is called when a request is rejected.
	OnReject func(name string) `json:"-" yaml:"-" mapstructure:"-"`
	// OnAcquire is called when a slot is acquired.
	OnAcquire func(name string) `json:"-" yaml:"-" mapstructure:"-"`
	// OnRelease is called when a slot is released.
	OnRelease func(name string) `json:"-" yaml:"-" mapstructure:"-"`
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
	config  BulkheadConfig
	sem     chan struct{}
	waiting atomic.Int64
}

// NewBulkhead creates a new bulkhead.
func NewBulkhead(config BulkheadConfig) (*Bulkhead, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if config.Clock == nil {
		config.Clock = util.MonotonicClock{}
	}

	return &Bulkhead{
		config: config,
		sem:    make(chan struct{}, config.MaxConcurrent),
	}, nil
}

// Execute runs the given function within the bulkhead. Returns ErrBulkheadFull
// or ErrBulkheadTimeout if no slot is available.
func (b *Bulkhead) Execute(ctx context.Context, fn func() error) error {
	if err := b.acquire(ctx); err != nil {
		return err
	}
	defer b.release()
	return fn()
}

// ExecuteWithResult runs a function that returns a value.
func ExecuteWithResult[T any](ctx context.Context, b *Bulkhead, fn func() (T, error)) (T, error) {
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
	if err := b.acquireSlot(ctx); err != nil {
		if b.config.OnReject != nil {
			b.config.OnReject(b.config.Name)
		}
		return err
	}
	if b.config.OnAcquire != nil {
		b.config.OnAcquire(b.config.Name)
	}
	return nil
}

func (b *Bulkhead) acquireSlot(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// Try immediate acquire
	select {
	case b.sem <- struct{}{}:
		return nil
	default:
	}

	// If no wait configured, fail immediately
	if b.config.MaxWait == 0 || b.config.MaxQueue == 0 {
		return ErrBulkheadFull
	}

	if !b.enqueue() {
		return ErrBulkheadFull
	}
	defer b.waiting.Add(-1)

	// Wait with timeout
	timer := b.config.Clock.NewTimer(b.config.MaxWait)
	defer timer.Stop()

	select {
	case b.sem <- struct{}{}:
		return nil
	case <-timer.C():
		return ErrBulkheadTimeout
	case <-ctx.Done():
		return ctx.Err()
	}
}

// enqueue admits the caller to the wait queue unless it is full.
func (b *Bulkhead) enqueue() bool {
	limit := int64(b.config.MaxQueue)
	for {
		current := b.waiting.Load()
		if current >= limit {
			return false
		}
		if b.waiting.CompareAndSwap(current, current+1) {
			return true
		}
	}
}

// release releases a slot back to the bulkhead.
func (b *Bulkhead) release() {
	<-b.sem
	if b.config.OnRelease != nil {
		b.config.OnRelease(b.config.Name)
	}
}

// Available returns the number of available slots.
func (b *Bulkhead) Available() int {
	return b.config.MaxConcurrent - len(b.sem)
}

// InUse returns the number of slots currently in use.
func (b *Bulkhead) InUse() int {
	return len(b.sem)
}

// Waiting returns the number of callers currently waiting for a slot.
func (b *Bulkhead) Waiting() int {
	return int(b.waiting.Load())
}

// MaxQueue returns the wait-queue bound; zero means no queue.
func (b *Bulkhead) MaxQueue() int {
	return b.config.MaxQueue
}

// MaxConcurrent returns the maximum concurrent calls allowed.
func (b *Bulkhead) MaxConcurrent() int {
	return b.config.MaxConcurrent
}
