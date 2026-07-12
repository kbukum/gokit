package logging

import (
	"sync"
)

// Registry is an injected collection of component-scoped loggers derived from a
// base logger. It replaces the previous package-global logger registry: callers
// construct a Registry via NewRegistry and pass it explicitly, so there is no
// mutable package-level state shared across unrelated components.
type Registry struct {
	mu      sync.RWMutex
	base    *Logger
	loggers map[string]*Logger
}

// NewRegistry creates a Registry backed by base. When base is nil the process
// default logger is used so the registry is always usable.
func NewRegistry(base *Logger) *Registry {
	if base == nil {
		base = Default()
	}
	return &Registry{
		base:    base,
		loggers: make(map[string]*Logger),
	}
}

// Register stores a named logger, overriding any previously registered or
// derived logger for that name.
func (r *Registry) Register(name string, l *Logger) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.loggers[name] = l
}

// Get returns the logger registered under name. When no logger is registered it
// derives one from the base logger tagged with the component name, caches it,
// and returns it. Get never returns nil.
func (r *Registry) Get(name string) *Logger {
	r.mu.RLock()
	l, ok := r.loggers[name]
	r.mu.RUnlock()
	if ok {
		return l
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if l, ok := r.loggers[name]; ok {
		return l
	}
	derived := r.base.WithComponent(name)
	r.loggers[name] = derived
	return derived
}

// Base returns the base logger the registry derives component loggers from.
func (r *Registry) Base() *Logger {
	return r.base
}
