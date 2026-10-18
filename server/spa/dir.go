package spa

import (
	"context"
	"errors"
	"net/http"
	"os"
	"sync"

	"github.com/kbukum/gokit/component"
	apperrors "github.com/kbukum/gokit/errors"
)

// Dir serves a built single-page app from a directory as a lifecycle component. Start opens the directory confined
// with [os.Root] and builds the handler; Stop closes it. Until Start and after Stop it answers 503, so register it
// before the listener that serves it.
type Dir struct {
	name string
	dir  string
	cfg  Config

	mu      sync.RWMutex
	root    *os.Root
	handler http.Handler
}

var _ component.Component = (*Dir)(nil)

// NewDir returns a stopped component named name that serves dir with cfg.
func NewDir(name, dir string, cfg Config) (*Dir, error) {
	if name == "" || dir == "" {
		return nil, apperrors.InvalidInput("assets", "SPA component name and directory are required")
	}
	return &Dir{name: name, dir: dir, cfg: cfg}, nil
}

// Name returns the component name.
func (d *Dir) Name() string { return d.name }

// Start opens the directory and builds the handler. It fails when the directory or its index cannot be read.
func (d *Dir) Start(context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.root != nil {
		return apperrors.InvalidInput("assets", "SPA directory is already open")
	}
	root, err := os.OpenRoot(d.dir)
	if err != nil {
		return apperrors.InvalidInput("assets", "SPA directory cannot be opened").WithCause(err)
	}
	handler, err := New(root.FS(), d.cfg)
	if err != nil {
		return errors.Join(err, root.Close())
	}
	d.root, d.handler = root, handler
	return nil
}

// Stop closes the directory. Later calls do nothing.
func (d *Dir) Stop(context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.root == nil {
		return nil
	}
	err := d.root.Close()
	d.root, d.handler = nil, nil
	return err
}

// Health is healthy while the directory is open.
func (d *Dir) Health(context.Context) component.Health {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.root == nil {
		return component.Unhealthy(d.name, "SPA directory is not open")
	}
	return component.Healthy(d.name)
}

// ServeHTTP serves the app, or 503 while the directory is not open.
func (d *Dir) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	d.mu.RLock()
	handler := d.handler
	d.mu.RUnlock()
	if handler == nil {
		http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		return
	}
	handler.ServeHTTP(w, r)
}
