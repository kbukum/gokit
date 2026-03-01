package process

import (
	"context"
	"time"

	"github.com/kbukum/gokit/provider"
)

// compile-time assertions
var _ provider.RequestResponse[Command, *Result] = (*Adapter)(nil)

// Config configures a process adapter.
type Config struct {
	// Name identifies this adapter instance (used by provider.Provider interface).
	Name string `yaml:"name,omitempty" mapstructure:"name"`
	// GracePeriod is the default grace period for SIGTERM→SIGKILL.
	GracePeriod time.Duration `yaml:"grace_period,omitempty" mapstructure:"grace_period"`
	// Timeout is the default execution timeout. Zero means no timeout.
	Timeout time.Duration `yaml:"timeout,omitempty" mapstructure:"timeout"`
}

// Adapter wraps subprocess execution as a provider.RequestResponse.
type Adapter struct {
	config Config
}

// NewAdapter creates a new process adapter.
func NewAdapter(cfg Config) *Adapter {
	return &Adapter{config: cfg}
}

// Run executes a command, applying adapter-level defaults.
func (a *Adapter) Run(ctx context.Context, cmd Command) (*Result, error) {
	if cmd.GracePeriod == 0 && a.config.GracePeriod > 0 {
		cmd.GracePeriod = a.config.GracePeriod
	}
	if a.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, a.config.Timeout)
		defer cancel()
	}
	return Run(ctx, cmd)
}

// Name returns the adapter name (implements provider.Provider).
func (a *Adapter) Name() string {
	return a.config.Name
}

// IsAvailable always returns true for process adapters (implements provider.Provider).
func (a *Adapter) IsAvailable(_ context.Context) bool {
	return true
}

// Execute runs a command (implements provider.RequestResponse[Command, *Result]).
func (a *Adapter) Execute(ctx context.Context, cmd Command) (*Result, error) {
	return a.Run(ctx, cmd)
}
