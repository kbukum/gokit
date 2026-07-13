package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/ai/semconv"
	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/observability"
	"github.com/kbukum/gokit/provider/namedregistry"
	"github.com/kbukum/gokit/resilience"
	"github.com/kbukum/gokit/schema"
)

// Registry manages a collection of callable tools.
// It is concurrent-safe for reads and writes.
type Registry struct {
	inner      *namedregistry.Registry[Callable]
	mu         sync.RWMutex
	authorizer Authorizer
	evaluator  SensitivityEvaluator
	approval   HumanApproval
	toolPolicy map[string]*resilience.Policy
	lifecycle  ai.Lifecycle
}

// Authorizer optionally gates tool calls before sensitivity evaluation. It is
// transport-neutral; mcp.Server has its own authorizer adapter that maps to
// authz.Decider — Registry-level authz applies to direct programmatic calls.
type Authorizer interface {
	Authorize(ctx context.Context, call ToolCall) (allowed bool, reason string, err error)
}

// NewRegistry creates an empty tool registry. Sensitivity evaluation defaults
// to DenyOnSensitive and human approval defaults to DenyHumanApproval, so
// any envelope-declared sensitive invocation fails closed unless the operator
// explicitly opts into a richer evaluator/approval flow.
func NewRegistry() *Registry {
	return &Registry{
		inner:      namedregistry.New[Callable]("tool"),
		evaluator:  DenyOnSensitive{},
		approval:   DenyHumanApproval{},
		toolPolicy: map[string]*resilience.Policy{},
	}
}

// WithAuthorizer wires a programmatic-call authorizer that runs before
// sensitivity evaluation.
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

// WithToolPolicy attaches a per-tool resilience policy. The policy is stored,
// not enforced: orchestrators that own the dispatch loop look it up via
// PolicyFor and wrap their invocation. Storing it here keeps the registry as
// the single source of truth for "what governs this tool".
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

// Name returns the registry component name.
func (r *Registry) Name() string { return "tool-registry" }

// Start marks the registry ready for tool lookup and invocation.
func (r *Registry) Start(_ context.Context) error {
	r.lifecycle.MarkReady()
	return nil
}

// Stop marks the registry stopped. Registered tools are caller-owned.
func (r *Registry) Stop(_ context.Context) error {
	r.lifecycle.MarkStopped()
	return nil
}

// Health reports whether the registry is ready to serve tool calls.
func (r *Registry) Health(_ context.Context) component.Health {
	if !r.lifecycle.Ready() {
		return component.Health{Name: r.Name(), Status: component.StatusDegraded, Message: "not started"}
	}
	r.mu.RLock()
	count := r.inner.Len()
	r.mu.RUnlock()
	return component.Health{Name: r.Name(), Status: component.StatusHealthy, Message: fmt.Sprintf("tools=%d", count)}
}

// Register adds a tool to the registry.
// Returns an error if a tool with the same name already exists.
func (r *Registry) Register(t Callable) error {
	if t == nil {
		return fmt.Errorf("tool: callable must not be nil")
	}
	return r.inner.Register(t.Definition().Name, t)
}

// Get retrieves a tool by name.
func (r *Registry) Get(name string) (Callable, bool) {
	return r.inner.Get(name)
}

// List returns the definitions of all registered tools.
func (r *Registry) List() []Definition {
	defs := make([]Definition, 0, r.inner.Len())
	r.inner.Each(func(_ string, t Callable) {
		defs = append(defs, t.Definition())
	})
	return defs
}

// ToolSpecs returns the lean ai.ToolSpec view of every registered tool. Use
// this when sending the tool catalog to LLM providers; it lets agent and llm
// layers describe tools without coupling those layers to package tool's
// richer permission envelope (D13: llm must not import tool).
func (r *Registry) ToolSpecs() []ai.ToolSpec {
	specs := make([]ai.ToolSpec, 0, r.inner.Len())
	r.inner.Each(func(_ string, t Callable) {
		specs = append(specs, t.Definition().ToolSpec())
	})
	return specs
}

// Len returns the number of registered tools.
func (r *Registry) Len() int { return r.inner.Len() }

