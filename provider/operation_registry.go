package provider

import (
	"fmt"
	"sort"
	"sync"
)

// OperationBinding binds an operation ID to a provider with priority and tier access.
type OperationBinding struct {
	OperationID  string   // operation this binding applies to
	ProviderName string   // name in the underlying Registry
	Tiers        []string // which user tiers can access (empty = all)
	Priority     int      // lower = preferred
}

// OperationRegistry resolves providers for operations based on tier and priority.
// It wraps an existing Registry and adds operation-level routing with tier-based
// access control and priority ordering.
type OperationRegistry[T Provider] struct {
	mu       sync.RWMutex
	registry *Registry[T]
	bindings map[string][]OperationBinding // operationID -> bindings
}

// NewOperationRegistry creates an OperationRegistry backed by the given Registry.
func NewOperationRegistry[T Provider](registry *Registry[T]) *OperationRegistry[T] {
	return &OperationRegistry[T]{
		registry: registry,
		bindings: make(map[string][]OperationBinding),
	}
}

// Bind adds an operation binding. Multiple bindings for the same operation ID
// are allowed; they are resolved by tier match and priority order.
func (r *OperationRegistry[T]) Bind(binding OperationBinding) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bindings[binding.OperationID] = append(r.bindings[binding.OperationID], binding)
}

// Resolve finds the best provider for an operation given a user tier.
// It filters by operation ID, then by tier (empty Tiers = all tiers),
// sorts by priority (lower first), and returns the first provider that
// can be created via the underlying Registry.
func (r *OperationRegistry[T]) Resolve(operationID, tier string) (T, error) {
	r.mu.RLock()
	bindings, ok := r.bindings[operationID]
	if !ok || len(bindings) == 0 {
		r.mu.RUnlock()
		var zero T
		return zero, fmt.Errorf("provider: no bindings for operation %q", operationID)
	}

	// Filter by tier and copy to avoid holding the lock during provider creation.
	var candidates []OperationBinding
	for _, b := range bindings {
		if tierAllowed(b.Tiers, tier) {
			candidates = append(candidates, b)
		}
	}
	r.mu.RUnlock()

	if len(candidates) == 0 {
		var zero T
		return zero, fmt.Errorf("provider: no bindings for operation %q accessible by tier %q", operationID, tier)
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Priority < candidates[j].Priority
	})

	// Try each candidate in priority order.
	for _, c := range candidates {
		// Check cache first.
		if p, found := r.registry.Get(c.ProviderName); found {
			return p, nil
		}
		// Try creating via factory.
		p, err := r.registry.Create(c.ProviderName, nil)
		if err == nil {
			r.registry.Set(c.ProviderName, p)
			return p, nil
		}
	}

	var zero T
	return zero, fmt.Errorf("provider: no available provider for operation %q tier %q", operationID, tier)
}

// ListBindings returns all bindings for the given operation ID, sorted by priority.
func (r *OperationRegistry[T]) ListBindings(operationID string) []OperationBinding {
	r.mu.RLock()
	defer r.mu.RUnlock()

	src := r.bindings[operationID]
	out := make([]OperationBinding, len(src))
	copy(out, src)
	sort.Slice(out, func(i, j int) bool {
		return out[i].Priority < out[j].Priority
	})
	return out
}

// tierAllowed returns true if the tier is permitted by the binding's tier list.
// An empty tier list means all tiers are allowed.
func tierAllowed(tiers []string, tier string) bool {
	if len(tiers) == 0 {
		return true
	}
	for _, t := range tiers {
		if t == tier {
			return true
		}
	}
	return false
}
