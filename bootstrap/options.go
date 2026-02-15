package bootstrap

import (
	"time"

	"github.com/skillsenselab/gokit/config"
	"github.com/skillsenselab/gokit/di"
	"github.com/skillsenselab/gokit/logger"
)

// Option is a functional option for configuring an App.
type Option func(*App)

// WithLogger sets a custom logger for the application.
func WithLogger(l *logger.Logger) Option {
	return func(a *App) {
		a.Logger = l
	}
}

// WithGracefulTimeout sets the maximum duration for graceful shutdown.
func WithGracefulTimeout(d time.Duration) Option {
	return func(a *App) {
		a.gracefulTimeout = d
	}
}

// WithContainer sets a custom DI container for the application.
func WithContainer(c di.Container) Option {
	return func(a *App) {
		a.Container = c
	}
}

// WithConfig loads configuration from YAML/env into the provided struct.
// Use this when you want App to own config loading instead of doing it in main.go.
func WithConfig(cfg interface{}, opts ...config.LoaderOption) Option {
	return func(a *App) {
		if err := config.LoadConfig(a.Name, cfg, opts...); err != nil {
			a.Logger.Error("Failed to load config", map[string]interface{}{
				"error": err.Error(),
			})
			return
		}
		a.cfg = cfg
	}
}

// WithLogging initializes the global logger from the given config.
// Use this when you want App to own logger initialization instead of doing it in main.go.
func WithLogging(cfg logger.Config) Option {
	return func(a *App) {
		cfg.ApplyDefaults()
		logger.Init(cfg)
		a.Logger = logger.GetGlobalLogger().WithComponent(a.Name)
	}
}
