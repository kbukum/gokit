package resilience

import (
	"errors"
	"sync"
	"sync/atomic"
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

func TestCB_HalfOpenMaxCallsConcurrentProbes(t *testing.T) {
	const halfOpenMax = 3
	config := CircuitBreakerConfig{
		Name:             "ho-max",
		MaxFailures:      1,
		Timeout:          10 * time.Millisecond,
		HalfOpenMaxCalls: halfOpenMax,
	}
	cb := NewCircuitBreaker(config)

	// Trip the breaker.
	_ = cb.Execute(func() error { return errors.New("fail") })
	time.Sleep(15 * time.Millisecond)

	// Launch exactly halfOpenMax goroutines that all block until released.
	var allowed int32
	started := make(chan struct{}, halfOpenMax+1)
	release := make(chan struct{})
	var wg sync.WaitGroup

	for i := 0; i < halfOpenMax+1; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := cb.Execute(func() error {
				atomic.AddInt32(&allowed, 1)
				started <- struct{}{}
				<-release
				return nil
			})
			if errors.Is(err, ErrCircuitOpen) {
				started <- struct{}{} // signal even on rejection
			}
		}()
	}

	// Wait for all goroutines to have attempted.
	for i := 0; i < halfOpenMax+1; i++ {
		<-started
	}

	got := int(atomic.LoadInt32(&allowed))
	if got != halfOpenMax {
		t.Errorf("expected %d allowed in half-open, got %d", halfOpenMax, got)
	}
	close(release)
	wg.Wait()
}

func TestCB_HalfOpenSuccessQuotaBoundary(t *testing.T) {
	const halfOpenMax = 3
	config := CircuitBreakerConfig{
		Name:             "quota",
		MaxFailures:      1,
		Timeout:          10 * time.Millisecond,
		HalfOpenMaxCalls: halfOpenMax,
	}

	// Sub-test: n-1 successes should NOT close the circuit.
	t.Run("below_quota", func(t *testing.T) {
		cb := NewCircuitBreaker(config)
		_ = cb.Execute(func() error { return errors.New("trip") })
		time.Sleep(15 * time.Millisecond)

		for i := 0; i < halfOpenMax-1; i++ {
			_ = cb.Execute(func() error { return nil })
		}
		if cb.State() == StateClosed {
			t.Error("circuit should NOT be closed before reaching quota")
		}
	})

	// Sub-test: exactly n successes should close the circuit.
	t.Run("at_quota", func(t *testing.T) {
		cb := NewCircuitBreaker(config)
		_ = cb.Execute(func() error { return errors.New("trip") })
		time.Sleep(15 * time.Millisecond)

		for i := 0; i < halfOpenMax; i++ {
			_ = cb.Execute(func() error { return nil })
		}
		if cb.State() != StateClosed {
			t.Errorf("expected StateClosed after %d successes, got %s", halfOpenMax, cb.State())
		}
	})
}

func TestCB_RapidConcurrentLoad(t *testing.T) {
	t.Parallel()
	config := CircuitBreakerConfig{
		Name:        "concurrent",
		MaxFailures: 5,
		Timeout:     5 * time.Millisecond,
	}
	cb := NewCircuitBreaker(config)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_ = cb.Execute(func() error {
				if n%3 == 0 {
					return errors.New("fail")
				}
				return nil
			})
			_ = cb.State()
			_ = cb.Failures()
		}(i)
	}
	wg.Wait()

	// Simply verify no panics and state is valid.
	s := cb.State()
	if s != StateClosed && s != StateOpen && s != StateHalfOpen {
		t.Errorf("unexpected state: %s", s)
	}
}

func TestCB_OnStateChangeCallbackOrdering(t *testing.T) {
	type transition struct{ from, to State }
	var transitions []transition
	var mu sync.Mutex

	config := CircuitBreakerConfig{
		Name:             "order",
		MaxFailures:      1,
		Timeout:          10 * time.Millisecond,
		HalfOpenMaxCalls: 1,
		OnStateChange: func(_ string, from, to State) {
			mu.Lock()
			transitions = append(transitions, transition{from, to})
			mu.Unlock()
		},
	}
	cb := NewCircuitBreaker(config)

	// Closed → Open
	_ = cb.Execute(func() error { return errors.New("fail") })
	// Open → HalfOpen
	time.Sleep(15 * time.Millisecond)
	// HalfOpen → Closed (success)
	_ = cb.Execute(func() error { return nil })

	mu.Lock()
	defer mu.Unlock()

	expected := []transition{
		{StateClosed, StateOpen},
		{StateOpen, StateHalfOpen},
		{StateHalfOpen, StateClosed},
	}
	if len(transitions) != len(expected) {
		t.Fatalf("expected %d transitions, got %d: %v", len(expected), len(transitions), transitions)
	}
	for i, want := range expected {
		if transitions[i] != want {
			t.Errorf("transition[%d] = %v, want %v", i, transitions[i], want)
		}
	}
}

