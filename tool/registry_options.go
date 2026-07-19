package tool

import (
	"context"

	"github.com/kbukum/gokit/resilience"
)

// Authorizer optionally gates tool calls before sensitivity evaluation. It is transport-neutral; mcp.Server has its own authorizer adapter that maps to authz.Decider — Registry-level authz applies to direct programmatic calls.
type Authorizer interface {
	Authorize(ctx context.Context, call ToolCall) (allowed bool, reason string, err error)
}

// WithAuthorizer wires a programmatic-call authorizer that runs before sensitivity evaluation.
func (r *Registry) WithAuthorizer(a Authorizer) *Registry {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.authorizer = a
	return r
}

// WithSensitivityEvaluator overrides the default DenyOnSensitive evaluator.
func (r *Registry) WithSensitivityEvaluator(e SensitivityEvaluator) *Registry {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e != nil {
		r.evaluator = e
	}
	return r
}

// WithHumanApproval overrides the default DenyHumanApproval gate.
func (r *Registry) WithHumanApproval(h HumanApproval) *Registry {
	r.mu.Lock()
	defer r.mu.Unlock()
	if h != nil {
		r.approval = h
	}
	return r
}

// WithToolPolicy attaches a per-tool resilience policy. The policy is stored, not enforced: orchestrators that own the dispatch loop look it up via PolicyFor and wrap their invocation. Storing it here keeps the registry as the single source of truth for "what governs this tool".
func (r *Registry) WithToolPolicy(name string, policy *resilience.Policy) *Registry {
	r.mu.Lock()
	defer r.mu.Unlock()
	if name == "" {
		return r
	}
	r.toolPolicy[name] = policy
	return r
}

// PolicyFor returns the per-tool policy stored via WithToolPolicy, or nil.
func (r *Registry) PolicyFor(name string) *resilience.Policy {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.toolPolicy[name]
}
