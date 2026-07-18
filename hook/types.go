package hook

import (
	"context"
	"errors"
)

// ErrFatalHook marks a hook error as fatal to the caller's flow.
var ErrFatalHook = errors.New("hook: fatal")

// EventType identifies the kind of hook event. Applications define their own EventType constants.
type EventType string

// EventOnError is emitted when a non-fatal hook handler returns an error.
const EventOnError EventType = "on_error"

// Event is the interface for all hook events.
// Applications define concrete event types that implement this interface.
type Event interface {
	// Type returns the event type identifier.
	Type() EventType
}

// ErrorEvent reports a non-fatal hook handler error.
type ErrorEvent struct {
	Err    error     `json:"-"`
	Source EventType `json:"source"`
}

// Type returns the canonical hook error event type.
func (ErrorEvent) Type() EventType { return EventOnError }

// Handler observes a hook event. Returning a non-nil error records the failure;
// only errors wrapping ErrFatalHook abort dispatch.
type Handler func(ctx context.Context, event Event) error
