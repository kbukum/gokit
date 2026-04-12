package logger

import (
	"strings"
	"sync"

	"github.com/rs/zerolog"
)

// ModuleLevelManager manages per-module log level overrides.
// It is safe for concurrent use.
type ModuleLevelManager struct {
	levels map[string]zerolog.Level
	mu     sync.RWMutex
}

// NewModuleLevelManager creates a manager from a map of module names to level strings.
// Unrecognized level strings are silently ignored.
func NewModuleLevelManager(levels map[string]string) *ModuleLevelManager {
	m := &ModuleLevelManager{
		levels: make(map[string]zerolog.Level, len(levels)),
	}
	for module, lvl := range levels {
		parsed, err := zerolog.ParseLevel(strings.ToLower(lvl))
		if err == nil {
			m.levels[strings.ToLower(module)] = parsed
		}
	}
	return m
}

// Level returns the override level for a module.
// The second return value is false if no override exists.
func (m *ModuleLevelManager) Level(module string) (zerolog.Level, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	lvl, ok := m.levels[strings.ToLower(module)]
	return lvl, ok
}

// SetLevel dynamically sets a module's log level.
// An unrecognized level string is silently ignored.
func (m *ModuleLevelManager) SetLevel(module string, level string) {
	parsed, err := zerolog.ParseLevel(strings.ToLower(level))
	if err != nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.levels[strings.ToLower(module)] = parsed
}
