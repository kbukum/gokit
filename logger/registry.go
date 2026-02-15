package logger

import (
	"sync"
)

// registry is the global named-logger registry.
var registry = &loggerRegistry{
	loggers: make(map[string]*Logger),
}

type loggerRegistry struct {
	mu      sync.RWMutex
	loggers map[string]*Logger
}

// Register stores a named logger in the registry.
func Register(name string, l *Logger) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.loggers[name] = l
}

// Get retrieves a named logger. If the name is not registered it returns the
// global logger tagged with the requested component name.
func Get(name string) *Logger {
	registry.mu.RLock()
	l, ok := registry.loggers[name]
	registry.mu.RUnlock()
	if ok {
		return l
	}
	return GetGlobalLogger().WithComponent(name)
}

// RegisterDefaults registers a set of named loggers from the global config.
// Call this after Init() to seed the registry with common component loggers.
func RegisterDefaults(names ...string) {
	for _, name := range names {
		Register(name, GetGlobalLogger().WithComponent(name))
	}
}
