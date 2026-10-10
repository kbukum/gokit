package auth

import (
	"fmt"
	"sync"

	"github.com/kbukum/gokit/provider/namedregistry"
)

// Registry is a thread-safe registry of named TokenValidator instances.
// Projects register their validators (JWT, OIDC, API key, etc.) by name
// and retrieve them in middleware or interceptors.
//
// Registry is a thin wrapper around the shared namedregistry.Registry type that adds auth-specific "default validator" semantics.
//
// Usage:
//
//	reg := auth.NewRegistry[*MyClaims]()
//	if err := reg.Register("jwt", jwtSvc); err != nil { ... }
//	if err := reg.Register("custom", auth.TokenValidatorFunc[*MyClaims](myValidator)); err != nil { ... }
//	if err := reg.SetDefault("jwt"); err != nil { ... }
//
//	// In middleware setup
//	validator, _ := reg.Default()
type Registry[T any] struct {
	inner       *namedregistry.Registry[TokenValidator[T]]
	mu          sync.RWMutex
	defaultName string
}

// NewRegistry creates a new empty Registry.
func NewRegistry[T any]() *Registry[T] {
	return &Registry[T]{inner: namedregistry.New[TokenValidator[T]]("auth")}
}

// Register adds a named TokenValidator. It returns an error if name is empty, the validator is nil,
// or name is already registered. If this is the first successful registration,
// the validator becomes the default.
func (r *Registry[T]) Register(name string, v TokenValidator[T]) error {
	if err := r.inner.Register(name, v); err != nil {
		return err
	}
	r.mu.Lock()
	if r.defaultName == "" {
		r.defaultName = name
	}
	r.mu.Unlock()
	return nil
}

// Get returns the TokenValidator registered under the given name. Returns nil
// and false if not found.
func (r *Registry[T]) Get(name string) (TokenValidator[T], bool) {
	return r.inner.Get(name)
}

// Default returns the default TokenValidator.
// The default is the first registered validator unless overridden with SetDefault.
func (r *Registry[T]) Default() (TokenValidator[T], bool) {
	r.mu.RLock()
	name := r.defaultName
	r.mu.RUnlock()
	if name == "" {
		return nil, false
	}
	return r.inner.Get(name)
}

// SetDefault sets the default validator by name. The name must already be registered.
func (r *Registry[T]) SetDefault(name string) error {
	if _, ok := r.inner.Get(name); !ok {
		return fmt.Errorf("auth: validator %q not registered", name)
	}
	r.mu.Lock()
	r.defaultName = name
	r.mu.Unlock()
	return nil
}

// Names returns all registered validator names in deterministic (sorted) order.
func (r *Registry[T]) Names() []string {
	return r.inner.Names()
}
