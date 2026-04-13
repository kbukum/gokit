package chain

// Builder provides a fluent API for constructing chain executors.
type Builder struct {
	operations []Operation
	config     Config
}

// NewBuilder creates a new empty builder.
func NewBuilder() *Builder {
	return &Builder{
		config: DefaultConfig(),
	}
}

// Step adds an operation to the chain.
func (b *Builder) Step(op Operation) *Builder {
	b.operations = append(b.operations, op)
	return b
}

// WithConfig sets the full chain configuration.
func (b *Builder) WithConfig(cfg Config) *Builder {
	b.config = cfg
	return b
}

// CleanupOnFailure enables or disables cleanup of completed steps on failure.
func (b *Builder) CleanupOnFailure(enabled bool) *Builder {
	b.config.CleanupOnFailure = enabled
	return b
}

// StopOnFailure enables or disables stopping on the first failure.
func (b *Builder) StopOnFailure(enabled bool) *Builder {
	b.config.StopOnFailure = enabled
	return b
}

// Build creates the chain executor.
func (b *Builder) Build() *Executor {
	return NewExecutor(b.operations).WithConfig(b.config)
}
