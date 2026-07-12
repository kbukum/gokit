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
	onConfigure     []func(ctx context.Context, app *App[C]) error
	hooks           *hook.Registry
}

// NewApp creates a new application instance from a typed config.
// It applies defaults, validates the config, and initializes the logging.
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
		gracefulTimeout: 15 * time.Second,
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
		app.Logger = logging.New(&base.Logging, base.Name)
	}

	app.Summary = NewSummary(base.Name, base.Version)
	return app, nil
}

// RegisterComponent adds a component to the application's registry.
func (a *App[C]) RegisterComponent(c component.Component) error {
	return a.Components.Register(c)
}

// OnConfigure registers a callback to run during the configure phase.
// Use this to set up business-layer dependencies after infrastructure is started.
func (a *App[C]) OnConfigure(fn func(ctx context.Context, app *App[C]) error) {
	a.onConfigure = append(a.onConfigure, fn)
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
// Initialize → OnStart hooks → Configure → ReadyCheck → OnReady hooks →
// Block on signal → OnStop hooks → Graceful Shutdown.
func (a *App[C]) Run(ctx context.Context) error {
	if err := a.startup(ctx); err != nil {
		return err
	}

	// Block until shutdown signal
	a.Logger.InfoCtx(ctx, "Application ready — waiting for shutdown signal")
	a.WaitForSignal(ctx)

	// Graceful shutdown
	return a.stop()
}

// RunTask executes a finite task with the full bootstrap lifecycle.
// Unlike Run(), it does not block on shutdown signals — it runs the task
// function and gracefully shuts down when the task completes or the context
// is canceled (e.g., via SIGINT/SIGTERM).
//
// Use RunTask for CLI tools, batch jobs, and one-shot processes that need
// the same bootstrap infrastructure (config, logger, components, hooks)
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
	if stopErr := a.stop(); stopErr != nil {
		if taskErr != nil {
			return taskErr
		}
		return stopErr
	}

	return taskErr
}

// startup performs the common initialization sequence shared by Run and RunTask.
func (a *App[C]) startup(ctx context.Context) error {
	start := time.Now()

	a.Logger.InfoCtx(ctx, "Starting application", map[string]any{
		"name":    a.Name,
		"version": a.Version,
	})

	// Phase 1: Configure — run business-layer setup callbacks that may register
	// additional components. This happens before StartAll so that all components
	// (infrastructure + application) start in a single pass.
	if err := a.configure(ctx); err != nil {
		return fmt.Errorf("configuration failed: %w", err)
	}

	// Phase 2: Start — single-pass StartAll for all registered components.
	if err := a.Components.StartAll(ctx); err != nil {
		// Partial rollback: run OnStop hooks for cleanup of any resources
		// that configure callbacks may have set up.
		if stopErr := a.emitLifecycleHooks(ctx, EventStop); stopErr != nil {
			a.Logger.ErrorCtx(ctx, "OnStop hook error during startup rollback", map[string]any{
				"error": stopErr.Error(),
			})
		}
		return fmt.Errorf("component startup failed: %w", err)
	}

	// Run OnStart hooks
	if err := a.emitLifecycleHooks(ctx, EventStart); err != nil {
		return fmt.Errorf("onStart hook failed: %w", err)
	}

	// Ready check — verify all components are healthy
	if err := a.ReadyCheck(ctx); err != nil {
		a.Logger.WarnCtx(ctx, "Ready check reported issues", map[string]any{
			"error": err.Error(),
		})
	}

	// Run OnReady hooks
	if err := a.emitLifecycleHooks(ctx, EventReady); err != nil {
		return fmt.Errorf("onReady hook failed: %w", err)
	}

	// Display startup summary
	a.Summary.SetStartupDuration(time.Since(start))
	a.DisplaySummary()

	return nil
}

// DisplaySummary prints the startup summary. It auto-collects infrastructure,
// routes, and health from the component registry and DI container.
func (a *App[C]) DisplaySummary() {
	a.Summary.DisplaySummary(a.Components, a.Container, a.Logger)
}

// configure runs registered configuration callbacks (Phase 2).
func (a *App[C]) configure(ctx context.Context) error {
	if len(a.onConfigure) == 0 {
		return nil
	}

	a.Logger.DebugCtx(ctx, "Phase 2: Running configuration callbacks", map[string]any{
		"count": len(a.onConfigure),
	})

	for _, fn := range a.onConfigure {
		if err := fn(ctx, a); err != nil {
			return err
		}
	}

	a.Logger.DebugCtx(ctx, "Phase 2: Configuration complete")
	return nil
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

// Startup performs the full bootstrap lifecycle (initialize, start hooks,
// configure, start components, ready check, ready hooks) without blocking
// on shutdown signals. Pair with Shutdown for test and CLI scenarios.
func (a *App[C]) Startup(ctx context.Context) error {
	return a.startup(ctx)
}

// Shutdown performs graceful shutdown using the supplied ctx. If ctx has no
// deadline, the configured gracefulTimeout is applied. If ctx has a deadline
// shorter than gracefulTimeout, ctx wins.
//
// Use when managing your own lifecycle (e.g. when not relying on signal
// handling via Run).
func (a *App[C]) Shutdown(ctx context.Context) error {
	return a.shutdownWith(ctx)
}

// stop is invoked by the internal signal handler. It seeds shutdown with a
// fresh context bounded by gracefulTimeout because the original Run ctx is
// already canceled by the time we get here.
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

	// Run OnStop hooks before stopping components — collect all errors.
	if err := a.emitLifecycleHooks(ctx, EventStop); err != nil {
		a.Logger.ErrorCtx(ctx, "OnStop hook error", map[string]any{
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

	// Close DI container (lazy components only — singletons are component-managed)
	if err := a.Container.Close(); err != nil {
		a.Logger.ErrorCtx(ctx, "DI container close error", map[string]any{
			"error": err.Error(),
		})
		shutdownErrs = append(shutdownErrs, err)
	}

	a.Logger.InfoCtx(ctx, "Application shutdown complete")
	return errors.Join(shutdownErrs...)
}
