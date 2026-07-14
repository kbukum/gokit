package skill

import (
	"fmt"
	"io"
	"sort"
	"sync"
)

type Provider interface {
	Manifest() Manifest
	OpenAsset(name string) (io.ReadCloser, error)
}

type Registry interface {
	Register(provider Provider) error
	Get(name string) (Provider, bool)
	List() []Manifest
}

type MemoryRegistry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

func NewRegistry() *MemoryRegistry { return &MemoryRegistry{providers: map[string]Provider{}} }

func (r *MemoryRegistry) Register(provider Provider) error {
	if provider == nil {
		return fmt.Errorf("skill: provider is nil")
	}
	m := provider.Manifest()
	if err := Validate(&m); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.providers[m.Name]; exists {
		return fmt.Errorf("%w: provider %q", ErrAlreadyRegistered, m.Name)
	}
	r.providers[m.Name] = provider
	return nil
}

func (r *MemoryRegistry) Get(name string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[name]
	return p, ok
}

func (r *MemoryRegistry) List() []Manifest {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Manifest, 0, len(r.providers))
	for _, p := range r.providers {
		out = append(out, p.Manifest())
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}
