package hook

import "context"

// EventType identifies the kind of hook event.
// Applications define their own EventType constants.
type EventType string

// Event is the interface for all hook events.
// Applications define concrete event types that implement this interface.
type Event interface {
	// Type returns the event type identifier.
	Type() EventType
}

// --- Hook Result ---

// Action determines how the caller proceeds after a hook.
type Action int

const (
	// ActionContinue lets execution proceed normally.
	ActionContinue Action = iota
	// ActionAbort stops execution with an optional reason.
	ActionAbort
	// ActionModify lets the handler modify data before proceeding.
	ActionModify
)

// Result is returned by hook handlers to control execution flow.
type Result struct {
	// Action determines whether to continue, abort, or modify.
	Action Action
	// ModifiedData carries replacement data when Action is Modify.
	ModifiedData any
	// Reason explains why execution was aborted (Action == Abort).
	Reason string
	// Err reports an error from the handler without aborting dispatch.
	Err error
}

// Continue returns a Result that lets execution proceed.
func Continue() Result {
	return Result{Action: ActionContinue}
}

// ContinueWithError returns a Result that lets execution proceed but records an error.
func ContinueWithError(err error) Result {
	return Result{Action: ActionContinue, Err: err}
}

// Abort returns a Result that stops execution.
func Abort(reason string) Result {
	return Result{Action: ActionAbort, Reason: reason}
}

// AbortWithError returns a Result that stops execution with an error.
func AbortWithError(reason string, err error) Result {
	return Result{Action: ActionAbort, Reason: reason, Err: err}
}

// Modify returns a Result that replaces event data.
func Modify(data any) Result {
	return Result{Action: ActionModify, ModifiedData: data}
}

// Handler processes a hook event and returns a result.
// The context carries deadlines, cancellation, and request-scoped values.
type Handler func(ctx context.Context, event Event) Result
