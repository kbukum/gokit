package hook

import (
	"context"
	"fmt"
	"sync"
)

// Registry manages hook handlers and dispatches events.
// Handlers are executed sequentially in registration order.
// An Abort result short-circuits remaining handlers.
// Modify results chain — each handler sees the previous modification.
// Panicking handlers are recovered and converted to errors without
// disrupting dispatch to subsequent handlers.
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
// Handlers run sequentially. First Abort short-circuits.
// Returns the merged result (last non-Continue result wins).
// Panicking handlers are recovered: the panic is converted to an error
// on the Result and dispatch continues to the next handler.
func (r *Registry) Emit(ctx context.Context, event Event) Result {
	r.mu.RLock()
	entries := make([]entry, len(r.handlers[event.Type()]))
	copy(entries, r.handlers[event.Type()])
	r.mu.RUnlock()

	merged := Continue()

	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return Result{Action: ActionAbort, Reason: "context canceled", Err: err}
		}

		result := r.safeCall(ctx, e.handler, event)
		switch result.Action {
		case ActionAbort:
			return result
		case ActionModify:
			merged = result
		case ActionContinue:
			if result.Err != nil {
				merged.Err = result.Err
			}
		}
	}

	return merged
}

// safeCall invokes a handler with panic recovery.
func (r *Registry) safeCall(ctx context.Context, h Handler, event Event) (result Result) {
	defer func() {
		if rv := recover(); rv != nil {
			result = Result{
				Action: ActionContinue,
				Err:    fmt.Errorf("hook handler panicked: %v", rv),
			}
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
