package bootstrap

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
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
// An App runs one lifecycle: after Run, RunTask, or Startup has begun, a later start returns
// [ErrLifecycleUsed]. Shutdown runs teardown once and repeated calls return the recorded outcome.
// The App owns and closes its DI container, including one supplied with [WithContainer], and the
// logger it creates from config. A logger supplied with [WithLogger] is borrowed and never closed.
//
// Example:
//
//	app, err := bootstrap.NewApp(&myConfig)
//	if err != nil {
//	    return err
//	}
//	app.OnConfigure(func(ctx context.Context, a *bootstrap.App[*MyConfig]) error {
//	    // a.Cfg is *MyConfig — fully typed
//	    return nil
//	})
//	return app.Run(ctx)
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
	modules         moduleSet
	ownsLogger      bool
	admin           *adminComponent
	state           atomic.Int32 // lifecycleState reported by the admin listener

	lifecycleMu     sync.Mutex
	used            bool
	modulesSealed   bool
	starting        bool
	cancelLifecycle context.CancelCauseFunc
	active          chan struct{}
	stopping        chan struct{}
	shutdownOnce    sync.Once
	shutdownErr     error
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
	if o.admin != nil {
		admin, err := newAdmin(*o.admin, app.readiness)
		if err != nil {
			return nil, err
		}
		app.admin = admin
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
		app.ownsLogger = true
	}

	regCfg := component.DefaultRegistryConfig()
	regCfg.Logger = app.Logger.WithComponent("component")
	app.Components = component.NewRegistryWithConfig(regCfg)
	app.Summary = NewSummary(base.Name, base.Version)
	if app.admin != nil {
		// The registry is new, so the first registration cannot collide.
		_ = app.Components.Register(app.admin)
	}
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
// Configure → Modules → OnBeforeStart hooks → StartAll → OnAfterStart hooks → ReadyCheck → OnReady hooks → Block on signal → Quiesce → OnBeforeStop hooks → Drain and release dependencies → OnAfterStop hooks → Release the owned logger.
// It returns a *StartupError when startup fails and a *ShutdownError when teardown fails.
func (a *App[C]) Run(ctx context.Context) error {
	lifeCtx, err := a.startup(ctx, false)
	if err != nil {
		return err
	}

	// Block until a shutdown signal, ctx cancellation, or a Shutdown call.
	a.Logger.InfoCtx(lifeCtx, "Application ready — waiting for shutdown signal")
	waitCtx, cancelWait := a.untilShutdown(lifeCtx)
	defer cancelWait()
	a.WaitForSignal(waitCtx)

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
// A startup failure returns a *StartupError and the task does not run. A task failure returns a
// *TaskError holding both the task error and any teardown failure. A teardown failure after a
// successful task returns a *ShutdownError. A Shutdown call cancels the task's context and waits
// for the task to return before teardown begins.
//
// Example:
//
//	app, err := bootstrap.NewApp(&cfg)
//	if err != nil {
//	    return err
//	}
//	return app.RunTask(ctx, func(ctx context.Context) error {
//	    return processData(ctx)
//	})
func (a *App[C]) RunTask(ctx context.Context, task func(ctx context.Context) error) error {
	lifeCtx, err := a.startup(ctx, true)
	if err != nil {
		return err
	}

	// Set up signal-based cancellation for the task
	taskCtx, cancel := a.untilShutdown(lifeCtx)
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

	taskErr := a.runActiveTask(taskCtx, task)
	stopErr := a.stop() //nolint:contextcheck // stop intentionally uses a fresh bounded context; the task ctx may be canceled at shutdown
	if taskErr != nil {
		return &TaskError{Cause: taskErr, Shutdown: stopErr}
	}
	return stopErr
}

// DisplaySummary prints the startup summary. It auto-collects infrastructure, routes,
// and health from the component registry and DI container.
func (a *App[C]) DisplaySummary(ctx context.Context) {
	a.Summary.DisplaySummary(ctx, a.Components, a.Container, a.Logger)
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
// Pair with Shutdown for test and CLI scenarios. A failure returns a *StartupError after rollback; a later Shutdown returns the recorded rollback outcome.
func (a *App[C]) Startup(ctx context.Context) error {
	_, err := a.startup(ctx, false)
	return err
}
