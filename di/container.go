package di

import (
	"context"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/kbukum/gokit/logging"
)

// RegistrationMode determines how a component should be resolved
type RegistrationMode int

const (
	Eager     RegistrationMode = iota // Initialize immediately on registration
	Lazy                              // Initialize on first resolve (cached)
	Singleton                         // Pre-created instance
	Transient                         // New instance per resolve (never cached)
)

// Container defines the interface for a dependency injection container.
//
// Type-safe resolution helpers are provided as free generic functions in
// this package: [Resolve], [MustResolve], [TryResolve]. The interface itself
// stays untyped so that alternative implementations remain easy to write;
// callers are expected to use the generic helpers rather than the raw
// untyped Resolve.
type Container interface {
	Register(key string, constructor any) error
	RegisterLazy(key string, constructor any, options ...LazyOption) error
	RegisterEager(key string, constructor any) error
	RegisterTransient(key string, constructor any) error
	Resolve(key string) (any, error)
	RegisterSingleton(key string, instance any) error
	Close() error

	// Introspection
	Registrations() []RegistrationInfo

	// Cache & lifecycle controls. These are first-class operations on a DI
	// container — not legacy: callers reload config-driven singletons via
	// [Container.InvalidateCache]/[Container.Refresh], and adapters that need
	// a deferred resolution closure use [Container.GetResolver]. Type-safe
	// resolution (where a generic helper is more ergonomic) is provided by the
	// package-level [Resolve], [MustResolve], and [TryResolve] helpers.
	InvalidateCache(name string) error
	Refresh(name string) (any, error)
	GetResolver(name string) func() (any, error)
}

// RegistrationInfo describes a registered component for introspection.
type RegistrationInfo struct {
	Key         string
	Mode        RegistrationMode // Eager, Lazy, or Singleton
	Initialized bool
}

// UnifiedContainer is our single, unified DI container
type UnifiedContainer struct {
	components map[string]*ComponentRegistration
	singletons map[string]any
	mutex      sync.RWMutex
	// resolving tracks keys currently being resolved to detect circular dependencies.
	// Each goroutine gets its own set via goroutine-local tracking.
	resolvingMu sync.Mutex
	resolving   map[uint64]map[string]bool
}

type ComponentRegistration struct {
	key            string
	constructor    any
	mode           RegistrationMode
	instance       any
	mutex          sync.RWMutex
	initialized    bool
	lastError      error
	retryPolicy    *RetryPolicy
	circuitBreaker *CircuitBreaker
}

type RetryPolicy struct {
	MaxAttempts       int
	InitialBackoffMs  int
	MaxBackoffMs      int
	BackoffMultiplier float64
}

type CircuitBreaker struct {
	failureCount    int64
	successCount    int64
	state           CircuitState
	lastFailureTime time.Time
	config          *CircuitBreakerConfig
	mutex           sync.RWMutex
}

type CircuitState int

const (
	CircuitClosed CircuitState = iota
	CircuitOpen
	CircuitHalfOpen
)

type CircuitBreakerConfig struct {
	FailureThreshold  int
	RecoveryTimeoutMs int
	HalfOpenRequests  int
}

type LazyOption func(*ComponentRegistration)

func NewContainer() Container {
	return &UnifiedContainer{
		components: make(map[string]*ComponentRegistration),
		singletons: make(map[string]any),
		resolving:  make(map[uint64]map[string]bool),
	}
}

// Register component with lazy loading by default (most common case)
func (c *UnifiedContainer) Register(key string, constructor any) error {
	return c.RegisterLazy(key, constructor)
}

// RegisterLazy registers a component for lazy initialization
func (c *UnifiedContainer) RegisterLazy(key string, constructor any, options ...LazyOption) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	registration := &ComponentRegistration{
		key:            key,
		constructor:    constructor,
		mode:           Lazy,
		retryPolicy:    defaultRetryPolicy(),
		circuitBreaker: NewCircuitBreaker(defaultCircuitBreakerConfig()),
	}

	// Apply options
	for _, opt := range options {
		opt(registration)
	}

	c.components[key] = registration
	return nil
}

// RegisterEager registers a component for immediate initialization
func (c *UnifiedContainer) RegisterEager(key string, constructor any) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	registration := &ComponentRegistration{
		key:         key,
		constructor: constructor,
		mode:        Eager,
	}

	// Initialize immediately
	instance, err := c.callConstructor(constructor)
	if err != nil {
		return fmt.Errorf("failed to initialize eager component '%s': %w", key, err)
	}

	registration.instance = instance
	registration.initialized = true

	c.components[key] = registration
	return nil
}

