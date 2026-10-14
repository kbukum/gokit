package storage

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/kbukum/gokit/component"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/util"
)

// Component wraps Storage and implements component.Component for lifecycle management.
type Component struct {
	mu       sync.RWMutex
	starting chan struct{}
	stopping *storageStop
	cancel   context.CancelFunc
	storage  Storage
	pending  Storage
	registry *FactoryRegistry
	cfg      Config
	log      *logging.Logger
}

type storageStop struct {
	done chan struct{}
	err  error
}

// NewComponent creates a storage component for use with the component registry.
// registry is mandatory; construct one and register the desired provider(s) before passing it.
func NewComponent(registry *FactoryRegistry, cfg Config, log *logging.Logger) *Component {
	if log == nil {
		log = logging.NewDefault("storage") //nolint:contextcheck // default logger construction has no request-scoped operation
	}
	return &Component{
		registry: registry,
		cfg:      cfg,
		log:      log.WithComponent("storage"),
	}
}

// Storage returns the underlying Storage, or nil if not started.
func (c *Component) Storage() Storage {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.storage
}

// Source returns the started backend, failing explicitly before startup and after teardown.
func (c *Component) Source() (Storage, error) {
	s := c.Storage()
	if util.IsNil(s) {
		return nil, apperrors.ServiceUnavailable("storage").WithReason("STORAGE_NOT_STARTED")
	}
	return s, nil
}

// ensure Component satisfies component.Component.
var _ component.Component = (*Component)(nil)

// Name returns the component name.
func (c *Component) Name() string { return "storage" }

// Start initializes the storage backend.
func (c *Component) Start(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	if c.stopping != nil || !util.IsNil(c.pending) {
		c.mu.Unlock()
		return apperrors.Conflict("Storage teardown is still in progress")
	}
	if !c.cfg.Enabled {
		c.mu.Unlock()
		c.log.Info("storage component is disabled") //nolint:contextcheck // lifecycle Start has no request context
		return nil
	}
	if !util.IsNil(c.storage) {
		c.mu.Unlock()
		return nil
	}
	if c.starting != nil {
		c.mu.Unlock()
		return apperrors.Conflict("Storage initialization is already in progress")
	}
	initializing, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	c.starting, c.cancel = done, cancel
	c.mu.Unlock()
	defer cancel()
	defer func() {
		c.mu.Lock()
		c.starting, c.cancel = nil, nil
		close(done)
		c.mu.Unlock()
	}()
	s, err := New(initializing, c.registry, c.cfg, c.log)
	if err != nil {
		return errors.Join(fmt.Errorf("storage start: %w", err), c.closeInitialization(ctx, s))
	}
	if util.IsNil(s) {
		return apperrors.Internal(fmt.Errorf("storage factory returned a nil backend"))
	}
	if checker, ok := s.(Checker); ok {
		if err := checker.Check(initializing); err != nil {
			return errors.Join(fmt.Errorf("storage readiness: %w", err), c.closeInitialization(ctx, s))
		}
	}
	c.mu.Lock()
	if err := initializing.Err(); err != nil {
		c.mu.Unlock()
		return errors.Join(err, c.closeInitialization(ctx, s))
	}
	c.storage = s
	c.mu.Unlock()
	return nil
}

// Stop gracefully shuts down the storage component.
func (c *Component) Stop(ctx context.Context) error {
	c.mu.Lock()
	if stopped := c.stopping; stopped != nil {
		c.mu.Unlock()
		select {
		case <-stopped.done:
			return stopped.err
		case <-ctx.Done():
			return context.Cause(ctx)
		}
	}
	stopped := &storageStop{done: make(chan struct{})}
	c.stopping = stopped
	starting, cancel := c.starting, c.cancel
	if !util.IsNil(c.storage) {
		c.pending = c.storage
	}
	c.storage = nil
	if cancel != nil {
		cancel()
	}
	c.mu.Unlock()
	var err error
	if starting != nil {
		select {
		case <-starting:
		case <-ctx.Done():
			err = errors.Join(err, context.Cause(ctx))
		}
	}
	if err == nil {
		c.mu.RLock()
		backend := c.pending
		c.mu.RUnlock()
		err = closeBackend(ctx, backend)
		if err == nil {
			c.mu.Lock()
			c.pending = nil
			c.mu.Unlock()
		}
	}
	c.mu.Lock()
	stopped.err = err
	close(stopped.done)
	c.stopping = nil
	c.mu.Unlock()
	return err
}

func (c *Component) closeInitialization(ctx context.Context, backend Storage) error {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	err := closeBackend(cleanup, backend)
	if err != nil {
		c.mu.Lock()
		c.pending = backend
		c.mu.Unlock()
	}
	return err
}

func closeBackend(ctx context.Context, backend Storage) error {
	if !util.IsNil(backend) {
		if closer, ok := backend.(Closer); ok {
			return closer.Close(ctx)
		}
	}
	return nil
}

// Health returns the current health status of the storage component.
func (c *Component) Health(ctx context.Context) component.Health {
	if !c.cfg.Enabled {
		return component.Health{
			Name:    c.Name(),
			Status:  component.StatusHealthy,
			Message: "disabled",
		}
	}

	s, err := c.Source()
	if err != nil {
		return component.Health{
			Name:    c.Name(),
			Status:  component.StatusUnhealthy,
			Message: "storage not initialized",
		}
	}

	var probeErr error
	if checker, ok := s.(Checker); ok {
		probeErr = checker.Check(ctx)
	} else {
		_, probeErr = s.URL(ctx, ".health")
	}
	if probeErr != nil {
		return component.Health{
			Name:    c.Name(),
			Status:  component.StatusUnhealthy,
			Message: fmt.Sprintf("health probe failed: %v", probeErr),
		}
	}

	return component.Health{
		Name:   c.Name(),
		Status: component.StatusHealthy,
	}
}

// Describe returns infrastructure summary info for the bootstrap display.
func (c *Component) Describe() component.Description {
	details := fmt.Sprintf("provider=%s", c.cfg.Provider)

	return component.Description{
		Name:    "Storage",
		Type:    "storage",
		Details: details,
	}
}

// BucketDescriber is optionally implemented by provider configs that use a bucket.
type BucketDescriber interface {
	GetBucket() string
}
