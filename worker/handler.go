package worker

import "context"

// Handler processes a task and emits events during execution. The handler MUST check ctx.Done() for cooperative cancellation.
type Handler[I, O any] interface {
	Handle(ctx context.Context, task I, emit func(Event[O])) error
}

// HandlerFunc is an adapter to use ordinary functions as Handlers.
type HandlerFunc[I, O any] func(ctx context.Context, task I, emit func(Event[O])) error

func (f HandlerFunc[I, O]) Handle(ctx context.Context, task I, emit func(Event[O])) error {
	return f(ctx, task, emit)
}
