package tool

import (
	"fmt"
	"strings"
	"sync"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/provider/namedregistry"
	"github.com/kbukum/gokit/resilience"
)

// Registry manages a collection of callable tools. It is concurrent-safe for reads and writes.
type Registry struct {
	inner      *namedregistry.Registry[Callable]
	mu         sync.RWMutex
	authorizer Authorizer
	evaluator  SensitivityEvaluator
	approval   HumanApproval
	toolPolicy map[string]*resilience.Policy
	lifecycle  ai.Lifecycle
}

// NewRegistry creates an empty tool registry. Sensitivity evaluation defaults to DenyOnSensitive and human approval defaults to DenyHumanApproval, so any envelope-declared sensitive invocation fails closed unless the operator explicitly opts into a richer evaluator/approval flow.
func NewRegistry() *Registry {
	return &Registry{
		inner:      namedregistry.New[Callable]("tool"),
		evaluator:  DenyOnSensitive{},
		approval:   DenyHumanApproval{},
		toolPolicy: map[string]*resilience.Policy{},
	}
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

// ToolSpecs returns the lean ai.ToolSpec view of every registered tool.
// Use this when sending the tool catalog to LLM providers; it lets agent and llm layers describe tools without coupling those layers to package tool's richer permission envelope (D13: llm must not import tool).
func (r *Registry) ToolSpecs() []ai.ToolSpec {
	specs := make([]ai.ToolSpec, 0, r.inner.Len())
	r.inner.Each(func(_ string, t Callable) {
		specs = append(specs, t.Definition().ToolSpec())
	})
	return specs
}

// Len returns the number of registered tools.
func (r *Registry) Len() int { return r.inner.Len() }

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
