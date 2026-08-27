package logging

import (
	"context"
	"log/slog"
	"strings"
	"sync"
)

// ModuleLevelManager holds per-module (component) log level overrides.
// It is safe for concurrent use.
type ModuleLevelManager struct {
	mu     sync.RWMutex
	levels map[string]slog.Level
}

// NewModuleLevelManager builds a manager from a map of module name to level
// string. Unrecognized level strings are ignored.
func NewModuleLevelManager(levels map[string]string) *ModuleLevelManager {
	m := &ModuleLevelManager{levels: make(map[string]slog.Level, len(levels))}
	for module, lvl := range levels {
		if parsed, ok := ParseLevel(lvl); ok {
			m.levels[strings.ToLower(module)] = parsed
		}
	}
	return m
}

// Level returns the override level for a module and whether one exists.
func (m *ModuleLevelManager) Level(module string) (slog.Level, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	lvl, ok := m.levels[strings.ToLower(module)]
	return lvl, ok
}

// SetLevel updates a module's override level at runtime. An unrecognized level
// string is ignored.
func (m *ModuleLevelManager) SetLevel(module, level string) {
	parsed, ok := ParseLevel(level)
	if !ok {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.levels[strings.ToLower(module)] = parsed
}

// moduleLevelHandler applies per-component level overrides. It captures the
// component name from the bound attributes and, when an override exists for
// that component, gates Enabled on the override instead of the base level.
type moduleLevelHandler struct {
	next      slog.Handler
	manager   *ModuleLevelManager
	component string
}

// newModuleLevelHandler wraps next with per-component level gating. When the
// manager is nil the next handler is returned unwrapped.
func newModuleLevelHandler(next slog.Handler, manager *ModuleLevelManager) slog.Handler {
	if manager == nil {
		return next
	}
	return &moduleLevelHandler{next: next, manager: manager}
}

func (h *moduleLevelHandler) Enabled(ctx context.Context, level slog.Level) bool {
	if h.component != "" {
		if override, ok := h.manager.Level(h.component); ok {
			return level >= override
		}
	}
	return h.next.Enabled(ctx, level)
}

func (h *moduleLevelHandler) Handle(ctx context.Context, rec slog.Record) error {
	return h.next.Handle(ctx, rec)
}

func (h *moduleLevelHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	component := h.component
	for _, a := range attrs {
		if a.Key == FieldComponent {
			component = a.Value.String()
		}
	}
	return &moduleLevelHandler{
		next:      h.next.WithAttrs(attrs),
		manager:   h.manager,
		component: component,
	}
}

func (h *moduleLevelHandler) WithGroup(name string) slog.Handler {
	return &moduleLevelHandler{
		next:      h.next.WithGroup(name),
		manager:   h.manager,
		component: h.component,
	}
}
