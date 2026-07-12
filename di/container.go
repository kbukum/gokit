package di

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
)

// Container is a type-keyed dependency injection container.
//
// Every dependency is keyed by its concrete Go type — optionally qualified by
// a name via [WithName] so that multiple values of the same type can coexist.
// Registration and resolution go through the generic package functions
// ([Register], [RegisterSingleton], [RegisterTransient], [Resolve]); the
// container itself exposes no untyped surface.
//
// Three registration modes are supported:
//
//   - Eager     — a pre-built value, returned as-is on every resolve.
//   - Singleton — a factory invoked once; the result is cached.
//   - Transient — a factory invoked fresh on every resolve.
//
// Constructor injection is the only supported wiring pattern: a factory
// receives the resolution [context.Context] and calls [Resolve] with it for
// each dependency it needs. Cyclic dependencies are detected by tracking the
// active resolution chain in that context and reported as an error rather than
// deadlocking.
//
// A [RegisterSingleton] factory is run at most once: concurrent first resolves
// are serialized, and the first result is cached for the rest. As with
// [sync.Once], a singleton whose construction depends — directly or through
// another singleton being built on a different goroutine — on its own not-yet-
// cached value cannot make progress; model mutually dependent singletons as
// transients or break the cycle.
type Container struct {
	mu      sync.RWMutex
	entries map[typeKey]*entry
}

// typeKey identifies a registration by concrete type and optional name.
type typeKey struct {
	typ  reflect.Type
	name string
}

func (k typeKey) String() string {
	if k.name != "" {
		return k.typ.String() + ":" + k.name
	}
	return k.typ.String()
}

type mode int

const (
	modeEager mode = iota
	modeSingleton
	modeTransient
)

// entry is a single registration slot.
type entry struct {
	mode     mode
	typeName string
	name     string
	factory  func(context.Context) (any, error)

	mu          sync.Mutex
	value       any
	initialized bool
}

// NewContainer returns an empty container.
func NewContainer() *Container {
	return &Container{entries: make(map[typeKey]*entry)}
}

func keyFor[T any](name string) typeKey {
	return typeKey{typ: reflect.TypeOf((*T)(nil)).Elem(), name: name}
}

func (c *Container) put(k typeKey, e *entry) {
	c.mu.Lock()
	c.entries[k] = e
	c.mu.Unlock()
}

func (c *Container) lookup(k typeKey) (*entry, bool) {
	c.mu.RLock()
	e, ok := c.entries[k]
	c.mu.RUnlock()
	return e, ok
}

// resKey is the context key under which the active resolution chain is stored.
type resKey struct{}

// resNode is one frame of the resolution chain. The chain is an immutable
// singly linked list threaded through [context.Context], so it is safe to read
// from any goroutine a factory may hand the context to.
type resNode struct {
	key    typeKey
	parent *resNode
}

func (n *resNode) contains(k typeKey) bool {
	for ; n != nil; n = n.parent {
		if n.key == k {
			return true
		}
	}
	return false
}

// resolveKey resolves the entry for k, threading cycle detection through ctx.
func (c *Container) resolveKey(ctx context.Context, k typeKey) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	e, ok := c.lookup(k)
	if !ok {
		return nil, fmt.Errorf("di: %s not registered", k)
	}

	chain, _ := ctx.Value(resKey{}).(*resNode)
	if chain.contains(k) {
		return nil, fmt.Errorf("di: circular dependency detected while resolving %s", k)
	}
	childCtx := context.WithValue(ctx, resKey{}, &resNode{key: k, parent: chain})

	return e.resolve(childCtx)
}

func (e *entry) resolve(ctx context.Context) (any, error) {
	switch e.mode {
	case modeEager:
		return e.value, nil
	case modeTransient:
		return e.factory(ctx)
	default: // modeSingleton
		e.mu.Lock()
		defer e.mu.Unlock()
		if e.initialized {
			return e.value, nil
		}
		v, err := e.factory(ctx)
		if err != nil {
			return nil, err
		}
		e.value = v
		e.initialized = true
		return v, nil
	}
}

// Close calls Close on every resolved value that implements
// interface{ Close() error }, once, and clears the container so a second call
// is a no-op. Errors are joined. Only currently-registered values are closed;
// a value dropped by re-registering its key is the caller's to close.
func (c *Container) Close() error {
	c.mu.Lock()
	entries := c.entries
	c.entries = make(map[typeKey]*entry)
	c.mu.Unlock()

	var errs []error
	for _, e := range entries {
		e.mu.Lock()
		value := e.value
		live := e.mode == modeEager || e.initialized
		e.mu.Unlock()

		if !live || value == nil {
			continue
		}
		if closer, ok := value.(interface{ Close() error }); ok {
			if err := closer.Close(); err != nil {
				errs = append(errs, fmt.Errorf("di: close %s: %w", e.displayKey(), err))
			}
		}
	}
	return errors.Join(errs...)
}

func (e *entry) displayKey() string {
	if e.name != "" {
		return e.name
	}
	return e.typeName
}
