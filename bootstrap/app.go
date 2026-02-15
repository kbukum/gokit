package bootstrap

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/skillsenselab/gokit/component"
	"github.com/skillsenselab/gokit/di"
	"github.com/skillsenselab/gokit/logger"
)

// App represents the base application with uniform lifecycle management.
// It orchestrates component registration, startup, and graceful shutdown
// without knowing about specific infrastructure modules.
type App struct {
	Name       string
	Version    string
	Container  di.Container
	Components *component.Registry
	Logger     *logger.Logger
	Summary    *Summary

	gracefulTimeout time.Duration
	onConfigure     []func(ctx context.Context, app *App) error

	onStart []Hook
	onReady []Hook
	onStop  []Hook
}

// NewApp creates a new application instance with the given name, version, and options.
func NewApp(name, version string, opts ...Option) *App {
	app := &App{
		Name:            name,
		Version:         version,
		Container:       di.NewContainer(),
		Components:      component.NewRegistry(),
		Logger:          logger.NewDefault(name),
		gracefulTimeout: 15 * time.Second,
	}

	for _, opt := range opts {
		opt(app)
	}

	app.Summary = NewSummary(name, version)
	return app
}

// RegisterComponent adds a component to the application's registry.
func (a *App) RegisterComponent(c component.Component) error {
	return a.Components.Register(c)
}

// OnConfigure registers a callback to run during the configure phase.
// Use this to set up business-layer dependencies after infrastructure is started.
func (a *App) OnConfigure(fn func(ctx context.Context, app *App) error) {
	a.onConfigure = append(a.onConfigure, fn)
}

// ReadyCheck verifies that all registered components are healthy.
// Returns nil when every component reports StatusHealthy, or an error
// describing which components are unhealthy.
func (a *App) ReadyCheck(ctx context.Context) error {
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

// Run executes the full application lifecycle:
// Initialize → OnStart hooks → Configure → ReadyCheck → OnReady hooks →
// Block on signal → OnStop hooks → Graceful Shutdown.
func (a *App) Run(ctx context.Context) error {
	start := time.Now()

	a.Logger.Info("Starting application", map[string]interface{}{
		"name":    a.Name,
		"version": a.Version,
	})

	// Phase 1: Initialize — start all registered components
	if err := a.initialize(ctx); err != nil {
		return fmt.Errorf("initialization failed: %w", err)
	}

	// Run OnStart hooks
	if err := runHooks(ctx, a.onStart); err != nil {
		return fmt.Errorf("onStart hook failed: %w", err)
	}

	// Phase 2: Configure — run business-layer setup callbacks
	if err := a.configure(ctx); err != nil {
		return fmt.Errorf("configuration failed: %w", err)
	}

	// Ready check — verify all components are healthy
	if err := a.ReadyCheck(ctx); err != nil {
		a.Logger.Warn("Ready check reported issues", map[string]interface{}{
			"error": err.Error(),
		})
	}

	// Run OnReady hooks
	if err := runHooks(ctx, a.onReady); err != nil {
		return fmt.Errorf("onReady hook failed: %w", err)
	}

	// Display startup summary
	a.Summary.SetStartupDuration(time.Since(start))
	a.Summary.DisplaySummary(a.Components, a.Logger)

	// Phase 3: Block until shutdown signal
	a.Logger.Info("Application ready — waiting for shutdown signal")
	a.waitForSignal(ctx)

	// Graceful shutdown
	return a.stop()
}

// initialize starts all registered components (Phase 1).
func (a *App) initialize(ctx context.Context) error {
	a.Logger.Info("Phase 1: Starting components")

	if err := a.Components.StartAll(ctx); err != nil {
		return fmt.Errorf("failed to start components: %w", err)
	}

	// Track started components in summary
	for _, h := range a.Components.HealthAll(ctx) {
		status := "active"
		healthy := h.Status == component.StatusHealthy
		if !healthy {
			status = string(h.Status)
		}
		a.Summary.TrackComponent(h.Name, status, healthy)
	}

	a.Logger.Info("Phase 1: All components started")
	return nil
}

// configure runs registered configuration callbacks (Phase 2).
func (a *App) configure(ctx context.Context) error {
	if len(a.onConfigure) == 0 {
		return nil
	}

	a.Logger.Info("Phase 2: Running configuration callbacks", map[string]interface{}{
		"count": len(a.onConfigure),
	})

	for _, fn := range a.onConfigure {
		if err := fn(ctx, a); err != nil {
			return err
		}
	}

	a.Logger.Info("Phase 2: Configuration complete")
	return nil
}

// waitForSignal blocks until an OS interrupt/term signal or context cancellation.
func (a *App) waitForSignal(ctx context.Context) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		a.Logger.Info("Received shutdown signal", map[string]interface{}{
			"signal": sig.String(),
		})
	case <-ctx.Done():
		a.Logger.Info("Context cancelled")
	}
}

// stop gracefully shuts down all components within the graceful timeout.
func (a *App) stop() error {
	a.Logger.Info("Shutting down application", map[string]interface{}{
		"timeout": a.gracefulTimeout.String(),
	})

	ctx, cancel := context.WithTimeout(context.Background(), a.gracefulTimeout)
	defer cancel()

	// Run OnStop hooks before stopping components
	if err := runHooks(ctx, a.onStop); err != nil {
		a.Logger.Error("OnStop hook error", map[string]interface{}{
			"error": err.Error(),
		})
	}

	if err := a.Components.StopAll(ctx); err != nil {
		a.Logger.Error("Shutdown completed with errors", map[string]interface{}{
			"error": err.Error(),
		})
		return err
	}

	if err := a.Container.Close(); err != nil {
		a.Logger.Error("DI container close error", map[string]interface{}{
			"error": err.Error(),
		})
		return err
	}

	a.Logger.Info("Application shutdown complete")
	return nil
}