// Call invokes a tool by name with raw JSON input.
//
// Dispatch order: schema validation → authz → sensitivity / destructive gate →
// (if RequireApproval) human approval → invoke (D10). Invalid input fails closed
// with ErrInvalidToolInput before any side effect; any deny short-circuits with
// ErrToolDenied wrapped with the reason. Per-tool resilience policy is applied
// by callers via PolicyFor.
func (r *Registry) Call(ctx *Context, name string, input json.RawMessage) (*Result, error) {
	spanCtx, span := observability.StartNamedSpan(ctx.Context, "github.com/kbukum/gokit/tool", "tool.call",
		observability.WithSpanKind(observability.SpanKindInternal),
		observability.WithSpanAttributes(
			observability.StringAttribute(semconv.GenAIOperationName, semconv.OpToolCall),
			observability.StringAttribute(semconv.GenAIToolName, name),
			observability.StringAttribute("tool.use_id", ctx.ToolUseID),
		),
	)
	defer span.End()
	innerCtx := *ctx
	innerCtx.Context = spanCtx
	ctx = &innerCtx

	t, ok := r.Get(name)
	if !ok {
		err := fmt.Errorf("tool %q not found", name)
		span.RecordError(err)
		return nil, err
	}

	def := t.Definition()
	call := ToolCall{Name: name, ToolUseID: ctx.ToolUseID, Input: input, Definition: def}

	if result := t.Validate(input); !result.Valid {
		err := fmt.Errorf("%w: %s", ErrInvalidToolInput, validationMessage(result))
		span.RecordError(err)
		return nil, err
	}

	r.mu.RLock()
	authorizer := r.authorizer
	evaluator := r.evaluator
	approval := r.approval
	r.mu.RUnlock()

	if authorizer != nil {
		allowed, reason, err := authorizer.Authorize(ctx.Context, call)
		if err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("tool %q: authorize: %w", name, err)
		}
		if !allowed {
			err := fmt.Errorf("%w: %s", ErrToolDenied, reason)
			span.RecordError(err)
			return nil, err
		}
	}

	approved := false
	if evaluator != nil {
		for _, predicate := range def.Envelope.SensitiveInvocations {
			decision, reason, err := evaluator.Evaluate(ctx.Context, call, predicate)
			if err != nil {
				span.RecordError(err)
				return nil, fmt.Errorf("tool %q: evaluate: %w", name, err)
			}
			switch decision {
			case DecisionAllow:
				continue
			case DecisionDeny:
				err := fmt.Errorf("%w: %s", ErrToolDenied, reason)
				span.RecordError(err)
				return nil, err
			case DecisionRequireApproval:
				if err := r.requireApproval(ctx.Context, approval, call, reason); err != nil {
					span.RecordError(err)
					return nil, err
				}
				approved = true
			default:
				err := fmt.Errorf("tool %q: unknown evaluator decision %q", name, decision)
				span.RecordError(err)
				return nil, err
			}
		}
	}

	// Destructive tools are always human-gated: an irreversible mutation must
	// be approved out of band. With the default DenyHumanApproval this fails
	// closed until an operator wires a real approver. A prior sensitivity
	// predicate may already have obtained approval for this dispatch; approve
	// only once per call.
	if def.Envelope.Safety == SafetyDestructive && !approved {
		if err := r.requireApproval(ctx.Context, approval, call, "destructive tool requires human approval"); err != nil {
			span.RecordError(err)
			return nil, err
		}
	}

	res, err := t.Call(ctx, input)
	if err != nil {
		span.RecordError(err)
	} else {
		r.lifecycle.Touch()
	}
	return res, err
}

// requireApproval routes a call through the human approver, failing closed when
// no approver is configured or approval is rejected.
func (r *Registry) requireApproval(ctx context.Context, approval HumanApproval, call ToolCall, reason string) error {
	if approval == nil {
		return fmt.Errorf("%w: %s (no approver configured)", ErrToolDenied, reason)
	}
	approved, err := approval.Approve(ctx, call)
	if err != nil {
		return fmt.Errorf("tool %q: approval: %w", call.Name, err)
	}
	if !approved {
		return fmt.Errorf("%w: %s (human approval rejected)", ErrToolDenied, reason)
	}
	return nil
}

// validationMessage renders a compact, human-readable summary of schema
// validation failures for error text.
func validationMessage(result schema.ValidationResult) string {
	if len(result.Errors) == 0 {
		return "input does not satisfy schema"
	}
	msgs := make([]string, 0, len(result.Errors))
	for _, e := range result.Errors {
		msgs = append(msgs, e.Error())
	}
	return strings.Join(msgs, "; ")
}

