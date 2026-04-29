package bootstrap

import (
	"context"
	"errors"

	"github.com/kbukum/gokit/hook"
)

// Hook is a lifecycle callback that runs during application startup or shutdown.
// Services register hooks to perform setup/teardown without bootstrap knowing
// about specific infrastructure.
type Hook func(ctx context.Context) error

// Lifecycle event types used with the hook registry.
var (
	EventStart = hook.EventType("bootstrap:start")
	EventReady = hook.EventType("bootstrap:ready")
	EventStop  = hook.EventType("bootstrap:stop")
)

// lifecycleEvent is a concrete hook.Event for bootstrap lifecycle events.
type lifecycleEvent struct {
	eventType hook.EventType
}

func (e lifecycleEvent) Type() hook.EventType { return e.eventType }

// OnStart registers a hook that runs after all components are started
// but before the application is marked as ready.
func (a *App[C]) OnStart(hooks ...Hook) {
	for _, h := range hooks {
		fn := h
		a.hooks.On(EventStart, func(ctx context.Context, _ hook.Event) hook.Result {
			if err := fn(ctx); err != nil {
				return hook.AbortWithError("onStart hook failed", err)
			}
			return hook.Continue()
		})
	}
}

// OnReady registers a hook that runs after the application passes its ready check
// and is about to begin accepting traffic.
func (a *App[C]) OnReady(hooks ...Hook) {
	for _, h := range hooks {
		fn := h
		a.hooks.On(EventReady, func(ctx context.Context, _ hook.Event) hook.Result {
			if err := fn(ctx); err != nil {
				return hook.AbortWithError("onReady hook failed", err)
			}
			return hook.Continue()
		})
	}
}

// OnStop registers a hook that runs during graceful shutdown before components
// are stopped. Use this for cleanup tasks like draining connections or
// deregistering from service discovery.
func (a *App[C]) OnStop(hooks ...Hook) {
	for _, h := range hooks {
		fn := h
		a.hooks.On(EventStop, func(ctx context.Context, _ hook.Event) hook.Result {
			if err := fn(ctx); err != nil {
				return hook.ContinueWithError(err)
			}
			return hook.Continue()
		})
	}
}

// emitLifecycleHooks dispatches a lifecycle event and returns any error.
func (a *App[C]) emitLifecycleHooks(ctx context.Context, eventType hook.EventType) error {
	result := a.hooks.Emit(ctx, lifecycleEvent{eventType: eventType})
	var errs []error
	if result.Err != nil {
		errs = append(errs, result.Err)
	}
	if result.Action == hook.ActionAbort && result.Reason != "" && result.Err == nil {
		errs = append(errs, errors.New(result.Reason))
	}
	return errors.Join(errs...)
}
