// Package resilience provides patterns for building fault-tolerant systems.
// It includes circuit breaker, retry, bulkhead, and rate limiting patterns.
package resilience

import (
	"errors"
	"sync"
	"time"
)

// State represents the circuit breaker state.
type State int

const (
	// StateClosed allows requests to pass through.
	StateClosed State = iota
	// StateOpen blocks all requests.
	StateOpen
	// StateHalfOpen allows limited requests to test recovery.
	StateHalfOpen
)

// String returns the state name.
func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// Common errors.
var (
	ErrCircuitOpen     = errors.New("circuit breaker is open")
	ErrTooManyFailures = errors.New("too many failures")
)

// CircuitBreakerConfig configures a circuit breaker.
type CircuitBreakerConfig struct {
	// Name identifies this circuit breaker for metrics/logging.
	Name string
	// MaxFailures is the number of failures before opening the circuit.
	MaxFailures int
	// Timeout is how long to wait before transitioning from open to half-open.
	Timeout time.Duration
	// HalfOpenMaxCalls is the number of calls allowed in half-open state.
	HalfOpenMaxCalls int
	// OnStateChange is called when state changes.
	OnStateChange func(name string, from, to State)
}

// DefaultCircuitBreakerConfig returns sensible defaults.
func DefaultCircuitBreakerConfig(name string) CircuitBreakerConfig {
	return CircuitBreakerConfig{
		Name:             name,
		MaxFailures:      5,
		Timeout:          30 * time.Second,
		HalfOpenMaxCalls: 1,
	}
}

// CircuitBreaker implements the circuit breaker pattern.
// It prevents cascading failures by failing fast when a service is unhealthy.
//
// States:
//   - Closed: Normal operation, requests pass through
//   - Open: Service is unhealthy, requests fail immediately
//   - Half-Open: Testing if service recovered, limited requests allowed
type CircuitBreaker struct {
	config CircuitBreakerConfig

	mu              sync.RWMutex
	state           State
	failures        int
	successes       int
	lastFailureTime time.Time
	halfOpenCalls   int
}

// NewCircuitBreaker creates a new circuit breaker.
func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	if config.MaxFailures <= 0 {
		config.MaxFailures = 5
	}
	if config.Timeout <= 0 {
		config.Timeout = 30 * time.Second
	}
	if config.HalfOpenMaxCalls <= 0 {
		config.HalfOpenMaxCalls = 1
	}

	return &CircuitBreaker{
		config: config,
		state:  StateClosed,
	}
}

// Execute runs the given function through the circuit breaker.
// Returns ErrCircuitOpen if the circuit is open.
func (cb *CircuitBreaker) Execute(fn func() error) error {
	if !cb.allowRequest() {
		return ErrCircuitOpen
	}

	err := fn()
	cb.recordResult(err)
	return err
}

// State returns the current circuit breaker state.
func (cb *CircuitBreaker) State() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.currentState()
}

// Reset resets the circuit breaker to closed state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.toState(StateClosed)
	cb.failures = 0
	cb.successes = 0
	cb.halfOpenCalls = 0
}

// Failures returns the current failure count.
func (cb *CircuitBreaker) Failures() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.failures
}

// allowRequest checks if a request should be allowed.
func (cb *CircuitBreaker) allowRequest() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	state := cb.currentState()

	switch state {
	case StateClosed:
		return true
	case StateOpen:
		return false
	case StateHalfOpen:
		if cb.halfOpenCalls < cb.config.HalfOpenMaxCalls {
			cb.halfOpenCalls++
			return true
		}
		return false
	default:
		return false
	}
}

// recordResult records the result of a request.
func (cb *CircuitBreaker) recordResult(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.onFailure()
	} else {
		cb.onSuccess()
	}
}

// onSuccess handles a successful request.
func (cb *CircuitBreaker) onSuccess() {
	switch cb.currentState() {
	case StateClosed:
		cb.failures = 0
	case StateHalfOpen:
		cb.successes++
		// If we've had enough successes in half-open, close the circuit
		if cb.successes >= cb.config.HalfOpenMaxCalls {
			cb.toState(StateClosed)
		}
	}
}

// onFailure handles a failed request.
func (cb *CircuitBreaker) onFailure() {
	cb.failures++
	cb.lastFailureTime = time.Now()

	switch cb.currentState() {
	case StateClosed:
		if cb.failures >= cb.config.MaxFailures {
			cb.toState(StateOpen)
		}
	case StateHalfOpen:
		cb.toState(StateOpen)
	}
}

// currentState returns the current state, handling timeout transitions.
func (cb *CircuitBreaker) currentState() State {
	if cb.state == StateOpen {
		if time.Since(cb.lastFailureTime) >= cb.config.Timeout {
			cb.toState(StateHalfOpen)
		}
	}
	return cb.state
}

// toState transitions to a new state.
func (cb *CircuitBreaker) toState(to State) {
	if cb.state == to {
		return
	}

	from := cb.state
	cb.state = to

	// Reset counters on state change
	switch to {
	case StateClosed:
		cb.failures = 0
		cb.successes = 0
		cb.halfOpenCalls = 0
	case StateHalfOpen:
		cb.halfOpenCalls = 0
		cb.successes = 0
	case StateOpen:
		cb.halfOpenCalls = 0
		cb.successes = 0
	}

	if cb.config.OnStateChange != nil {
		cb.config.OnStateChange(cb.config.Name, from, to)
	}
}
