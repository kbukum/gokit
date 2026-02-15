package bootstrap

import (
	"time"

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
