package bootstrap

import (
	"time"

	"github.com/kbukum/gokit/di"
	"github.com/kbukum/gokit/logger"
)

// Option configures the App during creation.
// Options are non-generic so they can be used with any config type.
type Option func(*appOptions)

// appOptions collects all option values before applying to App.
type appOptions struct {
	logger          *logger.Logger
	container       di.Container
	gracefulTimeout *time.Duration
}

// resolveOptions applies all options and returns the collected values.
func resolveOptions(opts []Option) *appOptions {
	o := &appOptions{}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// WithLogger sets a custom logger for the application.
// If not set, the logger is auto-initialized from the config's Logging field.
func WithLogger(l *logger.Logger) Option {
	return func(o *appOptions) {
		o.logger = l
	}
}

// WithGracefulTimeout sets the maximum duration for graceful shutdown.
func WithGracefulTimeout(d time.Duration) Option {
	return func(o *appOptions) {
		o.gracefulTimeout = &d
	}
}

// WithContainer sets a custom DI container for the application.
func WithContainer(c di.Container) Option {
	return func(o *appOptions) {
		o.container = c
	}
}
