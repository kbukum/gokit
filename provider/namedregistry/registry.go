// Package namedregistry provides a lightweight, thread-safe registry for named values.
package namedregistry

import (
	"fmt"
	"reflect"
	"sort"
	"sync"
)

// Registry is a thread-safe map of named values.
//
// Use it for explicit provider, adapter, dialect, or callable registration when
// the registered value is not itself a Provider. Provider implementations should
// use provider.Registry instead.
type Registry[T any] struct {
	domain string
	mu     sync.RWMutex
	items  map[string]T
}

type item[T any] struct {
	name  string
	value T
}

// New creates an empty Registry. The domain is used in error messages.
func New[T any](domain string) *Registry[T] {
	return &Registry[T]{
		domain: domain,
		items:  make(map[string]T),
	}
}

// Register adds v under name.
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

// Get returns the value registered under name and whether it was present.
func (r *Registry[T]) Get(name string) (T, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.items[name]
	return v, ok
}

// Lookup returns the value registered under name or an error when missing.
func (r *Registry[T]) Lookup(name string) (T, error) {
	v, ok := r.Get(name)
	if !ok {
		var zero T
		return zero, fmt.Errorf("%s: %q not registered", r.domain, name)
	}
	return v, nil
}

// Names returns the registered names in deterministic order.
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
func (r *Registry[T]) Each(fn func(name string, v T)) {
	r.mu.RLock()
	items := make([]item[T], 0, len(r.items))
	for k, v := range r.items {
		items = append(items, item[T]{name: k, value: v})
	}
	r.mu.RUnlock()

	for _, item := range items {
		fn(item.name, item.value)
	}
}

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
