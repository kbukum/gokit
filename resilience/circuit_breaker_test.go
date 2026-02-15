package resilience

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestCircuitBreaker_StartsInClosedState(t *testing.T) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig("test"))

	if cb.State() != StateClosed {
		t.Errorf("expected StateClosed, got %s", cb.State())
	}
}

func TestCircuitBreaker_AllowsRequestsWhenClosed(t *testing.T) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig("test"))

	var called bool
	err := cb.Execute(func() error {
		called = true
		return nil
	})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if !called {
		t.Error("function was not called")
	}
}

func TestCircuitBreaker_OpensAfterMaxFailures(t *testing.T) {
	config := CircuitBreakerConfig{
		Name:        "test",
		MaxFailures: 3,
		Timeout:     time.Second,
	}
	cb := NewCircuitBreaker(config)

	testErr := errors.New("test error")

	// Fail 3 times
	for i := 0; i < 3; i++ {
		_ = cb.Execute(func() error {
			return testErr
		})
	}

	if cb.State() != StateOpen {
		t.Errorf("expected StateOpen, got %s", cb.State())
	}

	// Next request should fail immediately
	err := cb.Execute(func() error {
		t.Error("function should not have been called")
		return nil
	})

	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("expected ErrCircuitOpen, got %v", err)
	}
}

func TestCircuitBreaker_TransitionsToHalfOpenAfterTimeout(t *testing.T) {
	config := CircuitBreakerConfig{
		Name:        "test",
		MaxFailures: 1,
		Timeout:     50 * time.Millisecond,
	}
	cb := NewCircuitBreaker(config)

	// Trip the breaker
	_ = cb.Execute(func() error {
		return errors.New("fail")
	})

	if cb.State() != StateOpen {
		t.Errorf("expected StateOpen, got %s", cb.State())
	}

	// Wait for timeout
	time.Sleep(60 * time.Millisecond)

	if cb.State() != StateHalfOpen {
		t.Errorf("expected StateHalfOpen, got %s", cb.State())
	}
}

func TestCircuitBreaker_ClosesAfterSuccessInHalfOpen(t *testing.T) {
	config := CircuitBreakerConfig{
		Name:             "test",
		MaxFailures:      1,
		Timeout:          10 * time.Millisecond,
		HalfOpenMaxCalls: 1,
	}
	cb := NewCircuitBreaker(config)

	// Trip the breaker
	_ = cb.Execute(func() error {
		return errors.New("fail")
	})

	// Wait for half-open
	time.Sleep(15 * time.Millisecond)

	// Successful call should close circuit
	err := cb.Execute(func() error {
		return nil
	})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if cb.State() != StateClosed {
		t.Errorf("expected StateClosed, got %s", cb.State())
	}
}

func TestCircuitBreaker_ReopensOnFailureInHalfOpen(t *testing.T) {
	config := CircuitBreakerConfig{
		Name:             "test",
		MaxFailures:      1,
		Timeout:          10 * time.Millisecond,
		HalfOpenMaxCalls: 1,
	}
	cb := NewCircuitBreaker(config)

	// Trip the breaker
	_ = cb.Execute(func() error {
		return errors.New("fail")
	})

	// Wait for half-open
	time.Sleep(15 * time.Millisecond)

	// Fail in half-open
	_ = cb.Execute(func() error {
		return errors.New("fail again")
	})

	if cb.State() != StateOpen {
		t.Errorf("expected StateOpen, got %s", cb.State())
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	config := CircuitBreakerConfig{
		Name:        "test",
		MaxFailures: 1,
		Timeout:     time.Hour, // Long timeout
	}
	cb := NewCircuitBreaker(config)

	// Trip the breaker
	_ = cb.Execute(func() error {
		return errors.New("fail")
	})

	if cb.State() != StateOpen {
		t.Errorf("expected StateOpen, got %s", cb.State())
	}

	cb.Reset()

	if cb.State() != StateClosed {
		t.Errorf("expected StateClosed after reset, got %s", cb.State())
	}

	if cb.Failures() != 0 {
		t.Errorf("expected 0 failures after reset, got %d", cb.Failures())
	}
}

func TestCircuitBreaker_StateChangeCallback(t *testing.T) {
	var stateChanges []struct{ from, to State }
	var mu sync.Mutex

	config := CircuitBreakerConfig{
		Name:        "test",
		MaxFailures: 1,
		Timeout:     10 * time.Millisecond,
		OnStateChange: func(name string, from, to State) {
			mu.Lock()
			stateChanges = append(stateChanges, struct{ from, to State }{from, to})
			mu.Unlock()
		},
	}
	cb := NewCircuitBreaker(config)

	// Trip the breaker
	_ = cb.Execute(func() error {
		return errors.New("fail")
	})

	// Wait for half-open
	time.Sleep(15 * time.Millisecond)
	_ = cb.State() // Trigger state check

	mu.Lock()
	defer mu.Unlock()

	if len(stateChanges) < 2 {
		t.Fatalf("expected at least 2 state changes, got %d", len(stateChanges))
	}

	if stateChanges[0].from != StateClosed || stateChanges[0].to != StateOpen {
		t.Errorf("expected Closed->Open, got %s->%s", stateChanges[0].from, stateChanges[0].to)
	}

	if stateChanges[1].from != StateOpen || stateChanges[1].to != StateHalfOpen {
		t.Errorf("expected Open->HalfOpen, got %s->%s", stateChanges[1].from, stateChanges[1].to)
	}
}

func TestCircuitBreaker_ConcurrentAccess(t *testing.T) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig("test"))

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = cb.Execute(func() error {
				return nil
			})
			_ = cb.State()
			_ = cb.Failures()
		}()
	}
	wg.Wait()

	// Should still be closed after all successes
	if cb.State() != StateClosed {
		t.Errorf("expected StateClosed, got %s", cb.State())
	}
}

func TestState_String(t *testing.T) {
	tests := []struct {
		state State
		want  string
	}{
		{StateClosed, "closed"},
		{StateOpen, "open"},
		{StateHalfOpen, "half-open"},
		{State(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("State(%d).String() = %s, want %s", tt.state, got, tt.want)
		}
	}
}
