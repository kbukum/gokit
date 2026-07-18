package worker

import (
	"context"
	"time"
)

// Middleware wraps a Handler to add cross-cutting behavior.
type Middleware[I, O any] func(Handler[I, O]) Handler[I, O]

// Chain composes multiple middlewares into one. Middlewares are applied in order:
// the first middleware is outermost (executes first on the way in, last on the way out).
//
// Chain(a, b, c)(handler) is equivalent to a(b(c(handler))).
func Chain[I, O any](middlewares ...Middleware[I, O]) Middleware[I, O] {
	return func(inner Handler[I, O]) Handler[I, O] {
		for i := len(middlewares) - 1; i >= 0; i-- {
			inner = middlewares[i](inner)
		}
		return inner
	}
}

// WithTimeout returns a Middleware that enforces a deadline on each Handle call.
func WithTimeout[I, O any](d time.Duration) Middleware[I, O] {
	return func(inner Handler[I, O]) Handler[I, O] {
		return HandlerFunc[I, O](func(ctx context.Context, task I, emit func(Event[O])) error {
			ctx, cancel := context.WithTimeout(ctx, d)
			defer cancel()
			return inner.Handle(ctx, task, emit)
		})
	}
}

// WithRecovery returns a Middleware that recovers from panics and converts them to errors.
func WithRecovery[I, O any]() Middleware[I, O] {
	return func(inner Handler[I, O]) Handler[I, O] {
		return HandlerFunc[I, O](func(ctx context.Context, task I, emit func(Event[O])) (err error) {
			defer func() {
				if r := recover(); r != nil {
					switch v := r.(type) {
					case error:
						err = v
					default:
						err = &PanicError{Value: v}
					}
				}
			}()
			return inner.Handle(ctx, task, emit)
		})
	}
}

// PanicError wraps a recovered panic value as an error.
type PanicError struct {
	Value any
}

func (e *PanicError) Error() string {
	return "worker: panic recovered"
}
