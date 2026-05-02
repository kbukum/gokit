package auth

import (
	"fmt"
	"sync"

	"github.com/kbukum/gokit/provider"
)

// Registry is a thread-safe registry of named TokenValidator instances.
// Projects register their validators (JWT, OIDC, API key, etc.) by name
// and retrieve them in middleware or interceptors.
//
// Registry is a thin wrapper around the shared provider.NamedRegistry type that adds
// auth-specific "default validator" semantics.
//
// Usage:
//
//	reg := auth.NewRegistry()
//	if err := reg.Register("jwt", jwtSvc.AsValidator()); err != nil { ... }
//	if err := reg.Register("apikey", auth.TokenValidatorFunc(myAPIKeyValidator)); err != nil { ... }
//	if err := reg.SetDefault("jwt"); err != nil { ... }
//
//	// In middleware setup
//	validator, _ := reg.Default()
type Registry struct {
	inner       *provider.NamedRegistry[TokenValidator]
	mu          sync.RWMutex
	defaultName string
}

// NewRegistry creates a new empty Registry.
func NewRegistry() *Registry {
	return &Registry{inner: provider.NewNamedRegistry[TokenValidator]("auth")}
}

// Register adds a named TokenValidator. It returns an error if name is
// empty, the validator is nil, or name is already registered. If this is
// the first successful registration, the validator becomes the default.
//
// Note: prior versions silently overwrote duplicates and returned no error.
// Callers that relied on this behavior must Unregister or use a fresh
// Registry.
func (r *Registry) Register(name string, v TokenValidator) error {
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

// Get returns the TokenValidator registered under the given name.
// Returns nil and false if not found.
func (r *Registry) Get(name string) (TokenValidator, bool) {
	return r.inner.Get(name)
}

// Default returns the default TokenValidator.
// The default is the first registered validator unless overridden with SetDefault.
func (r *Registry) Default() (TokenValidator, bool) {
	r.mu.RLock()
	name := r.defaultName
	r.mu.RUnlock()
	if name == "" {
		return nil, false
	}
	return r.inner.Get(name)
}

// SetDefault sets the default validator by name.
// The name must already be registered.
func (r *Registry) SetDefault(name string) error {
	if _, ok := r.inner.Get(name); !ok {
		return fmt.Errorf("auth: validator %q not registered", name)
	}
	r.mu.Lock()
	r.defaultName = name
	r.mu.Unlock()
	return nil
}

// Names returns all registered validator names in deterministic (sorted) order.
func (r *Registry) Names() []string {
	return r.inner.Names()
}