// BatchCall represents a single tool invocation in a batch.
type BatchCall struct {
	Name  string          `json:"name"`
	ID    string          `json:"id"`
	Input json.RawMessage `json:"input"`
}

// BatchResult pairs a batch call with its result.
type BatchResult struct {
	ID     string  `json:"id"`
	Result *Result `json:"result,omitempty"`
	Err    error   `json:"error,omitempty"`
}

// BatchOptions controls passive batch execution. The caller owns policy: agent supplies
// tool concurrency and fail-fast behavior; Registry does not infer concurrency from ReadOnly.
type BatchOptions struct {
	Concurrency int
	FailFast    bool
}

// CallBatch executes multiple tool calls with a caller-supplied concurrency cap.
// Results are returned in the same order as calls.
func (r *Registry) CallBatch(ctx *Context, calls []BatchCall, opts BatchOptions) []BatchResult {
	results := make([]BatchResult, len(calls))
	if len(calls) == 0 {
		return results
	}
	concurrency := opts.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}
	if concurrency > len(calls) {
		concurrency = len(calls)
	}
	sem := make(chan struct{}, concurrency)
	done := make(chan struct{})
	var once sync.Once
	var wg sync.WaitGroup
	stop := func() bool {
		select {
		case <-done:
			return true
		default:
			return false
		}
	}
	for i, c := range calls {
		if stop() {
			results[i] = BatchResult{ID: c.ID, Err: fmt.Errorf("tool: batch stopped after fail-fast")}
			continue
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(i int, c BatchCall) {
			defer wg.Done()
			defer func() { <-sem }()
			if stop() {
				results[i] = BatchResult{ID: c.ID, Err: fmt.Errorf("tool: batch stopped after fail-fast")}
				return
			}
			callCtx := ctx.clone()
			callCtx.ToolUseID = c.ID
			res, err := r.Call(callCtx, c.Name, c.Input)
			results[i] = BatchResult{ID: c.ID, Result: res, Err: err}
			if opts.FailFast && err != nil {
				once.Do(func() { close(done) })
			}
		}(i, c)
	}
	wg.Wait()
	return results
}

// Names returns the names of all registered tools.
func (r *Registry) Names() []string {
	return r.inner.Names()
}

// Search returns definitions matching a keyword query against name and description.
func (r *Registry) Search(query string) []Definition {
	q := strings.ToLower(query)
	var result []Definition
	r.inner.Each(func(_ string, t Callable) {
		def := t.Definition()
		if strings.Contains(strings.ToLower(def.Name), q) ||
			strings.Contains(strings.ToLower(def.Description), q) {
			result = append(result, def)
		}
	})
	return result
}

// Filter returns definitions matching the given options.
func (r *Registry) Filter(opts ...FilterOption) []Definition {
	cfg := &filterConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	var result []Definition
	r.inner.Each(func(_ string, t Callable) {
		def := t.Definition()
		if matchesFilter(def, cfg) {
			result = append(result, def)
		}
	})
	return result
}

// FilterOption configures tool filtering.
type FilterOption func(*filterConfig)

type filterConfig struct {
	category      string
	tags          []string
	executionHint *ExecutionHint
}

// WithCategory filters tools by category annotation.
func WithCategory(cat string) FilterOption {
	return func(c *filterConfig) { c.category = cat }
}

// WithTags filters tools that have all specified tags.
func WithTags(tags ...string) FilterOption {
	return func(c *filterConfig) { c.tags = tags }
}

// WithExecutionHint filters tools by resolved execution hint annotation.
func WithExecutionHint(hint ExecutionHint) FilterOption {
	return func(c *filterConfig) { c.executionHint = &hint }
}

func matchesFilter(def Definition, cfg *filterConfig) bool {
	if cfg.category != "" {
		if def.Annotations.Category != cfg.category {
			return false
		}
	}
	if cfg.executionHint != nil {
		if def.Annotations.ExecutionHint.Resolved() != cfg.executionHint.Resolved() {
			return false
		}
	}
	if len(cfg.tags) > 0 {
		tagSet := make(map[string]bool, len(def.Annotations.Tags))
		for _, t := range def.Annotations.Tags {
			tagSet[t] = true
		}
		for _, required := range cfg.tags {
			if !tagSet[required] {
				return false
			}
		}
	}
	return true
}
