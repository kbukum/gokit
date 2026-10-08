package bootstrap

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/kbukum/gokit/component"
)

// moduleWiring is the state module contexts share during the modules phase.
type moduleWiring struct {
	mu        sync.Mutex
	registry  *component.Registry
	listeners map[string]Listener
	mounted   map[string]map[string]bool
	fallbacks map[string]fallback
	values    map[PortRef]any
}

func (w *moduleWiring) mount(listener, pattern string, handler http.Handler) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	// Planning matched every listener a module declares to one the App has, and Handle accepts only declared listeners.
	l := w.listeners[listener]
	if w.mounted[listener][pattern] {
		return fmt.Errorf("%w: %s %q already mounted", ErrRouteConflict, listener, pattern)
	}
	if err := l.Handle(pattern, handler); err != nil {
		return fmt.Errorf("%w: %s %q: %w", ErrRouteConflict, listener, pattern, err)
	}
	w.mounted[listener][pattern] = true
	return nil
}

func (w *moduleWiring) provide(ref PortRef, value any) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.values[ref] = value
}

func (w *moduleWiring) value(ref PortRef) any {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.values[ref]
}
