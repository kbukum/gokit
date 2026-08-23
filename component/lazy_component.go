package component

import (
	"context"
	"fmt"
	"sync"

	"github.com/kbukum/gokit/logging"
)

// BaseLazyComponent provides thread-safe lazy initialization for components that defer expensive setup until first use.
type BaseLazyComponent struct {
	name        string
	mu          sync.RWMutex
	initialized bool
	lastError   error
	initializer func(ctx context.Context) error
	healthCheck func(ctx context.Context) error
	closer      func() error
}

// NewBaseLazyComponent creates a lazy component with the given initializer.
func NewBaseLazyComponent(name string, initializer func(context.Context) error) *BaseLazyComponent {
	return &BaseLazyComponent{
		name:        name,
		initializer: initializer,
	}
}

// Name returns the component name.
func (b *BaseLazyComponent) Name() string {
	return b.name
}

// Initialize performs thread-safe lazy initialization using double-check locking.
func (b *BaseLazyComponent) Initialize(ctx context.Context) error {
	b.mu.RLock()
	if b.initialized && b.lastError == nil {
		b.mu.RUnlock()
		return nil
	}
	b.mu.RUnlock()

	b.mu.Lock()
	defer b.mu.Unlock()

	// Double-check after acquiring write lock
	if b.initialized && b.lastError == nil {
		return nil
	}

	if b.initializer == nil {
		return fmt.Errorf("no initializer for component: %s", b.name)
	}

	logging.DebugCtx(ctx, "Initializing lazy component", map[string]any{
		"component": b.name,
	})

	if err := b.initializer(ctx); err != nil {
		b.lastError = err
		return fmt.Errorf("failed to initialize %s: %w", b.name, err)
	}

	b.initialized = true
	b.lastError = nil

	logging.DebugCtx(ctx, "Lazy component initialized", map[string]any{
		"component": b.name,
	})
	return nil
}

// IsInitialized returns whether the component has been successfully initialized.
func (b *BaseLazyComponent) IsInitialized() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.initialized && b.lastError == nil
}

// HealthCheck verifies the component is initialized and optionally runs a custom check.
func (b *BaseLazyComponent) HealthCheck(ctx context.Context) error {
	if !b.IsInitialized() {
		return fmt.Errorf("component %s not initialized", b.name)
	}
	if b.healthCheck != nil {
		return b.healthCheck(ctx)
	}
	return nil
}

// Close shuts down the component and marks it as uninitialized.
func (b *BaseLazyComponent) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closer != nil && b.initialized {
		err := b.closer()
		b.initialized = false
		return err
	}
	b.initialized = false
	return nil
}

// WithHealthCheck sets a custom health check function.
func (b *BaseLazyComponent) WithHealthCheck(fn func(context.Context) error) *BaseLazyComponent {
	b.healthCheck = fn
	return b
}

// WithCloser sets a custom close function.
func (b *BaseLazyComponent) WithCloser(fn func() error) *BaseLazyComponent {
	b.closer = fn
	return b
}

// LazyComponent wraps a component factory so the underlying Component is not constructed
// until Start is first called. This defers expensive construction (opening connections,
// allocating pools) out of registration and into the boot sequence, while still presenting
// a normal Component to the Registry.
//
// It differs from BaseLazyComponent, which defers an initializer on an already-constructed
// component; LazyComponent defers construction of the Component itself.
type LazyComponent struct {
	name    string
	factory func() Component
	mu      sync.Mutex
	inner   Component
}

// NewLazyComponent returns a LazyComponent that builds its delegate via factory on first Start.
func NewLazyComponent(name string, factory func() Component) *LazyComponent {
	return &LazyComponent{name: name, factory: factory}
}

// Name returns the component name.
func (l *LazyComponent) Name() string { return l.name }

// Start constructs the delegate (once) and starts it.
func (l *LazyComponent) Start(ctx context.Context) error {
	l.mu.Lock()
	if l.inner == nil {
		l.inner = l.factory()
	}
	inner := l.inner
	l.mu.Unlock()
	return inner.Start(ctx)
}

// Stop stops the delegate if it was constructed; a never-started LazyComponent is a no-op.
func (l *LazyComponent) Stop(ctx context.Context) error {
	l.mu.Lock()
	inner := l.inner
	l.mu.Unlock()
	if inner == nil {
		return nil
	}
	return inner.Stop(ctx)
}

// Health reports the delegate's health, or a degraded "not started" report before first Start.
func (l *LazyComponent) Health(ctx context.Context) Health {
	l.mu.Lock()
	inner := l.inner
	l.mu.Unlock()
	if inner == nil {
		return Degraded(l.name, "not started")
	}
	return inner.Health(ctx)
}
