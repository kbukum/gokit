package bootstrap

import (
	"time"

	"github.com/kbukum/gokit/di"
	"github.com/kbukum/gokit/logging"
)

// Option configures the App during creation. Options are non-generic
// so they can be used with any config type.
type Option func(*appOptions)

// appOptions collects all option values before applying to App.
type appOptions struct {
	logger          *logging.Logger
	container       *di.Container
	gracefulTimeout *time.Duration
	admin           *AdminConfig
}

// resolveOptions applies all options and returns the collected values.
func resolveOptions(opts []Option) *appOptions {
	o := &appOptions{}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// WithLogger sets a borrowed logger for the application. The App never closes it; the caller
// closes it after the App shuts down. If not set, the App creates a logger from the config's
// Logging field, owns it, and releases it as the last step of shutdown.
func WithLogger(l *logging.Logger) Option {
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

// WithContainer sets a custom DI container for the application. Ownership moves to the App,
// which closes the container during shutdown like its default container.
func WithContainer(c *di.Container) Option {
	return func(o *appOptions) {
		o.container = c
	}
}
