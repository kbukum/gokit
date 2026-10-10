package resilience

import (
	"context"
	"sync"
)

// Acquire admits one operation through the policy's timeout, rate limiter, bulkhead and circuit breaker. Use the returned context for the entire operation and call finish after closing its resources. Cancellation signals teardown but does not release capacity before finish. Acquire never retries.
//
// The finish function records the supplied terminal outcome, releases capacity and cancels the owned context exactly once. Nil explicitly reports success; an operation stopped by cancellation or timeout must report its context error. Wrapped context.Canceled is neutral to breaker health; deadlines and other errors count as failures. Concurrent or repeated finish calls return the first outcome.
//
// A nil policy applies only caller cancellation; callers needing a finite budget must configure a timeout or supply a deadline. On admission failure, both the context and finish function are nil and no resources remain owned.
func (p *Policy) Acquire(ctx context.Context) (callCtx context.Context, finish func(error) error, err error) {
	var cancel context.CancelFunc
	_, hasDeadline := ctx.Deadline()
	if p != nil && p.Timeout > 0 && (p.timeoutMode != TimeoutIfUnset || !hasDeadline) {
		callCtx, cancel = context.WithTimeout(ctx, p.Timeout)
	} else {
		callCtx, cancel = context.WithCancel(ctx)
	}
	var (
		bulkhead   *Bulkhead
		breaker    *CircuitBreaker
		generation uint64
		once       sync.Once
		outcome    error
	)
	finish = func(err error) error {
		once.Do(func() {
			outcome = err
			if breaker != nil {
				breaker.recordResult(generation, err)
			}
			if bulkhead != nil {
				bulkhead.release()
			}
			cancel()
		})
		return outcome
	}
	defer func() {
		if err != nil {
			err = finish(err)
			callCtx, finish = nil, nil
		}
	}()
	if err := callCtx.Err(); err != nil {
		return callCtx, finish, err
	}

	if p == nil {
		return callCtx, finish, nil
	}
	p.init()
	if p.err != nil {
		return callCtx, finish, p.err
	}
	if p.rl != nil {
		if err := p.rl.Wait(callCtx); err != nil {
			return callCtx, finish, err
		}
	}
	if err := callCtx.Err(); err != nil {
		return callCtx, finish, err
	}
	if p.bh != nil {
		if err := p.bh.acquire(callCtx); err != nil {
			return callCtx, finish, err
		}
		bulkhead = p.bh
	}
	if err := callCtx.Err(); err != nil {
		return callCtx, finish, err
	}
	if p.cb != nil {
		var allowed bool
		generation, allowed = p.cb.allowRequest()
		if !allowed {
			return callCtx, finish, ErrCircuitOpen
		}
		breaker = p.cb
	}
	return callCtx, finish, nil
}
