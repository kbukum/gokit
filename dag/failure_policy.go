package dag

// FailurePolicy controls how execution reacts to node failures.
type FailurePolicy int

const (
	// FailFast aborts the DAG after the current level finishes when any node fails.
	FailFast FailurePolicy = iota
	// Continue allows dependents to run even when upstream nodes fail.
	Continue
	// SkipDependents skips nodes whose upstream dependencies failed or were skipped.
	SkipDependents
)

// EngineConfig configures engine-wide execution behavior.
type EngineConfig struct {
	MaxParallel   int
	FailurePolicy FailurePolicy
}

// NewEngine creates an engine from configuration.
func NewEngine(cfg EngineConfig) *Engine {
	return &Engine{MaxParallel: cfg.MaxParallel, Config: &cfg}
}

func (e *Engine) failurePolicy() FailurePolicy {
	if e == nil || e.Config == nil {
		return SkipDependents
	}
	return e.Config.FailurePolicy
}

func (e *Engine) nodeFailurePolicy(def NodeDef) FailurePolicy {
	switch def.OnError {
	case OnErrorFail:
		return FailFast
	case OnErrorContinue:
		return Continue
	case OnErrorSkip:
		return SkipDependents
	default:
		return e.failurePolicy()
	}
}
