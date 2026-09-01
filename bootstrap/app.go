package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/di"
	"github.com/kbukum/gokit/hook"
	"github.com/kbukum/gokit/logging"
)

// App represents a generic application with uniform lifecycle management.
// The type parameter C is the config type, which must satisfy the Config interface.
// Any struct embedding config.ServiceConfig automatically satisfies Config.
//
// Example:
//
//	app, err := bootstrap.NewApp(&myConfig)
//	app.OnConfigure(func(ctx context.Context, a *bootstrap.App[*MyConfig]) error {
//	    // a.Cfg is *MyConfig — fully typed
//	    return nil
//	})
//	app.Run(context.Background())
type App[C Config] struct {
	Name       string
	Version    string
	Cfg        C
	Container  *di.Container
	Components *component.Registry
	Logger     *logging.Logger
	Summary    *Summary

	gracefulTimeout time.Duration
	hooks           *hook.Registry
}

// NewApp creates a new application instance from a typed config. It applies defaults,
// validates the config, and initializes the logging.
func NewApp[C Config](cfg C, opts ...Option) (*App[C], error) {
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}

	base := cfg.GetServiceConfig()

	app := &App[C]{
		Name:            base.Name,
		Version:         base.Version,
		Cfg:             cfg,
		Container:       di.NewContainer(),
		Components:      component.NewRegistry(),
		gracefulTimeout: 30 * time.Second,
		hooks:           hook.NewRegistry(),
	}

	// Apply options (may override logger, container, timeout).
	o := resolveOptions(opts)
	if o.container != nil {
		app.Container = o.container
	}
	if o.gracefulTimeout != nil {
		app.gracefulTimeout = *o.gracefulTimeout
	}

	// Logger: use custom if provided, otherwise create from config (no global state).
	if o.logger != nil {
		app.Logger = o.logger
	} else {
		l, err := logging.New(&base.Logging, base.Name)
		if err != nil {
			return nil, fmt.Errorf("initialize logger: %w", err)
		}
		app.Logger = l
	}

	app.Summary = NewSummary(base.Name, base.Version)
	return app, nil
}

// RegisterComponent adds a component to the application's registry.
func (a *App[C]) RegisterComponent(c component.Component) error {
	return a.Components.Register(c)
}

// OnConfigure registers a callback to run during the configure phase. Use it to register
// application components and wire business-layer dependencies. Configure runs before
// StartAll, so components it registers start in the same single pass as infrastructure —
// they are not yet running when the callback executes. Callbacks run in registration order
// through the lifecycle hook registry (EventConfigure); a callback error is fatal and aborts
// startup, rolling back anything earlier phases created.
func (a *App[C]) OnConfigure(fn func(ctx context.Context, app *App[C]) error) {
	a.hooks.On(EventConfigure, func(ctx context.Context, _ hook.Event) error {
		if err := fn(ctx, a); err != nil {
			return fmt.Errorf("%w: onConfigure hook failed: %w", hook.ErrFatalHook, err)
		}
		return nil
	})
}

// ReadyCheck verifies that all registered components are healthy.
func (a *App[C]) ReadyCheck(ctx context.Context) error {
	results := a.Components.HealthAll(ctx)
	var unhealthy []string
	for _, h := range results {
		if h.Status != component.StatusHealthy {
			detail := h.Name + "=" + string(h.Status)
			if h.Message != "" {
				detail += "(" + h.Message + ")"
			}
			unhealthy = append(unhealthy, detail)
		}
	}
	if len(unhealthy) > 0 {
		return fmt.Errorf("unhealthy components: %v", unhealthy)
	}
	return nil
}

// Run executes the full application lifecycle for long-running services:
// Configure → OnBeforeStart hooks → StartAll → OnAfterStart hooks → ReadyCheck → OnReady hooks → Block on signal → OnBeforeStop hooks → StopAll → OnAfterStop hooks → Graceful Shutdown.
func (a *App[C]) Run(ctx context.Context) error {
	if err := a.startup(ctx); err != nil {
		return err
	}

	// Block until shutdown signal
	a.Logger.InfoCtx(ctx, "Application ready — waiting for shutdown signal")
	a.WaitForSignal(ctx)

	// Graceful shutdown
	return a.stop() //nolint:contextcheck // stop intentionally uses a fresh bounded context; the Run ctx is already canceled at shutdown
}

