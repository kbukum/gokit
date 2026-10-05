// Package resilience provides patterns for building fault-tolerant systems.
// It includes circuit breaker, retry, bulkhead, and rate limiting patterns.
package resilience

import (
	"context"
	"errors"
	"sync"
	"time"

	apperr "github.com/kbukum/gokit/errors"
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

// ErrCircuitOpen is a typed AppError so callers can branch on the error code and
// map to an HTTP status, while errors.Is still matches the sentinel.
var ErrCircuitOpen = apperr.New(apperr.ErrCodeServiceUnavailable, "circuit breaker is open")

// CircuitBreakerConfig configures a circuit breaker.
type CircuitBreakerConfig struct {
	// Name identifies this circuit breaker for metrics/logging.
	Name string `json:"name,omitempty" yaml:"name" mapstructure:"name"`
	// MaxFailures is the number of failures before opening the circuit.
	MaxFailures int `json:"max_failures,omitempty" yaml:"max_failures" mapstructure:"max_failures"`
	// Timeout is how long to wait before transitioning from open to half-open.
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout" mapstructure:"timeout"`
	// HalfOpenMaxCalls is the number of calls allowed in half-open state.
	HalfOpenMaxCalls int `json:"half_open_max_calls,omitempty" yaml:"half_open_max_calls" mapstructure:"half_open_max_calls"`
	// OnStateChange is called when state changes.
	OnStateChange func(name string, from, to State) `json:"-" yaml:"-" mapstructure:"-"`
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
	generation      uint64
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

// Execute runs fn through the circuit breaker or returns ErrCircuitOpen when admission is unavailable. Wrapped context.Canceled leaves breaker health unchanged and returns probe capacity. A panic propagates without recording success or retaining a probe.
func (cb *CircuitBreaker) Execute(fn func() error) (err error) {
	generation, allowed := cb.allowRequest()
	if !allowed {
		return ErrCircuitOpen
	}

	// Preserve a panicking callback's panic while returning its probe capacity.
	err = context.Canceled
	defer func() { cb.recordResult(generation, err) }()
	err = fn()
	return err
}

// State returns the current circuit breaker state.
func (cb *CircuitBreaker) State() State {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.currentState()
}

// Reset resets the circuit breaker to closed state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.toState(StateClosed)
	cb.generation++
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
func (cb *CircuitBreaker) allowRequest() (uint64, bool) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	state := cb.currentState()

	switch state {
	case StateClosed:
		return cb.generation, true
	case StateOpen:
		return cb.generation, false
	case StateHalfOpen:
		if cb.halfOpenCalls < cb.config.HalfOpenMaxCalls {
			cb.halfOpenCalls++
			return cb.generation, true
		}
		return cb.generation, false
	default:
		return cb.generation, false
	}
}

// recordResult records the result of a request.
func (cb *CircuitBreaker) recordResult(generation uint64, err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	state := cb.currentState()
	if generation != cb.generation {
		return
	}
	if errors.Is(err, context.Canceled) {
		if state == StateHalfOpen {
			cb.halfOpenCalls--
		}
		return
	}
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
	default:
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
	default:
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
	cb.generation++

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
