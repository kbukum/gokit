package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Shutdown performs graceful shutdown using the supplied ctx. If ctx has no deadline,
// the configured gracefulTimeout is applied. If ctx has a deadline shorter than gracefulTimeout,
// ctx wins. Teardown runs once: repeated or concurrent calls wait for it and return the same
// outcome, nil or a *ShutdownError. Shutdown before Startup releases owned resources and ends
// the App's lifecycle.
//
// Shutdown also stops the running lifecycle: it cancels startup with [ErrShutdownRequested],
// wakes Run, and cancels the RunTask task, then waits for startup to roll back or the task to
// return before teardown. If that does not happen within the shutdown budget, Shutdown returns a
// *ShutdownError without recording it; teardown still runs when startup or the task returns, and
// a later Shutdown returns that outcome.
//
// Use when managing your own lifecycle (e.g. when not relying on signal handling via Run).
func (a *App[C]) Shutdown(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, a.gracefulTimeout)
	defer cancel()

	if active := a.requestShutdown(); active != nil {
		select {
		case <-active:
		case <-ctx.Done():
			return &ShutdownError{Cause: fmt.Errorf("wait for startup or task to return: %w", ctx.Err())}
		}
	}
	return a.shutdownWith(ctx)
}

// stop is invoked by the internal signal handler.
// It seeds shutdown with a fresh context bounded by gracefulTimeout because the original Run ctx is already canceled by the time we get here.
func (a *App[C]) stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), a.gracefulTimeout)
	defer cancel()
	return a.shutdownWith(ctx)
}

// shutdownWith runs teardown once, bounded by the shorter of the caller's deadline and
// gracefulTimeout, and returns the recorded outcome.
func (a *App[C]) shutdownWith(parent context.Context) error {
	a.shutdownOnce.Do(func() {
		a.lifecycleMu.Lock()
		a.used = true
		a.lifecycleMu.Unlock()
		if err := a.teardown(parent); err != nil {
			a.shutdownErr = &ShutdownError{Cause: err}
		}
		a.lifecycleMu.Lock()
		cancelLifecycle := a.cancelLifecycle
		a.lifecycleMu.Unlock()
		if cancelLifecycle != nil {
			cancelLifecycle(context.Canceled)
		}
	})
	return a.shutdownErr
}

func (a *App[C]) teardown(parent context.Context) error {
	a.state.Store(int32(stateDraining))
	a.Logger.InfoCtx(parent, "Shutting down application", map[string]any{
		"timeout": a.gracefulTimeout.String(),
	})

	ctx, cancel := context.WithTimeout(parent, a.gracefulTimeout)
	defer cancel()

	var shutdownErrs []error
	if err := a.Components.QuiesceAll(); err != nil {
		shutdownErrs = append(shutdownErrs, err)
	}

	// Phase: before_stop — hooks run before stopping components — collect all errors.
	deadline, _ := ctx.Deadline()
	hookCtx, hookCancel := context.WithTimeout(ctx, max(time.Until(deadline)/4, 0))
	if err := a.emitLifecycleHooks(hookCtx, EventBeforeStop); err != nil {
		a.Logger.ErrorCtx(ctx, "OnBeforeStop hook error", map[string]any{
			"error": err.Error(),
		})
		shutdownErrs = append(shutdownErrs, err)
	}
	hookCancel()

	// Drain work, release components and container resources, then stop telemetry/admin.
	if err := a.Components.Shutdown(ctx, a.Container.Close); err != nil {
		a.Logger.ErrorCtx(ctx, "Shutdown completed with errors", map[string]any{
			"error": err.Error(),
		})
		shutdownErrs = append(shutdownErrs, err)
	}

	// Phase: after_stop — components and container resources have been released.
	if err := a.emitLifecycleHooks(ctx, EventAfterStop); err != nil {
		a.Logger.ErrorCtx(ctx, "OnAfterStop hook error", map[string]any{
			"error": err.Error(),
		})
		shutdownErrs = append(shutdownErrs, err)
	}

	a.Logger.InfoCtx(ctx, "Application shutdown complete")

	// The owned logger is released last, after every phase that logs, within the same budget.
	if a.ownsLogger {
		if err := a.Logger.Shutdown(ctx); err != nil {
			shutdownErrs = append(shutdownErrs, fmt.Errorf("release logger: %w", err))
		}
	}
	return errors.Join(shutdownErrs...)
}
