package tool

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// Registry manages a collection of callable tools.
// It is concurrent-safe for reads and writes.
type Registry struct {
	tools map[string]Callable
	mu    sync.RWMutex
}

// NewRegistry creates an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Callable),
	}
}

// Register adds a tool to the registry.
// Returns an error if a tool with the same name already exists.
func (r *Registry) Register(t Callable) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := t.Definition().Name
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("tool %q already registered", name)
	}
	r.tools[name] = t
	return nil
}

// MustRegister registers a tool and panics on error.
func (r *Registry) MustRegister(t Callable) {
	if err := r.Register(t); err != nil {
		panic(err)
	}
}

// Get retrieves a tool by name.
func (r *Registry) Get(name string) (Callable, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// List returns the definitions of all registered tools.
func (r *Registry) List() []Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	defs := make([]Definition, 0, len(r.tools))
	for _, t := range r.tools {
		defs = append(defs, t.Definition())
	}
	return defs
}

// Len returns the number of registered tools.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tools)
}

// Call invokes a tool by name with raw JSON input.
func (r *Registry) Call(ctx *Context, name string, input json.RawMessage) (*Result, error) {
	t, ok := r.Get(name)
	if !ok {
		return nil, fmt.Errorf("tool %q not found", name)
	}
	return t.Call(ctx, input)
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

// CallBatch executes multiple tool calls. Read-only tools run concurrently;
// non-read-only tools run serially. Results are returned in the same order
// as the input calls.
func (r *Registry) CallBatch(ctx *Context, calls []BatchCall) []BatchResult {
	results := make([]BatchResult, len(calls))

	// Partition into read-only (concurrent) and write (serial)
	type indexedCall struct {
		idx  int
		call BatchCall
	}
	var readOnly, write []indexedCall
	for i, c := range calls {
		t, ok := r.Get(c.Name)
		if !ok {
			results[i] = BatchResult{ID: c.ID, Err: fmt.Errorf("tool %q not found", c.Name)}
			continue
		}
		ic := indexedCall{idx: i, call: c}
		if t.Definition().ReadOnly {
			readOnly = append(readOnly, ic)
		} else {
			write = append(write, ic)
		}
	}

	// Execute read-only tools concurrently
	if len(readOnly) > 0 {
		var wg sync.WaitGroup
		for _, ic := range readOnly {
			wg.Add(1)
			go func(ic indexedCall) {
				defer wg.Done()
				callCtx := ctx.clone()
				callCtx.ToolUseID = ic.call.ID
				t, _ := r.Get(ic.call.Name)
				res, err := t.Call(callCtx, ic.call.Input)
				results[ic.idx] = BatchResult{ID: ic.call.ID, Result: res, Err: err}
			}(ic)
		}
		wg.Wait()
	}

	// Execute write tools serially
	for _, ic := range write {
		callCtx := ctx.clone()
		callCtx.ToolUseID = ic.call.ID
		t, _ := r.Get(ic.call.Name)
		res, err := t.Call(callCtx, ic.call.Input)
		results[ic.idx] = BatchResult{ID: ic.call.ID, Result: res, Err: err}
	}

	return results
}

// Names returns the names of all registered tools.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}

// Search returns definitions matching a keyword query against name and description.
func (r *Registry) Search(query string) []Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	q := strings.ToLower(query)
	var result []Definition
	for _, t := range r.tools {
		def := t.Definition()
		if strings.Contains(strings.ToLower(def.Name), q) ||
			strings.Contains(strings.ToLower(def.Description), q) {
			result = append(result, def)
		}
	}
	return result
}

// Filter returns definitions matching the given options.
func (r *Registry) Filter(opts ...FilterOption) []Definition {
	cfg := &filterConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Definition
	for _, t := range r.tools {
		def := t.Definition()
		if matchesFilter(def, cfg) {
			result = append(result, def)
		}
	}
	return result
}

// FilterOption configures tool filtering.
type FilterOption func(*filterConfig)

type filterConfig struct {
	category string
	tags     []string
}

// WithCategory filters tools by category annotation.
func WithCategory(cat string) FilterOption {
	return func(c *filterConfig) { c.category = cat }
}

// WithTags filters tools that have all specified tags.
func WithTags(tags ...string) FilterOption {
	return func(c *filterConfig) { c.tags = tags }
}

func matchesFilter(def Definition, cfg *filterConfig) bool {
	if cfg.category != "" {
		if def.Annotations == nil || def.Annotations.Category != cfg.category {
			return false
		}
	}
	if len(cfg.tags) > 0 {
		if def.Annotations == nil {
			return false
		}
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
