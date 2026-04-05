package middleware

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kbukum/gokit/messaging"
	"github.com/kbukum/gokit/resilience"
)

func TestCircuitBreakerHandler_ClosedPassesThrough(t *testing.T) {
	t.Parallel()

	var calls int32
	handler := func(_ context.Context, _ messaging.Message) error {
		atomic.AddInt32(&calls, 1)
		return nil
	}

	wrapped := CircuitBreakerHandler(handler, CircuitBreakerConfig{
		Name:      "test",
		Threshold: 5,
		Timeout:   time.Second,
	})

	for i := 0; i < 3; i++ {
		if err := wrapped(context.Background(), messaging.Message{}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("calls = %d, want 3", got)
	}
}

func TestCircuitBreakerHandler_OpensAfterThreshold(t *testing.T) {
	t.Parallel()

	fail := errors.New("downstream error")
	handler := func(_ context.Context, _ messaging.Message) error {
		return fail
	}

	wrapped := CircuitBreakerHandler(handler, CircuitBreakerConfig{
		Name:      "test-open",
		Threshold: 3,
		Timeout:   time.Hour,
	})

	// 3 failures to trip the breaker.
	for i := 0; i < 3; i++ {
		err := wrapped(context.Background(), messaging.Message{})
		if !errors.Is(err, fail) {
			t.Fatalf("iteration %d: error = %v, want %v", i, err, fail)
		}
	}

	// Next call should be rejected by circuit breaker.
	err := wrapped(context.Background(), messaging.Message{})
	if !errors.Is(err, resilience.ErrCircuitOpen) {
		t.Errorf("error = %v, want ErrCircuitOpen", err)
	}
}

func TestCircuitBreakerHandler_HalfOpenProbes(t *testing.T) {
	t.Parallel()

	var calls int32
	var shouldFail atomic.Value
	shouldFail.Store(true)

	handler := func(_ context.Context, _ messaging.Message) error {
		atomic.AddInt32(&calls, 1)
		if shouldFail.Load().(bool) {
			return errors.New("fail")
		}
		return nil
	}

	wrapped := CircuitBreakerHandler(handler, CircuitBreakerConfig{
		Name:        "test-halfopen",
		Threshold:   2,
		Timeout:     50 * time.Millisecond,
		HalfOpenMax: 1,
	})

	// Trip the circuit.
	_ = wrapped(context.Background(), messaging.Message{})
	_ = wrapped(context.Background(), messaging.Message{})

	// Should be open now.
	err := wrapped(context.Background(), messaging.Message{})
	if !errors.Is(err, resilience.ErrCircuitOpen) {
		t.Fatalf("expected ErrCircuitOpen, got %v", err)
	}

	// Wait for timeout → half-open.
	time.Sleep(100 * time.Millisecond)

	// Switch to success for the probe.
	shouldFail.Store(false)

	// Half-open probe should pass through and succeed → circuit closes.
	if err := wrapped(context.Background(), messaging.Message{}); err != nil {
		t.Fatalf("half-open probe error: %v", err)
	}

	// Circuit should now be closed — further calls should succeed.
	if err := wrapped(context.Background(), messaging.Message{}); err != nil {
		t.Errorf("expected nil after close, got %v", err)
	}
}

func TestCircuitBreakerHandler_DefaultConfig(t *testing.T) {
	t.Parallel()

	handler := func(_ context.Context, _ messaging.Message) error {
		return nil
	}

	// Zero-value config should use resilience defaults (5 failures, 30s timeout).
	wrapped := CircuitBreakerHandler(handler, CircuitBreakerConfig{})
	if err := wrapped(context.Background(), messaging.Message{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCircuitBreakerHandler_OnStateChange(t *testing.T) {
	t.Parallel()

	var transitions []string
	fail := errors.New("fail")

	handler := func(_ context.Context, _ messaging.Message) error {
		return fail
	}

	wrapped := CircuitBreakerHandler(handler, CircuitBreakerConfig{
		Name:      "test-callback",
		Threshold: 2,
		Timeout:   time.Hour,
		OnStateChange: func(name string, from, to resilience.State) {
			transitions = append(transitions, from.String()+"→"+to.String())
		},
	})

	_ = wrapped(context.Background(), messaging.Message{})
	_ = wrapped(context.Background(), messaging.Message{})

	if len(transitions) == 0 {
		t.Error("expected at least one state transition")
	}
	if transitions[0] != "closed→open" {
		t.Errorf("transition = %q, want closed→open", transitions[0])
	}
}
