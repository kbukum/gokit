package middleware

import (
	"context"
	"time"

	"github.com/kbukum/gokit/messaging"
	"github.com/kbukum/gokit/resilience"
)

// CircuitBreakerConfig configures the circuit breaker middleware for message handlers.
// It maps directly to resilience.CircuitBreakerConfig.
type CircuitBreakerConfig struct {
	// Name identifies this circuit breaker (used in state-change callbacks).
	Name string

	// Threshold is the number of consecutive failures before opening the circuit. Default: 5.
	Threshold int

	// Timeout is how long to wait before transitioning from open to half-open. Default: 30s.
	Timeout time.Duration

	// HalfOpenMax is the maximum number of probe requests allowed in half-open state. Default: 2.
	HalfOpenMax int

	// OnStateChange is an optional callback invoked on circuit state transitions.
	OnStateChange func(name string, from, to resilience.State)
}

// CircuitBreakerHandler wraps a MessageHandler with circuit breaker logic powered by resilience.CircuitBreaker.
// When the circuit is open, messages are rejected immediately with resilience.ErrCircuitOpen.
func CircuitBreakerHandler(handler messaging.MessageHandler, cfg CircuitBreakerConfig) messaging.MessageHandler {
	rcfg := resilience.CircuitBreakerConfig{
		Name:             cfg.Name,
		MaxFailures:      cfg.Threshold,
		Timeout:          cfg.Timeout,
		HalfOpenMaxCalls: cfg.HalfOpenMax,
		OnStateChange:    cfg.OnStateChange,
	}

	cb := resilience.NewCircuitBreaker(rcfg)

	return func(ctx context.Context, msg messaging.Message) error {
		return cb.Execute(func() error {
			return handler(ctx, msg)
		})
	}
}