// RunTask executes a finite task with the full bootstrap lifecycle. Unlike Run(),
// it does not block on shutdown signals — it runs the task function
// and gracefully shuts down when the task completes
// or the context is canceled (e.g., via SIGINT/SIGTERM).
//
// Use RunTask for CLI tools, batch jobs,
// and one-shot processes that need the same bootstrap infrastructure (config, logger, components, hooks)
// but have a finite workflow instead of running forever.
//
// Example:
//
//	app, _ := bootstrap.NewApp(&cfg)
//	app.RunTask(ctx, func(ctx context.Context) error {
//	    return processData(ctx)
//	})
func (a *App[C]) RunTask(ctx context.Context, task func(ctx context.Context) error) error {
	if err := a.startup(ctx); err != nil {
		return err
	}

	// Set up signal-based cancellation for the task
	taskCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	go func() {
		select {
		case sig := <-sigCh:
			a.Logger.InfoCtx(taskCtx, "Received signal — canceling task", map[string]any{
				"signal": sig.String(),
			})
			cancel()
		case <-taskCtx.Done():
		}
	}()

	// Execute the task
	taskErr := task(taskCtx)

	// Graceful shutdown
	if stopErr := a.stop(); stopErr != nil { //nolint:contextcheck // stop intentionally uses a fresh bounded context; the task ctx may be canceled at shutdown
		if taskErr != nil {
			return taskErr
		}
		return stopErr
	}

	return taskErr
}

// startup performs the common initialization sequence shared by Run and RunTask. Any fatal
// error after the configure phase begins rolls back through the full shutdown sequence
// (stop hooks, component teardown, DI container close) before returning, so a failed startup
// never leaves components or container resources running.
func (a *App[C]) startup(ctx context.Context) error {
	start := time.Now()

	a.Logger.InfoCtx(ctx, "Starting application", map[string]any{
		"name":    a.Name,
		"version": a.Version,
	})

	// Phase: configure — run application-layer setup callbacks (registered via OnConfigure)
	// that may register additional components. This happens before StartAll so that all
	// components (infrastructure + application) start in a single pass. A configure error is
	// fatal and aborts startup.
	if err := a.emitLifecycleHooks(ctx, EventConfigure); err != nil {
		return a.abortStartup("configure hook failed", err)
	}

	// Phase: before_start — hooks run before any component is started.
	if err := a.emitLifecycleHooks(ctx, EventBeforeStart); err != nil {
		return a.abortStartup("onBeforeStart hook failed", err)
	}

	// Phase: start — single-pass StartAll for all registered components.
	if err := a.Components.StartAll(ctx); err != nil {
		return a.abortStartup("component startup failed", err)
	}

	// Phase: after_start — hooks run after all components are started, before the ready check.
	if err := a.emitLifecycleHooks(ctx, EventAfterStart); err != nil {
		return a.abortStartup("onAfterStart hook failed", err)
	}

	// Ready check — advisory health probe of all components. A failure is logged and startup
	// continues (degraded start); the ready phase below runs regardless of the outcome.
	if err := a.ReadyCheck(ctx); err != nil {
		a.Logger.WarnCtx(ctx, "Ready check reported issues", map[string]any{
			"error": err.Error(),
		})
	}

	// Phase: ready — hooks run after the ready check completes, before accepting traffic.
	if err := a.emitLifecycleHooks(ctx, EventReady); err != nil {
		return a.abortStartup("onReady hook failed", err)
	}

	// Display startup summary
	a.Summary.SetStartupDuration(time.Since(start))
	a.DisplaySummary()

	return nil
}

// abortStartup tears down whatever earlier startup phases created after a fatal error and
// returns the wrapped cause. Teardown runs through the normal shutdown sequence on a fresh
// bounded context (via stop) because the startup context may already be canceled; StopAll
// skips components that never started, so this is safe regardless of how far startup reached.
func (a *App[C]) abortStartup(phase string, cause error) error {
	if err := a.stop(); err != nil {
		a.Logger.ErrorCtx(context.Background(), "Startup rollback reported errors", map[string]any{
			"phase": phase,
			"error": err.Error(),
		})
	}
	return fmt.Errorf("%s: %w", phase, cause)
}

