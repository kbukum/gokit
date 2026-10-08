package testutil

import (
	"context"
	"time"
)

// Budgets bounds cooperative component operations. Nonpositive values use the defaults.
type Budgets struct {
	Setup   time.Duration
	Cleanup time.Duration
}

// Option configures a test lifecycle.
type Option func(*Budgets)

// WithBudgets overrides the setup/reset and fresh cleanup budgets.
func WithBudgets(b Budgets) Option {
	return func(target *Budgets) { *target = b }
}

// ResolveBudgets applies opts to the defaults: 30 seconds for setup and reset, 10 seconds for cleanup. Other test harnesses use it so every gokit lifecycle helper shares one budget option.
func ResolveBudgets(opts ...Option) Budgets {
	b := Budgets{Setup: 30 * time.Second, Cleanup: 10 * time.Second}
	for _, opt := range opts {
		opt(&b)
	}
	if b.Setup <= 0 {
		b.Setup = 30 * time.Second
	}
	if b.Cleanup <= 0 {
		b.Cleanup = 10 * time.Second
	}
	return b
}

func cleanupContext(ctx context.Context, budget time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), budget)
}
