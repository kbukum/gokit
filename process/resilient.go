package process

import (
	"context"

	goerrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/provider"
)

// Runner wraps subprocess execution with persistent resilience state.
// Use NewRunner to create one, then call Run repeatedly. The circuit breaker
// state persists across calls — repeated crashes trip the breaker.
type Runner struct {
	state *provider.ResilienceState
}

// NewRunner creates a Runner with the given resilience config.
// Nil config fields are skipped. Empty config means Run() calls process.Run directly.
func NewRunner(cfg provider.ResilienceConfig) *Runner {
	return &Runner{state: provider.BuildResilience(cfg)}
}

// Run executes a subprocess through the resilience chain.
func (r *Runner) Run(ctx context.Context, cmd Command) (*Result, error) {
	if r.state == nil {
		return Run(ctx, cmd)
	}
	return provider.ExecuteWithResilience(ctx, r.state, func() (*Result, error) {
		return Run(ctx, cmd)
	})
}

// RunWithResilience is a convenience for one-shot subprocess execution with resilience.
// For repeated calls where circuit breaker state should persist, use NewRunner instead.
func RunWithResilience(ctx context.Context, cmd Command, runner *Runner) (*Result, error) {
	if runner == nil {
		return Run(ctx, cmd)
	}
	return runner.Run(ctx, cmd)
}

// SubprocessProvider wraps a Command as a provider.RequestResponse.
// The input function builds a Command from the input, and the output function
// parses the Result into the desired output type.
type SubprocessProvider[I, O any] struct {
	name      string
	buildCmd  func(I) Command
	parseOut  func(*Result) (O, error)
	available func(context.Context) bool
}

// NewSubprocessProvider creates a RequestResponse provider backed by subprocess execution.
func NewSubprocessProvider[I, O any](
	name string,
	buildCmd func(I) Command,
	parseOut func(*Result) (O, error),
) *SubprocessProvider[I, O] {
	return &SubprocessProvider[I, O]{
		name:     name,
		buildCmd: buildCmd,
		parseOut: parseOut,
	}
}

// WithAvailabilityCheck sets a custom availability check for the provider.
func (p *SubprocessProvider[I, O]) WithAvailabilityCheck(fn func(context.Context) bool) *SubprocessProvider[I, O] {
	p.available = fn
	return p
}

func (p *SubprocessProvider[I, O]) Name() string { return p.name }

func (p *SubprocessProvider[I, O]) IsAvailable(ctx context.Context) bool {
	if p.available != nil {
		return p.available(ctx)
	}
	return true
}

func (p *SubprocessProvider[I, O]) Execute(ctx context.Context, input I) (O, error) {
	cmd := p.buildCmd(input)
	result, err := Run(ctx, cmd)
	if err != nil {
		var zero O
		return zero, goerrors.ExternalServiceError(p.name, err)
	}
	return p.parseOut(result)
}
