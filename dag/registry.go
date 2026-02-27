package dag

import (
	"sort"
	"sync"
)

// Registry provides named node lookup for dynamic graph construction.
type Registry struct {
	mu    sync.RWMutex
	nodes map[string]Node
}

// NewRegistry creates a new empty Registry.
func NewRegistry() *Registry {
	return &Registry{nodes: make(map[string]Node)}
}

// Register adds a node to the registry.
func (r *Registry) Register(name string, node Node) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nodes[name] = node
}

// Get retrieves a node by name.
func (r *Registry) Get(name string) (Node, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	n, ok := r.nodes[name]
	return n, ok
}

// List returns sorted names of all registered nodes.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.nodes))
	for name := range r.nodes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