// RegisterSingleton registers a pre-created instance
func (c *UnifiedContainer) RegisterSingleton(key string, instance any) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.singletons[key] = instance
	return nil
}

// RegisterTransient registers a constructor that creates a new instance on every Resolve call.
// The result is never cached.
func (c *UnifiedContainer) RegisterTransient(key string, constructor any) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	registration := &ComponentRegistration{
		key:         key,
		constructor: constructor,
		mode:        Transient,
	}

	c.components[key] = registration
	return nil
}

// Resolve gets a component instance
func (c *UnifiedContainer) Resolve(key string) (any, error) {
	// Check singletons first
	c.mutex.RLock()
	if singleton, exists := c.singletons[key]; exists {
		c.mutex.RUnlock()
		return singleton, nil
	}

	registration, exists := c.components[key]
	c.mutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("component not registered: %s", key)
	}

	// Circular dependency detection
	gid := goroutineID()
	if err := c.pushResolving(gid, key); err != nil {
		return nil, err
	}
	defer c.popResolving(gid, key)

	return c.resolveComponent(registration)
}

// goroutineID extracts the goroutine ID from the runtime stack.
func goroutineID() uint64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	// Stack starts with "goroutine NNN [..."
	s := string(buf[:n])
	s = strings.TrimPrefix(s, "goroutine ")
	var id uint64
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		id = id*10 + uint64(c-'0')
	}
	return id
}

// pushResolving marks key as being resolved on this goroutine. Returns an error
// if a cycle is detected.
func (c *UnifiedContainer) pushResolving(gid uint64, key string) error {
	c.resolvingMu.Lock()
	defer c.resolvingMu.Unlock()

	set, ok := c.resolving[gid]
	if !ok {
		set = make(map[string]bool)
		c.resolving[gid] = set
	}
	if set[key] {
		return fmt.Errorf("circular dependency detected: resolving %q is already in progress on this goroutine", key)
	}
	set[key] = true
	return nil
}

// popResolving removes the key from the goroutine's resolving set.
func (c *UnifiedContainer) popResolving(gid uint64, key string) {
	c.resolvingMu.Lock()
	defer c.resolvingMu.Unlock()

	if set, ok := c.resolving[gid]; ok {
		delete(set, key)
		if len(set) == 0 {
			delete(c.resolving, gid)
		}
	}
}

func (c *UnifiedContainer) resolveComponent(registration *ComponentRegistration) (any, error) {
	switch registration.mode {
	case Eager:
		return c.resolveEager(registration)
	case Lazy:
		return c.resolveLazy(registration)
	case Transient:
		return c.resolveTransient(registration)
	default:
		return nil, fmt.Errorf("unknown registration mode for component: %s", registration.key)
	}
}

func (c *UnifiedContainer) resolveEager(registration *ComponentRegistration) (any, error) {
	registration.mutex.RLock()
	if registration.initialized && registration.instance != nil {
		instance := registration.instance
		registration.mutex.RUnlock()
		return instance, nil
	}
	registration.mutex.RUnlock()

	return nil, fmt.Errorf("eager component not properly initialized: %s", registration.key)
}

func (c *UnifiedContainer) resolveLazy(registration *ComponentRegistration) (any, error) {
	// Circuit breaker check
	if registration.circuitBreaker.IsOpen() {
		return nil, fmt.Errorf("circuit breaker open for component: %s", registration.key)
	}

	// Try to get cached instance
	registration.mutex.RLock()
	if registration.initialized && registration.instance != nil && registration.lastError == nil {
		instance := registration.instance
		registration.mutex.RUnlock()
		return instance, nil
	}
	registration.mutex.RUnlock()

	// Initialize with retry logic
	return c.initializeWithRetry(registration)
}

// resolveTransient creates a new instance every time. No caching.
func (c *UnifiedContainer) resolveTransient(registration *ComponentRegistration) (any, error) {
	return c.callConstructor(registration.constructor)
}