func TestCB_ExecuteWithPanic(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		Name:        "panic",
		MaxFailures: 5,
		Timeout:     time.Second,
	})

	// Execute panics – the panic propagates to the caller. We just verify it
	// does not corrupt state (no deadlock, no invalid state after recovery).
	func() {
		defer func() { _ = recover() }()
		_ = cb.Execute(func() error { panic("boom") })
	}()

	// The CB should still be usable. The panic prevented recordResult from
	// running so the failure counter should still be 0.
	if cb.Failures() != 0 {
		t.Logf("failures after panic: %d (panic bypassed recordResult)", cb.Failures())
	}
	// Must not deadlock.
	err := cb.Execute(func() error { return nil })
	if err != nil {
		t.Errorf("expected successful execution after panic recovery, got %v", err)
	}
}

func TestCB_ResetDuringHalfOpen(t *testing.T) {
	config := CircuitBreakerConfig{
		Name:             "reset-ho",
		MaxFailures:      1,
		Timeout:          10 * time.Millisecond,
		HalfOpenMaxCalls: 1,
	}
	cb := NewCircuitBreaker(config)

	_ = cb.Execute(func() error { return errors.New("trip") })
	time.Sleep(15 * time.Millisecond)

	if cb.State() != StateHalfOpen {
		t.Fatalf("expected StateHalfOpen, got %s", cb.State())
	}

	cb.Reset()

	if cb.State() != StateClosed {
		t.Errorf("expected StateClosed after Reset, got %s", cb.State())
	}
	if cb.Failures() != 0 {
		t.Errorf("expected 0 failures after Reset, got %d", cb.Failures())
	}
}

func TestCB_ZeroMaxFailuresDefaultsToFive(t *testing.T) {
	t.Parallel()
	config := CircuitBreakerConfig{
		Name:        "zero-max",
		MaxFailures: 0,
		Timeout:     time.Second,
	}
	cb := NewCircuitBreaker(config)

	// Should default to 5.
	for i := 0; i < 4; i++ {
		_ = cb.Execute(func() error { return errors.New("fail") })
	}
	if cb.State() != StateClosed {
		t.Errorf("should still be closed after 4 failures with default MaxFailures=5, got %s", cb.State())
	}
	_ = cb.Execute(func() error { return errors.New("fail") })
	if cb.State() != StateOpen {
		t.Errorf("should be open after 5 failures, got %s", cb.State())
	}
}

func TestCB_VeryShortTimeout(t *testing.T) {
	config := CircuitBreakerConfig{
		Name:        "short-timeout",
		MaxFailures: 1,
		Timeout:     1 * time.Millisecond,
	}
	cb := NewCircuitBreaker(config)

	_ = cb.Execute(func() error { return errors.New("fail") })

	// Wait well beyond 1ms.
	time.Sleep(10 * time.Millisecond)

	if s := cb.State(); s != StateHalfOpen {
		t.Errorf("expected StateHalfOpen after timeout, got %s", s)
	}
}

func TestCB_ConcurrentResetAndExecute(t *testing.T) {
	t.Parallel()
	config := CircuitBreakerConfig{
		Name:        "reset-exec",
		MaxFailures: 2,
		Timeout:     50 * time.Millisecond,
	}
	cb := NewCircuitBreaker(config)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = cb.Execute(func() error { return errors.New("fail") })
		}()
		go func() {
			defer wg.Done()
			cb.Reset()
		}()
	}
	wg.Wait()

	// No panics, no deadlocks.
	s := cb.State()
	if s != StateClosed && s != StateOpen && s != StateHalfOpen {
		t.Errorf("invalid state %s", s)
	}
}
