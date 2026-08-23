package component

import "time"

// DefaultStartTimeout bounds a single component's Start call when the caller-supplied
// context has no deadline. The bound is cooperative — Start must return when its context is
// canceled (see Component.Start); it caps a well-behaved Start, not one that ignores ctx.
const DefaultStartTimeout = 30 * time.Second

// RegistryConfig configures a component Registry: how many components may start in
// parallel and the per-component start/stop timeouts applied when the caller's context
// carries no deadline.
type RegistryConfig struct {
	// Concurrency is the maximum number of components started in parallel by
	// StartAllConcurrent. Zero means "no limit" (start every candidate at once).
	// Sequential StartAll ignores this field.
	Concurrency int

	// StartTimeout bounds each component's Start call when ctx has no deadline.
	StartTimeout time.Duration

	// StopTimeout bounds each component's Stop call when ctx has no deadline.
	StopTimeout time.Duration
}

// DefaultRegistryConfig returns the registry defaults: sequential start (Concurrency 1)
// with bounded start and stop timeouts.
func DefaultRegistryConfig() RegistryConfig {
	return RegistryConfig{
		Concurrency:  1,
		StartTimeout: DefaultStartTimeout,
		StopTimeout:  DefaultStopTimeout,
	}
}

// withDefaults fills zero fields with the registry defaults so a partially-specified
// config is still usable.
func (c RegistryConfig) withDefaults() RegistryConfig {
	if c.Concurrency < 0 {
		c.Concurrency = 0
	}
	if c.StartTimeout <= 0 {
		c.StartTimeout = DefaultStartTimeout
	}
	if c.StopTimeout <= 0 {
		c.StopTimeout = DefaultStopTimeout
	}
	return c
}