// DisplaySummary prints the startup summary. It auto-collects infrastructure, routes,
// and health from the component registry and DI container.
func (a *App[C]) DisplaySummary() {
	a.Summary.DisplaySummary(a.Components, a.Container, a.Logger)
}

// WaitForSignal blocks until an OS interrupt/term signal or context cancellation.
func (a *App[C]) WaitForSignal(ctx context.Context) os.Signal {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	select {
	case sig := <-sigCh:
		a.Logger.InfoCtx(ctx, "Received shutdown signal — graceful shutdown starting", map[string]any{
			"signal": sig.String(),
		})
		return sig
	case <-ctx.Done():
		a.Logger.InfoCtx(ctx, "Context canceled — shutting down")
		return nil
	}
}

// Startup performs the full bootstrap lifecycle (configure, before-start hooks, start components, after-start hooks, ready check, ready hooks) without blocking on shutdown signals.
// Pair with Shutdown for test and CLI scenarios.
func (a *App[C]) Startup(ctx context.Context) error {
	return a.startup(ctx)
}

// Shutdown performs graceful shutdown using the supplied ctx. If ctx has no deadline,
// the configured gracefulTimeout is applied. If ctx has a deadline shorter than gracefulTimeout,
// ctx wins.
//
// Use when managing your own lifecycle (e.g. when not relying on signal handling via Run).
func (a *App[C]) Shutdown(ctx context.Context) error {
	return a.shutdownWith(ctx)
}

// stop is invoked by the internal signal handler.
// It seeds shutdown with a fresh context bounded by gracefulTimeout because the original Run ctx is already canceled by the time we get here.
func (a *App[C]) stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), a.gracefulTimeout)
	defer cancel()
	return a.shutdownWith(ctx)
}

// shutdownWith runs the actual shutdown sequence. If ctx has no deadline,
// gracefulTimeout is applied so a misbehaving Stop cannot block forever.
func (a *App[C]) shutdownWith(parent context.Context) error {
	a.Logger.InfoCtx(parent, "Shutting down application", map[string]any{
		"timeout": a.gracefulTimeout.String(),
	})

	ctx := parent
	var cancel context.CancelFunc
	if _, hasDeadline := parent.Deadline(); !hasDeadline {
		ctx, cancel = context.WithTimeout(parent, a.gracefulTimeout)
		defer cancel()
	}

	var shutdownErrs []error

	// Phase: before_stop — hooks run before stopping components — collect all errors.
	if err := a.emitLifecycleHooks(ctx, EventBeforeStop); err != nil {
		a.Logger.ErrorCtx(ctx, "OnBeforeStop hook error", map[string]any{
			"error": err.Error(),
		})
		shutdownErrs = append(shutdownErrs, err)
	}

	// Stop all components (reverse order)
	if err := a.Components.StopAll(ctx); err != nil {
		a.Logger.ErrorCtx(ctx, "Shutdown completed with errors", map[string]any{
			"error": err.Error(),
		})
		shutdownErrs = append(shutdownErrs, err)
	}

	// Phase: after_stop — hooks run after all components are stopped, before DI teardown.
	if err := a.emitLifecycleHooks(ctx, EventAfterStop); err != nil {
		a.Logger.ErrorCtx(ctx, "OnAfterStop hook error", map[string]any{
			"error": err.Error(),
		})
		shutdownErrs = append(shutdownErrs, err)
	}

	// Close DI container:
	// runs disposers for container-owned resources (RegisterCloseable / RegisterSingletonCloseable) in reverse order.
	if err := a.Container.Close(ctx); err != nil {
		a.Logger.ErrorCtx(ctx, "DI container close error", map[string]any{
			"error": err.Error(),
		})
		shutdownErrs = append(shutdownErrs, err)
	}

	a.Logger.InfoCtx(ctx, "Application shutdown complete")
	return errors.Join(shutdownErrs...)
}
