// Package hook provides a lightweight, generic observe-only event system.
//
// It allows registering handlers for arbitrary event types. Handlers run sequentially in registration order. Non-fatal errors are aggregated and observed through the canonical on_error event; only errors wrapping ErrFatalHook abort dispatch.
//
// The hook module is domain-agnostic — applications define their own event types by implementing the Event interface.
//
// Usage:
//
// registry := hook.NewRegistry()
//
//	registry.On("my_event", func(ctx context.Context, e hook.Event) error {
//	   log.Printf("event: %s", e.Type())
//	   return nil
//	})
package hook
