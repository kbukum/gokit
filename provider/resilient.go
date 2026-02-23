package provider

import (
	"context"
	"errors"

	goerrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/resilience"
)

// WithResilience wraps a RequestResponse provider with resilience middleware.
// Execution chain: RateLimiter → Bulkhead → CircuitBreaker → Retry → Execute.
// Nil config fields are skipped. Empty config returns the provider unchanged.
func WithResilience[I, O any](p RequestResponse[I, O], cfg ResilienceConfig) RequestResponse[I, O] {
	if cfg.IsEmpty() {
		return p
	}
	return &resilientRR[I, O]{
		inner: p,
		state: BuildResilience(cfg),
	}
}

// WithStreamResilience wraps a Stream provider with resilience middleware.
// Resilience is applied to the Execute call that opens the stream.
// Individual Next() calls on the returned Iterator are NOT wrapped.
func WithStreamResilience[I, O any](p Stream[I, O], cfg ResilienceConfig) Stream[I, O] {
	if cfg.IsEmpty() {
		return p
	}
	return &resilientStream[I, O]{
		inner: p,
		state: BuildResilience(cfg),
	}
}

// WithSinkResilience wraps a Sink provider with resilience middleware.
// Execution chain: RateLimiter → Bulkhead → CircuitBreaker → Retry → Send.
func WithSinkResilience[I any](p Sink[I], cfg ResilienceConfig) Sink[I] {
	if cfg.IsEmpty() {
		return p
	}
	return &resilientSink[I]{
		inner: p,
		state: BuildResilience(cfg),
	}
}

// WithDuplexResilience wraps a Duplex provider with resilience middleware.
// Only CircuitBreaker and RateLimiter are applied to the Open call.
// Retry is not applied — persistent connections should reconnect at a higher level.
func WithDuplexResilience[I, O any](p Duplex[I, O], cfg ResilienceConfig) Duplex[I, O] {
	if cfg.IsEmpty() {
		return p
	}
	return &resilientDuplex[I, O]{
		inner: p,
		state: BuildResilience(cfg),
	}
}

// --- RequestResponse wrapper ---

type resilientRR[I, O any] struct {
	inner RequestResponse[I, O]
	state *ResilienceState
}

func (r *resilientRR[I, O]) Name() string                         { return r.inner.Name() }
func (r *resilientRR[I, O]) IsAvailable(ctx context.Context) bool { return r.inner.IsAvailable(ctx) }

func (r *resilientRR[I, O]) Execute(ctx context.Context, input I) (O, error) {
	return ExecuteWithResilience(ctx, r.state, func() (O, error) {
		return r.inner.Execute(ctx, input)
	})
}

// --- Stream wrapper ---

type resilientStream[I, O any] struct {
	inner Stream[I, O]
	state *ResilienceState
}

func (r *resilientStream[I, O]) Name() string                         { return r.inner.Name() }
func (r *resilientStream[I, O]) IsAvailable(ctx context.Context) bool { return r.inner.IsAvailable(ctx) }

func (r *resilientStream[I, O]) Execute(ctx context.Context, input I) (Iterator[O], error) {
	return ExecuteWithResilience(ctx, r.state, func() (Iterator[O], error) {
		return r.inner.Execute(ctx, input)
	})
}

// --- Sink wrapper ---

type resilientSink[I any] struct {
	inner Sink[I]
	state *ResilienceState
}

func (r *resilientSink[I]) Name() string                         { return r.inner.Name() }
func (r *resilientSink[I]) IsAvailable(ctx context.Context) bool { return r.inner.IsAvailable(ctx) }

func (r *resilientSink[I]) Send(ctx context.Context, input I) error {
	_, err := ExecuteWithResilience(ctx, r.state, func() (struct{}, error) {
		return struct{}{}, r.inner.Send(ctx, input)
	})
	return err
}

// --- Duplex wrapper ---

type resilientDuplex[I, O any] struct {
	inner Duplex[I, O]
	state *ResilienceState
}

func (r *resilientDuplex[I, O]) Name() string                         { return r.inner.Name() }
func (r *resilientDuplex[I, O]) IsAvailable(ctx context.Context) bool { return r.inner.IsAvailable(ctx) }

func (r *resilientDuplex[I, O]) Open(ctx context.Context) (DuplexStream[I, O], error) {
	// Duplex skips retry — reconnect should happen at a higher level.
	noRetry := &ResilienceState{
		cb: r.state.cb,
		rl: r.state.rl,
		bh: r.state.bh,
	}
	return ExecuteWithResilience(ctx, noRetry, func() (DuplexStream[I, O], error) {
		return r.inner.Open(ctx)
	})
}

// --- Core execution chain ---

// ExecuteWithResilience runs fn through the resilience chain:
// RateLimiter.Wait → Bulkhead → CircuitBreaker → Retry → fn.
// Exported so other packages (e.g., process) can reuse the chain.
// Resilience errors are wrapped as gokit AppError for consistency.
func ExecuteWithResilience[T any](ctx context.Context, s *ResilienceState, fn func() (T, error)) (T, error) {
	if s == nil {
		return fn()
	}

	// Layer 1: Rate limiter (wait for token)
	if s.rl != nil {
		if err := s.rl.Wait(ctx); err != nil {
			var zero T
			return zero, wrapResilienceError(err)
		}
	}

	// Build the innermost call: retry wrapping fn, or bare fn
	call := fn
	if s.retryCfg != nil {
		retryCfg := *s.retryCfg
		call = func() (T, error) {
			return resilience.Retry(ctx, retryCfg, fn)
		}
	}

	// Layer 2: Circuit breaker wrapping call
	if s.cb != nil {
		cbCall := call
		call = func() (T, error) {
			var result T
			var resultErr error
			cbErr := s.cb.Execute(func() error {
				result, resultErr = cbCall()
				return resultErr
			})
			if cbErr != nil && resultErr == nil {
				return result, wrapResilienceError(cbErr)
			}
			return result, resultErr
		}
	}

	// Layer 3: Bulkhead wrapping everything
	if s.bh != nil {
		bhCall := call
		result, err := resilience.ExecuteWithResult(ctx, s.bh, func() (T, error) {
			return bhCall()
		})
		if err != nil {
			return result, wrapResilienceError(err)
		}
		return result, nil
	}

	return call()
}

// wrapResilienceError converts resilience sentinel errors to gokit AppError
// for consistent error handling across the stack.
func wrapResilienceError(err error) error {
	if err == nil {
		return nil
	}

	// Already an AppError — return as-is
	if _, ok := goerrors.AsAppError(err); ok {
		return err
	}

	switch {
	case errors.Is(err, resilience.ErrCircuitOpen):
		return goerrors.ServiceUnavailable("provider").WithCause(err)
	case errors.Is(err, resilience.ErrRateLimited):
		return goerrors.RateLimited().WithCause(err)
	case errors.Is(err, resilience.ErrBulkheadFull), errors.Is(err, resilience.ErrBulkheadTimeout):
		return goerrors.ServiceUnavailable("provider").
			WithCause(err).
			WithDetail("reason", "concurrency limit reached")
	case errors.Is(err, context.Canceled):
		return goerrors.Timeout("request canceled").WithCause(err)
	case errors.Is(err, context.DeadlineExceeded):
		return goerrors.Timeout("deadline exceeded").WithCause(err)
	default:
		return err
	}
}
