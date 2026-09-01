package provider

import (
	"fmt"
	"net/http"
	"sort"
	"sync"

	goerrors "github.com/kbukum/gokit/errors"
)

// Registry manages named provider factories and cached instances.
type Registry[T Provider] struct {
	mu        sync.RWMutex
	factories map[string]Factory[T]
	instances map[string]T
}

// NewRegistry creates a new empty Registry.
func NewRegistry[T Provider]() *Registry[T] {
	return &Registry[T]{
		factories: make(map[string]Factory[T]),
		instances: make(map[string]T),
	}
}

// RegisterFactory registers a named factory for creating providers.
func (r *Registry[T]) RegisterFactory(name string, factory Factory[T]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[name] = factory
}

// Create instantiates a provider using the named factory and config.
func (r *Registry[T]) Create(name string, cfg map[string]any) (T, error) {
	r.mu.RLock()
	factory, ok := r.factories[name]
	r.mu.RUnlock()
	if !ok {
		var zero T
		return zero, goerrors.New(goerrors.ErrCodeNotFound,
			fmt.Sprintf("provider factory %q not registered", name), http.StatusNotFound)
	}
	return factory(cfg)
}

// Get returns a cached provider instance by name.
func (r *Registry[T]) Get(name string) (T, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	inst, ok := r.instances[name]
	return inst, ok
}

// Set caches a provider instance by name.
func (r *Registry[T]) Set(name string, instance T) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.instances[name] = instance
}

// List returns sorted names of all registered factories.
func (r *Registry[T]) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.factories))
	for name := range r.factories {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