func (c *UnifiedContainer) initializeWithRetry(registration *ComponentRegistration) (any, error) {
	var lastError error
	backoffMs := registration.retryPolicy.InitialBackoffMs

	for attempt := 0; attempt < registration.retryPolicy.MaxAttempts; attempt++ {
		if attempt > 0 {
			// Sleep outside the lock to avoid blocking concurrent Resolve() calls
			time.Sleep(time.Duration(backoffMs) * time.Millisecond)
			backoffMs = int(float64(backoffMs) * registration.retryPolicy.BackoffMultiplier)
			if backoffMs > registration.retryPolicy.MaxBackoffMs {
				backoffMs = registration.retryPolicy.MaxBackoffMs
			}
		}

		registration.mutex.Lock()

		// Double-check: another goroutine may have succeeded while we slept
		if registration.initialized && registration.instance != nil && registration.lastError == nil {
			instance := registration.instance
			registration.mutex.Unlock()
			return instance, nil
		}

		// Try to construct (unlock first to avoid holding lock during potentially slow constructor)
		registration.mutex.Unlock()
		instance, err := c.callConstructor(registration.constructor)
		if err != nil {
			lastError = err
			// Circular dependency errors are deterministic — retrying won't help.
			if strings.Contains(err.Error(), "circular dependency detected") {
				return nil, fmt.Errorf("failed to initialize component '%s': %w",
					registration.key, err)
			}
			registration.circuitBreaker.RecordFailure()
			logging.Debug("Lazy component initialization failed", map[string]any{
				"component": registration.key,
				"attempt":   attempt + 1,
				"error":     err.Error(),
			})
			continue
		}

		// Success — acquire lock to store the result
		registration.mutex.Lock()
		// Double-check again: another goroutine may have initialized while we were constructing
		if registration.initialized && registration.instance != nil && registration.lastError == nil {
			existing := registration.instance
			registration.mutex.Unlock()
			return existing, nil
		}
		registration.instance = instance
		registration.initialized = true
		registration.lastError = nil
		registration.mutex.Unlock()
		registration.circuitBreaker.RecordSuccess()

		logging.Info("Lazy component initialized successfully", map[string]any{
			"component": registration.key,
			"attempts":  attempt + 1,
		})

		return instance, nil
	}

	// All attempts failed
	registration.mutex.Lock()
	registration.lastError = lastError
	registration.mutex.Unlock()
	registration.circuitBreaker.RecordFailure()

	return nil, fmt.Errorf("failed to initialize lazy component '%s' after %d attempts: %w",
		registration.key, registration.retryPolicy.MaxAttempts, lastError)
}

func (c *UnifiedContainer) callConstructor(constructor any) (any, error) {
	fn := reflect.ValueOf(constructor)
	if fn.Kind() != reflect.Func {
		return nil, fmt.Errorf("constructor must be a function")
	}

	fnType := fn.Type()

	// Handle different constructor signatures
	switch fnType.NumIn() {
	case 0:
		// Simple constructor: func() (Service, error) or func() Service
		results := fn.Call(nil)
		return c.handleConstructorResults(results)

	case 1:
		// Context-aware constructor: func(context.Context) (Service, error)
		if fnType.In(0).String() == "context.Context" {
			ctx := context.Background()
			results := fn.Call([]reflect.Value{reflect.ValueOf(ctx)})
			return c.handleConstructorResults(results)
		}
		fallthrough

	default:
		// DI-aware constructor: func(Container) (Service, error)
		if fnType.NumIn() > 1 {
			return nil, fmt.Errorf("constructors with %d arguments are not supported; use 0 or 1 parameter", fnType.NumIn())
		}
		results := fn.Call([]reflect.Value{reflect.ValueOf(c)})
		return c.handleConstructorResults(results)
	}
}

func (c *UnifiedContainer) handleConstructorResults(results []reflect.Value) (any, error) {
	switch len(results) {
	case 1:
		// Constructor returns just the instance
		return results[0].Interface(), nil
	case 2:
		// Constructor returns (instance, error)
		instance := results[0].Interface()
		if err := results[1].Interface(); err != nil {
			return nil, err.(error)
		}
		return instance, nil
	default:
		return nil, fmt.Errorf("constructor must return either (instance) or (instance, error)")
	}
}

// Registrations returns info about all registered components for introspection.
func (c *UnifiedContainer) Registrations() []RegistrationInfo {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	result := make([]RegistrationInfo, 0, len(c.components)+len(c.singletons))

	for key, reg := range c.components {
		reg.mutex.RLock()
		result = append(result, RegistrationInfo{
			Key:         key,
			Mode:        reg.mode,
			Initialized: reg.initialized,
		})
		reg.mutex.RUnlock()
	}

	for key := range c.singletons {
		result = append(result, RegistrationInfo{
			Key:         key,
			Mode:        Singleton,
			Initialized: true,
		})
	}

	return result
}

