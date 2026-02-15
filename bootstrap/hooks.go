package bootstrap

import (
	"context"
	"fmt"
)

// Hook is a lifecycle callback that runs during application startup or shutdown.
// Services register hooks to perform setup/teardown without bootstrap knowing
// about specific infrastructure.
type Hook func(ctx context.Context) error

// OnStart registers a hook that runs after all components are started (Phase 1)
// but before the application is marked as ready.
func (a *App) OnStart(hooks ...Hook) {
	a.onStart = append(a.onStart, hooks...)
}

// OnReady registers a hook that runs after the application passes its ready check
// and is about to begin accepting traffic.
func (a *App) OnReady(hooks ...Hook) {
	a.onReady = append(a.onReady, hooks...)
}

// OnStop registers a hook that runs during graceful shutdown before components
// are stopped. Use this for cleanup tasks like draining connections or
// deregistering from service discovery.
func (a *App) OnStop(hooks ...Hook) {
	a.onStop = append(a.onStop, hooks...)
}

// runHooks executes a slice of hooks sequentially, returning the first error.
func runHooks(ctx context.Context, hooks []Hook) error {
	for i, h := range hooks {
		if err := h(ctx); err != nil {
			return fmt.Errorf("hook %d failed: %w", i, err)
		}
	}
	return nil
}
