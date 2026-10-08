package bootstrap

import (
	"context"
	"errors"
	"time"
)

// startupPhase is one fatal step of the startup sequence.
type startupPhase struct {
	phase Phase
	run   func(context.Context) error
}

// startup performs the initialization sequence shared by Run, RunTask, and Startup. Any fatal
// error rolls back through the full shutdown sequence (stop hooks, component teardown, DI
// container close, owned logger release) before returning a *StartupError, so a failed startup
// never leaves components or container resources running.
//
// Phases run on a lifecycle context derived from ctx. Shutdown cancels it with
// ErrShutdownRequested and waits for the lifecycle to become inactive; startup then rolls back
// after the running phase instead of creating more resources. With holdActive, a successful
// startup leaves the lifecycle active for the caller to end with endActive.
func (a *App[C]) startup(ctx context.Context, holdActive bool) (_ context.Context, err error) {
	ctx, err = a.beginLifecycle(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { a.endStartup(holdActive && err == nil) }()
	start := time.Now()

	a.Logger.InfoCtx(ctx, "Starting application", map[string]any{
		"name":    a.Name,
		"version": a.Version,
	})

	// Configure registers the command's own components and may add modules and bindings; modules
	// then register so their components start after those and before listeners; everything starts
	// in one StartAll pass; after_start hooks run before the ready phase.
	phases := []startupPhase{
		{PhaseConfigure, func(ctx context.Context) error { return a.emitLifecycleHooks(ctx, EventConfigure) }},
		{PhaseModules, a.wireModules},
		{PhaseBeforeStart, func(ctx context.Context) error { return a.emitLifecycleHooks(ctx, EventBeforeStart) }},
		{PhaseStart, a.Components.StartAll},
		{PhaseAfterStart, func(ctx context.Context) error { return a.emitLifecycleHooks(ctx, EventAfterStart) }},
		{PhaseReady, a.ready},
	}
	for _, p := range phases {
		if err := p.run(ctx); err != nil {
			return nil, a.abortStartup(ctx, p.phase, err)
		}
		// A phase can succeed after its context ends, for example when it does no blocking work;
		// startup must still stop rather than run later phases or the task on a canceled context.
		if ctx.Err() != nil {
			return nil, a.abortStartup(ctx, p.phase, ctx.Err())
		}
	}

	a.Summary.SetStartupDuration(time.Since(start))
	a.DisplaySummary(ctx)
	if ctx.Err() != nil {
		return nil, a.abortStartup(ctx, PhaseReady, ctx.Err())
	}
	// Readiness turns ready only once startup can no longer roll back, and never over a drain that already began.
	a.state.CompareAndSwap(int32(stateStarting), int32(stateServing))
	return ctx, nil
}

// ready runs the advisory health probe, then the ready hooks. A failed probe is logged and
// startup continues degraded; a shutdown requested during the probe skips the ready hooks.
func (a *App[C]) ready(ctx context.Context) error {
	if err := a.ReadyCheck(ctx); err != nil {
		a.Logger.WarnCtx(ctx, "Ready check reported issues", map[string]any{
			"error": err.Error(),
		})
	}
	if err := shutdownRequested(ctx); err != nil {
		return err
	}
	return a.emitLifecycleHooks(ctx, EventReady)
}

// shutdownRequested reports ErrShutdownRequested once Shutdown has canceled the lifecycle
// context. Cancellation of the caller's own context surfaces through each phase's error.
func shutdownRequested(ctx context.Context) error {
	if cause := context.Cause(ctx); errors.Is(cause, ErrShutdownRequested) {
		return cause
	}
	return nil
}

// abortStartup tears down whatever earlier startup phases created after a fatal error and
// returns a *StartupError holding both the cause and the teardown outcome. When the startup
// context has ended, its cause (ErrShutdownRequested, or the caller's cancellation cause) is kept
// alongside the phase error, so a phase that returned only context.Canceled does not hide why.
// Teardown runs through the normal shutdown sequence on a context that keeps values but detaches
// cancellation, because the startup context may already be canceled; shutdown skips components
// that never started, so this is safe regardless of how far startup reached.
func (a *App[C]) abortStartup(ctx context.Context, phase Phase, err error) error {
	if cause := context.Cause(ctx); cause != nil && !errors.Is(err, cause) {
		err = errors.Join(cause, err)
	}
	return &StartupError{Phase: phase, Cause: err, Rollback: a.shutdownWith(context.WithoutCancel(ctx))}
}