func (c *UnifiedContainer) Close() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	var errs []error

	// Close all initialized lazy components that implement closer
	for _, registration := range c.components {
		if registration.initialized && registration.instance != nil {
			if closer, ok := registration.instance.(interface{ Close() error }); ok {
				if err := closer.Close(); err != nil {
					errs = append(errs, fmt.Errorf("close component: %w", err))
				}
			}
		}
	}

	// Close singletons that implement closer
	for _, singleton := range c.singletons {
		if singleton == nil {
			continue
		}
		if closer, ok := singleton.(interface{ Close() error }); ok {
			if err := closer.Close(); err != nil {
				errs = append(errs, fmt.Errorf("close singleton: %w", err))
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("container close errors: %v", errs)
	}
	return nil
}

// Cache & lifecycle controls — see [Container] interface docs.

// InvalidateCache drops the cached instance for a registered component (or removes
// a registered singleton). The next [Container.Resolve] call will re-run the
// constructor. Returns an error if the name is not registered.
func (c *UnifiedContainer) InvalidateCache(name string) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if registration, exists := c.components[name]; exists {
		registration.mutex.Lock()
		registration.initialized = false
		registration.instance = nil
		registration.lastError = nil
		registration.mutex.Unlock()
		return nil
	}

	if _, exists := c.singletons[name]; exists {
		delete(c.singletons, name)
		return nil
	}

	return fmt.Errorf("component '%s' not registered", name)
}

// Refresh invalidates the cached instance for name and resolves it again,
// returning the freshly constructed value. Useful after configuration changes.
func (c *UnifiedContainer) Refresh(name string) (any, error) {
	if err := c.InvalidateCache(name); err != nil {
		return nil, err
	}
	return c.Resolve(name)
}

// GetResolver returns a closure that resolves the named component lazily on
// each call. Adapter code that must hand out a `func() (any, error)`
// (e.g., third-party plug-in loaders) uses this instead of capturing the
// container directly.
func (c *UnifiedContainer) GetResolver(name string) func() (any, error) {
	return func() (any, error) {
		return c.Resolve(name)
	}
}

// Helper functions and options
func WithRetryPolicy(policy *RetryPolicy) LazyOption {
	return func(reg *ComponentRegistration) {
		reg.retryPolicy = policy
	}
}

func WithCircuitBreaker(config *CircuitBreakerConfig) LazyOption {
	return func(reg *ComponentRegistration) {
		reg.circuitBreaker = NewCircuitBreaker(config)
	}
}

func defaultRetryPolicy() *RetryPolicy {
	return &RetryPolicy{
		MaxAttempts:       3,
		InitialBackoffMs:  1000,
		MaxBackoffMs:      30000,
		BackoffMultiplier: 2.0,
	}
}

func defaultCircuitBreakerConfig() *CircuitBreakerConfig {
	return &CircuitBreakerConfig{
		FailureThreshold:  5,
		RecoveryTimeoutMs: 60000,
		HalfOpenRequests:  3,
	}
}

func NewCircuitBreaker(config *CircuitBreakerConfig) *CircuitBreaker {
	return &CircuitBreaker{
		state:  CircuitClosed,
		config: config,
	}
}

func (cb *CircuitBreaker) IsOpen() bool {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	if cb.state == CircuitOpen {
		// Check if recovery timeout has passed
		if time.Since(cb.lastFailureTime) > time.Duration(cb.config.RecoveryTimeoutMs)*time.Millisecond {
			cb.state = CircuitHalfOpen
			return false
		}
		return true
	}

	return false
}

func (cb *CircuitBreaker) RecordSuccess() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.successCount++
	cb.failureCount = 0
	cb.state = CircuitClosed
}

func (cb *CircuitBreaker) RecordFailure() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.failureCount++
	cb.lastFailureTime = time.Now()

	if cb.failureCount >= int64(cb.config.FailureThreshold) {
		cb.state = CircuitOpen
	}
}

// NewSimpleContainer is an alias for [NewContainer] retained as the canonical
// constructor name used by application bootstrap code. Both return the same
// [*UnifiedContainer] type satisfying [Container].
func NewSimpleContainer() Container {
	return NewContainer()
}

// ResolveTyped provides type-safe resolution with generics.
func ResolveTyped[T any](container Container, name string) (T, error) {
	instance, err := container.Resolve(name)
	if err != nil {
		var zero T
		return zero, err
	}
	typed, ok := instance.(T)
	if !ok {
		var zero T
		return zero, fmt.Errorf("di: type mismatch for '%s': want %T, got %T", name, zero, instance)
	}
	return typed, nil
}

// GetTypedResolver returns a type-safe resolver function.
func GetTypedResolver[T any](container Container, name string) func() (T, error) {
	return func() (T, error) {
		return ResolveTyped[T](container, name)
	}
}
