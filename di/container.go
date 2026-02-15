package di

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/skillsenselab/gokit/logger"
)

// RegistrationMode determines how a component should be resolved
type RegistrationMode int

const (
	Eager     RegistrationMode = iota // Initialize immediately on registration
	Lazy                              // Initialize on first resolve
	Singleton                         // Pre-created instance
)

// Container defines the interface for a dependency injection container
type Container interface {
	Register(key string, constructor interface{}) error
	RegisterLazy(key string, constructor interface{}, options ...LazyOption) error
	RegisterEager(key string, constructor interface{}) error
	Resolve(key string) (interface{}, error)
	RegisterSingleton(key string, instance interface{}) error
	Close() error

	// Introspection
	Registrations() []RegistrationInfo

	// Legacy methods for backward compatibility
	InvalidateCache(name string) error
	Refresh(name string) (interface{}, error)
	GetResolver(name string) func() (interface{}, error)
	MustResolve(name string) interface{}
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
	singletons map[string]interface{}
	mutex      sync.RWMutex
}

type ComponentRegistration struct {
	key            string
	constructor    interface{}
	mode           RegistrationMode
	instance       interface{}
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
		singletons: make(map[string]interface{}),
	}
}

// Register component with lazy loading by default (most common case)
func (c *UnifiedContainer) Register(key string, constructor interface{}) error {
	return c.RegisterLazy(key, constructor)
}

// RegisterLazy registers a component for lazy initialization
func (c *UnifiedContainer) RegisterLazy(key string, constructor interface{}, options ...LazyOption) error {
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
func (c *UnifiedContainer) RegisterEager(key string, constructor interface{}) error {
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
func (c *UnifiedContainer) RegisterSingleton(key string, instance interface{}) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.singletons[key] = instance
	return nil
}

// Resolve gets a component instance
func (c *UnifiedContainer) Resolve(key string) (interface{}, error) {
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

	return c.resolveComponent(registration)
}

func (c *UnifiedContainer) resolveComponent(registration *ComponentRegistration) (interface{}, error) {
	switch registration.mode {
	case Eager:
		return c.resolveEager(registration)
	case Lazy:
		return c.resolveLazy(registration)
	default:
		return nil, fmt.Errorf("unknown registration mode for component: %s", registration.key)
	}
}

func (c *UnifiedContainer) resolveEager(registration *ComponentRegistration) (interface{}, error) {
	registration.mutex.RLock()
	if registration.initialized && registration.instance != nil {
		instance := registration.instance
		registration.mutex.RUnlock()
		return instance, nil
	}
	registration.mutex.RUnlock()

	return nil, fmt.Errorf("eager component not properly initialized: %s", registration.key)
}

func (c *UnifiedContainer) resolveLazy(registration *ComponentRegistration) (interface{}, error) {
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

func (c *UnifiedContainer) initializeWithRetry(registration *ComponentRegistration) (interface{}, error) {
	registration.mutex.Lock()
	defer registration.mutex.Unlock()

	// Double-check pattern
	if registration.initialized && registration.instance != nil && registration.lastError == nil {
		return registration.instance, nil
	}

	var lastError error
	backoffMs := registration.retryPolicy.InitialBackoffMs

	for attempt := 0; attempt < registration.retryPolicy.MaxAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(backoffMs) * time.Millisecond)
			backoffMs = int(float64(backoffMs) * registration.retryPolicy.BackoffMultiplier)
			if backoffMs > registration.retryPolicy.MaxBackoffMs {
				backoffMs = registration.retryPolicy.MaxBackoffMs
			}
		}

		// Try to construct
		instance, err := c.callConstructor(registration.constructor)
		if err != nil {
			lastError = err
			registration.circuitBreaker.RecordFailure()
			logger.Debug("Lazy component initialization failed", map[string]interface{}{
				"component": registration.key,
				"attempt":   attempt + 1,
				"error":     err.Error(),
			})
			continue
		}

		// Success
		registration.instance = instance
		registration.initialized = true
		registration.lastError = nil
		registration.circuitBreaker.RecordSuccess()

		logger.Info("Lazy component initialized successfully", map[string]interface{}{
			"component": registration.key,
			"attempts":  attempt + 1,
		})

		return instance, nil
	}

	// All attempts failed
	registration.lastError = lastError
	registration.circuitBreaker.RecordFailure()

	return nil, fmt.Errorf("failed to initialize lazy component '%s' after %d attempts: %w",
		registration.key, registration.retryPolicy.MaxAttempts, lastError)
}

func (c *UnifiedContainer) callConstructor(constructor interface{}) (interface{}, error) {
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
		results := fn.Call([]reflect.Value{reflect.ValueOf(c)})
		return c.handleConstructorResults(results)
	}
}

func (c *UnifiedContainer) handleConstructorResults(results []reflect.Value) (interface{}, error) {
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

	// Close all initialized components that implement closer
	for _, registration := range c.components {
		if registration.initialized && registration.instance != nil {
			if closer, ok := registration.instance.(interface{ Close() error }); ok {
				closer.Close()
			}
		}
	}

	// Close singletons
	for _, singleton := range c.singletons {
		if closer, ok := singleton.(interface{ Close() error }); ok {
			closer.Close()
		}
	}

	return nil
}

// Legacy methods for backward compatibility
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

func (c *UnifiedContainer) Refresh(name string) (interface{}, error) {
	if err := c.InvalidateCache(name); err != nil {
		return nil, err
	}
	return c.Resolve(name)
}

func (c *UnifiedContainer) GetResolver(name string) func() (interface{}, error) {
	return func() (interface{}, error) {
		return c.Resolve(name)
	}
}

func (c *UnifiedContainer) MustResolve(name string) interface{} {
	instance, err := c.Resolve(name)
	if err != nil {
		panic(err)
	}
	return instance
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
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()

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

// NewSimpleContainer creates the new unified container (backward compatibility)
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
	return instance.(T), nil
}

// GetTypedResolver returns a type-safe resolver function.
func GetTypedResolver[T any](container Container, name string) func() (T, error) {
	return func() (T, error) {
		return ResolveTyped[T](container, name)
	}
}
