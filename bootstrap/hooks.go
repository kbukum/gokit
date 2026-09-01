package bootstrap

import (
	"context"
	"fmt"

	"github.com/kbukum/gokit/hook"
)

// Hook is a lifecycle callback that runs during application startup or shutdown.
// Services register hooks to perform setup/teardown without bootstrap knowing about specific infrastructure.
type Hook func(ctx context.Context) error

// Lifecycle event types emitted by the application in deterministic phase order:
// configure → before_start → (start components) → after_start → ready →
// (run) → before_stop → (stop components) → after_stop.
var (
	EventConfigure   = hook.EventType("bootstrap:configure")
	EventBeforeStart = hook.EventType("bootstrap:before_start")
	EventAfterStart  = hook.EventType("bootstrap:after_start")
	EventReady       = hook.EventType("bootstrap:ready")
	EventBeforeStop  = hook.EventType("bootstrap:before_stop")
	EventAfterStop   = hook.EventType("bootstrap:after_stop")
)

// lifecycleEvent is a concrete hook.Event for bootstrap lifecycle events.
type lifecycleEvent struct {
	eventType hook.EventType
}

func (e lifecycleEvent) Type() hook.EventType { return e.eventType }

// registerFatal registers start-side hooks whose failure aborts the lifecycle. Their error
// is wrapped with hook.ErrFatalHook so dispatch short-circuits and startup returns the error.
func (a *App[C]) registerFatal(event hook.EventType, label string, hooks []Hook) {
	for _, h := range hooks {
		fn := h
		a.hooks.On(event, func(ctx context.Context, _ hook.Event) error {
			if err := fn(ctx); err != nil {
				return fmt.Errorf("%w: %s hook failed: %w", hook.ErrFatalHook, label, err)
			}
			return nil
		})
	}
}

// registerNonFatal registers stop-side hooks whose failure is collected and logged during
// shutdown but does not prevent the remaining teardown from running.
func (a *App[C]) registerNonFatal(event hook.EventType, hooks []Hook) {
	for _, h := range hooks {
		fn := h
		a.hooks.On(event, func(ctx context.Context, _ hook.Event) error {
			return fn(ctx)
		})
	}
}

// OnBeforeStart registers hooks that run before any component is started.
func (a *App[C]) OnBeforeStart(hooks ...Hook) {
	a.registerFatal(EventBeforeStart, "onBeforeStart", hooks)
}

// OnAfterStart registers hooks that run after all components are started
// but before the application is marked as ready.
func (a *App[C]) OnAfterStart(hooks ...Hook) {
	a.registerFatal(EventAfterStart, "onAfterStart", hooks)
}

// OnReady registers a hook that runs after the ready check completes, immediately before the
// application begins accepting traffic. The ready check is advisory — a failed check is
// logged and startup continues (degraded start), so this phase runs regardless of the
// check's outcome; inspect component health inside the hook if readiness must gate behavior.
func (a *App[C]) OnReady(hooks ...Hook) {
	a.registerFatal(EventReady, "onReady", hooks)
}

// OnBeforeStop registers hooks that run during graceful shutdown before components are stopped.
// Use this for cleanup tasks like draining connections or deregistering from service discovery.
func (a *App[C]) OnBeforeStop(hooks ...Hook) {
	a.registerNonFatal(EventBeforeStop, hooks)
}

// OnAfterStop registers hooks that run during graceful shutdown after all components are stopped.
func (a *App[C]) OnAfterStop(hooks ...Hook) {
	a.registerNonFatal(EventAfterStop, hooks)
}

// emitLifecycleHooks dispatches a lifecycle event and returns any error.
func (a *App[C]) emitLifecycleHooks(ctx context.Context, eventType hook.EventType) error {
	return a.hooks.Emit(ctx, lifecycleEvent{eventType: eventType})
}
