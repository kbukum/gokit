package hook

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
}

// Continue returns a Result that lets execution proceed.
func Continue() Result {
	return Result{Action: ActionContinue}
}

// Abort returns a Result that stops execution.
func Abort(reason string) Result {
	return Result{Action: ActionAbort, Reason: reason}
}

// Modify returns a Result that replaces event data.
func Modify(data any) Result {
	return Result{Action: ActionModify, ModifiedData: data}
}

// Handler processes a hook event and returns a result.
type Handler func(Event) Result
