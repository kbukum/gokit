// Package registry provides a single, generic, thread-safe named registry
// used by domain packages (auth, discovery, storage, tool, workload, llm).
//
// Domain packages historically each carried their own ad-hoc map+mutex
// implementation with inconsistent semantics — some panicked on duplicate,
// some returned an error, and one silently overwrote. This package replaces
// all of them with a single typed implementation whose Register always
// returns an error on programmer mistakes (empty name, nil zero value,
// duplicate name).
//
// See OSS-review issue F-015 / #45 and F-016 / #46.
package registry

import (
	"fmt"
	"reflect"
	"sort"
	"sync"
)

// Registry is a thread-safe map of named values of type T.
//
// The zero value is not usable; construct with [New].
type Registry[T any] struct {
	domain string
	mu     sync.RWMutex
	items  map[string]T
}

// New creates an empty Registry. The domain string is used in error messages
// (e.g. "auth", "discovery") so callers can identify which subsystem produced
// a registration error.
func New[T any](domain string) *Registry[T] {
	return &Registry[T]{
		domain: domain,
		items:  make(map[string]T),
	}
}

// Register adds v under name. It returns an error if name is empty, if v is
// the zero value of an interface/pointer/func type (nil), or if name is
// already registered.
func (r *Registry[T]) Register(name string, v T) error {
	if name == "" {
		return fmt.Errorf("%s: name must not be empty", r.domain)
	}
	if isNil(v) {
		return fmt.Errorf("%s: value for %q must not be nil", r.domain, name)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.items[name]; exists {
		return fmt.Errorf("%s: %q already registered", r.domain, name)
	}
	r.items[name] = v
	return nil
}

// Get returns the value registered under name and a bool indicating whether
// it was present. Callers that prefer an error should use [Registry.Lookup].
func (r *Registry[T]) Get(name string) (T, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.items[name]
	return v, ok
}

// Lookup is the error-returning counterpart to [Registry.Get]; intended for
// call sites that propagate the not-found case as an error.
func (r *Registry[T]) Lookup(name string) (T, error) {
	v, ok := r.Get(name)
	if !ok {
		var zero T
		return zero, fmt.Errorf("%s: %q not registered", r.domain, name)
	}
	return v, nil
}

// Names returns the registered names in deterministic (sorted) order.
func (r *Registry[T]) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.items))
	for k := range r.items {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Len returns the number of registered entries.
func (r *Registry[T]) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.items)
}

// Each calls fn for every registered entry. Iteration order is unspecified.
// fn must not call back into the registry (which would deadlock on the
// underlying RWMutex).
func (r *Registry[T]) Each(fn func(name string, v T)) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for k, v := range r.items {
		fn(k, v)
	}
}

// isNil reports whether v carries a nil interface/pointer/func/map/chan/slice
// payload. Plain value types (struct, string, int, bool, ...) are never nil.
func isNil[T any](v T) bool {
	rv := reflect.ValueOf(&v).Elem()
	switch rv.Kind() {
	case reflect.Interface, reflect.Ptr, reflect.Func,
		reflect.Map, reflect.Chan, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}
