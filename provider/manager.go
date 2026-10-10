package provider

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	goerrors "github.com/kbukum/gokit/errors"
)

// Manager provides the main API for working with providers, combining a Registry for storage
// and a Selector for choosing providers.
type Manager[T Provider] struct {
	mu          sync.RWMutex
	registry    *Registry[T]
	selector    Selector[T]
	providers   map[string]T
	defaultName string
	log         *slog.Logger
}

// ManagerOption configures a Manager.
type ManagerOption[T Provider] func(*Manager[T])

// WithLogger sets the logger for the Manager. If not provided, a default no-op logger is used.
func WithLogger[T Provider](l *slog.Logger) ManagerOption[T] {
	return func(m *Manager[T]) {
		if l != nil {
			m.log = l
		}
	}
}

// NewManager creates a Manager backed by the given registry and selector.
func NewManager[T Provider](registry *Registry[T], selector Selector[T], opts ...ManagerOption[T]) *Manager[T] {
	m := &Manager[T]{
		registry:  registry,
		selector:  selector,
		providers: make(map[string]T),
		log:       slog.Default(),
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Register adds a factory to the underlying registry.
func (m *Manager[T]) Register(name string, factory Factory[T]) {
	m.registry.RegisterFactory(name, factory)
	m.log.Info("factory registered", "provider", name)
}

// Initialize creates a provider from its factory,
// calls Init() if the provider implements Initializable, and stores it for use.
func (m *Manager[T]) Initialize(name string, cfg map[string]any) error {
	return m.InitializeWithContext(context.Background(), name, cfg)
}

// InitializeWithResilience creates a provider from its factory,
// wraps it with the given middleware function, calls Init(), and stores it for use.
// The wrap function applies resilience (or any other middleware) to the provider. Example:
//
//	mgr.InitializeWithResilience(ctx, "http", nil, func(p MyProvider) (MyProvider, error) {
//	    return provider.WithResilience(p, resilienceCfg)
//	})
//
// A wrap error leaves the provider unpublished and closes it when it is Closeable.
func (m *Manager[T]) InitializeWithResilience(ctx context.Context, name string, cfg map[string]any, wrap func(T) (T, error)) error {
	instance, err := m.registry.Create(name, cfg)
	if err != nil {
		return fmt.Errorf("initialize provider %q: %w", name, err)
	}

	// Call Init() if the provider supports it (before wrapping)
	if init, ok := any(instance).(Initializable); ok {
		if err = init.Init(ctx); err != nil {
			return fmt.Errorf("init provider %q: %w", name, err)
		}
	}

	// Apply middleware wrapper
	if wrap != nil {
		wrapped, wrapErr := wrap(instance)
		if wrapErr != nil {
			wrapErr = fmt.Errorf("wrap provider %q: %w", name, wrapErr)
			return errors.Join(wrapErr, closeUnpublished(ctx, name, instance))
		}
		instance = wrapped
	}

	m.mu.Lock()
	m.providers[name] = instance
	m.mu.Unlock()
	m.registry.Set(name, instance)
	m.log.InfoContext(ctx, "provider initialized with resilience", "provider", name)
	return nil
}

// unpublishedCloseTimeout bounds cleanup of a provider that never reached the manager.
const unpublishedCloseTimeout = 5 * time.Second

// closeUnpublished releases an initialized provider whose publication failed;
// nothing else owns it, so CloseAll could never reach it.
func closeUnpublished[T any](ctx context.Context, name string, instance T) error {
	c, ok := any(instance).(Closeable)
	if !ok {
		return nil
	}
	closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), unpublishedCloseTimeout)
	defer cancel()
	if err := c.Close(closeCtx); err != nil {
		return fmt.Errorf("close unpublished provider %q: %w", name, err)
	}
	return nil
}

// InitializeWithContext creates a provider from its factory,
// calls Init() if the provider implements Initializable, and stores it for use.
func (m *Manager[T]) InitializeWithContext(ctx context.Context, name string, cfg map[string]any) error {
	instance, err := m.registry.Create(name, cfg)
	if err != nil {
		return fmt.Errorf("initialize provider %q: %w", name, err)
	}

	// Call Init() if the provider supports it
	if init, ok := any(instance).(Initializable); ok {
		if err = init.Init(ctx); err != nil {
			return fmt.Errorf("init provider %q: %w", name, err)
		}
	}

	m.mu.Lock()
	m.providers[name] = instance
	m.mu.Unlock()
	m.registry.Set(name, instance)
	m.log.InfoContext(ctx, "provider initialized", "provider", name)
	return nil
}

// Get returns a provider chosen by the selector, or the default if set.
func (m *Manager[T]) Get(ctx context.Context) (T, error) {
	m.mu.RLock()
	defaultName := m.defaultName
	providers := m.snapshotLocked()
	m.mu.RUnlock()

	if defaultName != "" {
		if p, ok := providers[defaultName]; ok {
			return p, nil
		}
		var zero T
		return zero, goerrors.New(goerrors.ErrCodeNotFound,
			fmt.Sprintf("default provider %q not found", defaultName))
	}
	return m.selector.Select(ctx, providers)
}

// GetByName returns a specific provider by name.
func (m *Manager[T]) GetByName(name string) (T, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if p, ok := m.providers[name]; ok {
		return p, nil
	}
	var zero T
	return zero, goerrors.New(goerrors.ErrCodeNotFound,
		fmt.Sprintf("provider %q not found", name))
}

// SetDefault sets the default provider by name.
func (m *Manager[T]) SetDefault(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.providers[name]; !ok {
		return goerrors.New(goerrors.ErrCodeNotFound,
			fmt.Sprintf("provider %q not initialized", name))
	}
	m.defaultName = name
	m.log.Info("default provider set", "provider", name)
	return nil
}

// Available returns the names of all initialized providers.
func (m *Manager[T]) Available() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.providers))
	for name := range m.providers {
		names = append(names, name)
	}
	return names
}

// snapshotLocked returns a shallow copy of the providers map.
// Must be called while holding at least a read lock.
func (m *Manager[T]) snapshotLocked() map[string]T {
	cp := make(map[string]T, len(m.providers))
	for k, v := range m.providers {
		cp[k] = v
	}
	return cp
}

// CloseAll calls Close() on all providers that implement Closeable.
func (m *Manager[T]) CloseAll(ctx context.Context) error {
	m.mu.RLock()
	snapshot := m.snapshotLocked()
	m.mu.RUnlock()

	var errs []error
	for name, p := range snapshot {
		if c, ok := any(p).(Closeable); ok {
			if err := c.Close(ctx); err != nil {
				errs = append(errs, fmt.Errorf("close provider %q: %w", name, err))
			} else {
				m.log.InfoContext(ctx, "provider closed", "provider", name)
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("close errors: %v", errs)
	}
	return nil
}
