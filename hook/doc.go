// Package hook provides a lightweight, generic event system for lifecycle hooks.
//
// It allows registering handlers for arbitrary event types. Handlers run
// sequentially in registration order, with short-circuit on Abort and
// chained Modify results. Panicking handlers are recovered and converted
// to errors without disrupting subsequent handler dispatch.
//
// The hook module is domain-agnostic — applications define their own event
// types by implementing the Event interface. For example, the agent module
// defines PreToolCall, PreLLMCall, TurnStart, etc.
//
// Usage:
//
//	registry := hook.NewRegistry()
//	registry.On("my_event", func(ctx context.Context, e hook.Event) hook.Result {
//	    log.Printf("event: %s", e.Type())
//	    return hook.Continue()
//	})
package hook
