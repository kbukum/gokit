package auth

import (
	"fmt"
	"sync"
)

// Registry is a thread-safe registry of named TokenValidator instances.
// Projects register their validators (JWT, OIDC, API key, etc.) by name
// and retrieve them in middleware or interceptors.
//
// Usage:
//
//	reg := auth.NewRegistry()
//	reg.Register("jwt", jwtSvc.AsValidator())
//	reg.Register("apikey", auth.TokenValidatorFunc(myAPIKeyValidator))
//	reg.SetDefault("jwt")
//
//	// In middleware setup
//	validator, _ := reg.Default()
type Registry struct {
	mu          sync.RWMutex
	validators  map[string]TokenValidator
	defaultName string
}

// NewRegistry creates a new empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		validators: make(map[string]TokenValidator),
	}
}

// Register adds a named TokenValidator to the registry.
// If this is the first validator registered, it becomes the default.
func (r *Registry) Register(name string, v TokenValidator) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.validators[name] = v
	if r.defaultName == "" {
		r.defaultName = name
	}
}

// Get returns the TokenValidator registered under the given name.
// Returns nil and false if not found.
func (r *Registry) Get(name string) (TokenValidator, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.validators[name]
	return v, ok
}

// MustGet returns the TokenValidator registered under the given name.
// Panics if the name is not registered.
func (r *Registry) MustGet(name string) TokenValidator {
	v, ok := r.Get(name)
	if !ok {
		panic(fmt.Sprintf("auth: validator %q not registered", name))
	}
	return v
}

// Default returns the default TokenValidator.
// The default is the first registered validator unless overridden with SetDefault.
func (r *Registry) Default() (TokenValidator, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.defaultName == "" {
		return nil, false
	}
	v, ok := r.validators[r.defaultName]
	return v, ok
}

// SetDefault sets the default validator by name.
// The name must already be registered.
func (r *Registry) SetDefault(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.validators[name]; !ok {
		return fmt.Errorf("auth: validator %q not registered", name)
	}
	r.defaultName = name
	return nil
}

// Names returns all registered validator names.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.validators))
	for name := range r.validators {
		names = append(names, name)
	}
	return names
}
