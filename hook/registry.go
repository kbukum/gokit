package hook

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// Registry manages hook handlers and dispatches events.
// Handlers are executed sequentially in registration order. Non-fatal errors are
// aggregated and emitted as EventOnError observations. Fatal errors short-circuit
// dispatch when they wrap ErrFatalHook.
type Registry struct {
	mu       sync.RWMutex
	handlers map[EventType][]entry
	nextID   int
}

type entry struct {
	id      int
	handler Handler
}

// NewRegistry creates an empty hook registry.
func NewRegistry() *Registry {
	return &Registry{
		handlers: make(map[EventType][]entry),
	}
}

// On registers a handler for the given event type.
// Returns an unsubscribe function that removes the handler.
func (r *Registry) On(eventType EventType, h Handler) func() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.nextID++
	id := r.nextID
	r.handlers[eventType] = append(r.handlers[eventType], entry{id: id, handler: h})

	return func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		entries := r.handlers[eventType]
		for i, e := range entries {
			if e.id == id {
				r.handlers[eventType] = append(entries[:i], entries[i+1:]...)
				return
			}
		}
	}
}

// Emit dispatches an event to all registered handlers for its type.
// Panicking handlers are recovered and converted to non-fatal errors. Non-fatal
// errors do not stop dispatch; each is observed through EventOnError and the
// aggregate is returned. Fatal errors wrapping ErrFatalHook return immediately.
func (r *Registry) Emit(ctx context.Context, event Event) error {
	entries := r.entries(event.Type())
	var joined error

	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("%w: context canceled: %w", ErrFatalHook, err)
		}

		err := r.safeCall(ctx, e.handler, event)
		if err == nil {
			continue
		}
		if errors.Is(err, ErrFatalHook) {
			return err
		}
		joined = errors.Join(joined, err)
		if event.Type() != EventOnError {
			emitErr := r.emitError(ctx, event.Type(), err)
			if errors.Is(emitErr, ErrFatalHook) {
				return errors.Join(joined, emitErr)
			}
			joined = errors.Join(joined, emitErr)
		}
	}

	return joined
}

func (r *Registry) entries(eventType EventType) []entry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entries := make([]entry, len(r.handlers[eventType]))
	copy(entries, r.handlers[eventType])
	return entries
}

func (r *Registry) emitError(ctx context.Context, source EventType, err error) error {
	var joined error
	for _, e := range r.entries(EventOnError) {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("%w: context canceled: %w", ErrFatalHook, ctxErr)
		}
		handlerErr := r.safeCall(ctx, e.handler, ErrorEvent{Err: err, Source: source})
		if handlerErr == nil {
			continue
		}
		if errors.Is(handlerErr, ErrFatalHook) {
			return handlerErr
		}
		joined = errors.Join(joined, handlerErr)
	}
	return joined
}

// safeCall invokes a handler with panic recovery.
func (r *Registry) safeCall(ctx context.Context, h Handler, event Event) (err error) {
	defer func() {
		if rv := recover(); rv != nil {
			err = fmt.Errorf("hook handler panicked: %v", rv)
		}
	}()
	return h(ctx, event)
}

// HasHandlers returns true if any handlers are registered for the event type.
func (r *Registry) HasHandlers(eventType EventType) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.handlers[eventType]) > 0
}

// Clear removes all handlers for the given event type.
// If no event type is specified, clears all handlers.
func (r *Registry) Clear(eventTypes ...EventType) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(eventTypes) == 0 {
		r.handlers = make(map[EventType][]entry)
		return
	}
	for _, et := range eventTypes {
		delete(r.handlers, et)
	}
}
