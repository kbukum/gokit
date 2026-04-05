package di

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// 1. Concurrent Resolve with slow constructors
// ---------------------------------------------------------------------------

func TestConcurrentResolveSlowConstructor(t *testing.T) {
	c := NewContainer()
	var callCount atomic.Int32

	_ = c.RegisterLazy("slow", func() string {
		callCount.Add(1)
		time.Sleep(50 * time.Millisecond)
		return "slow-value"
	}, WithRetryPolicy(&RetryPolicy{MaxAttempts: 1, InitialBackoffMs: 1, MaxBackoffMs: 1, BackoffMultiplier: 1}))

	var wg sync.WaitGroup
	errs := make([]error, 20)
	vals := make([]interface{}, 20)

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			v, err := c.Resolve("slow")
			vals[idx] = v
			errs[idx] = err
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: unexpected error: %v", i, err)
		}
		if vals[i] != "slow-value" {
			t.Errorf("goroutine %d: expected 'slow-value', got %v", i, vals[i])
		}
	}
}

// ---------------------------------------------------------------------------
// 2. Concurrent singleton reads
// ---------------------------------------------------------------------------

func TestConcurrentSingletonReads(t *testing.T) {
	c := NewContainer()
	_ = c.RegisterSingleton("shared", "singleton-value")

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			val, err := c.Resolve("shared")
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if val != "singleton-value" {
				t.Errorf("expected 'singleton-value', got %v", val)
			}
		}()
	}
	wg.Wait()
}

// ---------------------------------------------------------------------------
// 3. Register same key twice with different modes
// ---------------------------------------------------------------------------

func TestRegisterSameKeyDifferentModes(t *testing.T) {
	t.Run("lazy_then_eager", func(t *testing.T) {
		c := NewContainer()
		_ = c.RegisterLazy("svc", func() string { return "lazy" },
			WithRetryPolicy(&RetryPolicy{MaxAttempts: 1, InitialBackoffMs: 1, MaxBackoffMs: 1, BackoffMultiplier: 1}))
		_ = c.RegisterEager("svc", func() string { return "eager" })

		val, err := c.Resolve("svc")
		if err != nil {
			t.Fatalf("Resolve failed: %v", err)
		}
		if val != "eager" {
			t.Errorf("expected 'eager', got %v", val)
		}
	})

	t.Run("eager_then_lazy", func(t *testing.T) {
		c := NewContainer()
		_ = c.RegisterEager("svc", func() string { return "eager" })
		_ = c.RegisterLazy("svc", func() string { return "lazy" },
			WithRetryPolicy(&RetryPolicy{MaxAttempts: 1, InitialBackoffMs: 1, MaxBackoffMs: 1, BackoffMultiplier: 1}))

		val, err := c.Resolve("svc")
		if err != nil {
			t.Fatalf("Resolve failed: %v", err)
		}
		if val != "lazy" {
			t.Errorf("expected 'lazy', got %v", val)
		}
	})

	t.Run("singleton_then_lazy", func(t *testing.T) {
		c := NewContainer()
		_ = c.RegisterSingleton("svc", "singleton-val")
		_ = c.RegisterLazy("svc", func() string { return "lazy" },
			WithRetryPolicy(&RetryPolicy{MaxAttempts: 1, InitialBackoffMs: 1, MaxBackoffMs: 1, BackoffMultiplier: 1}))

		// Singleton map is checked first in Resolve
		val, err := c.Resolve("svc")
		if err != nil {
			t.Fatalf("Resolve failed: %v", err)
		}
		if val != "singleton-val" {
			t.Errorf("expected 'singleton-val' (singleton priority), got %v", val)
		}
	})
}

// ---------------------------------------------------------------------------
// 4. Close() error aggregation with multiple failures
// ---------------------------------------------------------------------------

type errCloser struct {
	msg string
}

func (e *errCloser) Close() error { return fmt.Errorf("%s", e.msg) }

func TestCloseErrorAggregation(t *testing.T) {
	c := NewContainer()
	_ = c.RegisterSingleton("a", &errCloser{msg: "error-a"})
	_ = c.RegisterSingleton("b", &errCloser{msg: "error-b"})

	err := c.Close()
	if err == nil {
		t.Fatal("expected error from Close()")
	}
	if !strings.Contains(err.Error(), "error-a") {
		t.Errorf("expected 'error-a' in error, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "error-b") {
		t.Errorf("expected 'error-b' in error, got %q", err.Error())
	}
}

func TestCloseWithMixedCloseableNonCloseable(t *testing.T) {
	c := NewContainer()
	closed := false
	_ = c.RegisterSingleton("closeable", &mockCloser{onClose: func() error {
		closed = true
		return nil
	}})
	_ = c.RegisterSingleton("plain", "just-a-string")

	if err := c.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if !closed {
		t.Error("expected closeable to be closed")
	}
}

func TestCloseNoCloseables(t *testing.T) {
	c := NewContainer()
	_ = c.RegisterSingleton("a", "val-a")
	_ = c.RegisterSingleton("b", 42)

	if err := c.Close(); err != nil {
		t.Fatalf("Close with no closeables should succeed, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// 5. Constructor with context.Context parameter
// ---------------------------------------------------------------------------

func TestConstructorWithContext(t *testing.T) {
	c := NewContainer()
	_ = c.RegisterLazy("ctx-svc", func(ctx context.Context) (string, error) {
		if ctx == nil {
			return "", fmt.Errorf("context is nil")
		}
		return "ctx-value", nil
	}, WithRetryPolicy(&RetryPolicy{MaxAttempts: 1, InitialBackoffMs: 1, MaxBackoffMs: 1, BackoffMultiplier: 1}))

	val, err := c.Resolve("ctx-svc")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if val != "ctx-value" {
		t.Errorf("expected 'ctx-value', got %v", val)
	}
}

// ---------------------------------------------------------------------------
// 6. Constructor with Container parameter (DI-aware)
// ---------------------------------------------------------------------------

func TestConstructorWithContainerParam(t *testing.T) {
	c := NewContainer()
	_ = c.RegisterSingleton("config-val", "db://localhost")

	_ = c.RegisterLazy("db-svc", func(container Container) (string, error) {
		cfg, err := container.Resolve("config-val")
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("connected to %s", cfg), nil
	}, WithRetryPolicy(&RetryPolicy{MaxAttempts: 1, InitialBackoffMs: 1, MaxBackoffMs: 1, BackoffMultiplier: 1}))

	val, err := c.Resolve("db-svc")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if val != "connected to db://localhost" {
		t.Errorf("expected 'connected to db://localhost', got %v", val)
	}
}

// ---------------------------------------------------------------------------
// 7. Resolve unregistered — error message format
// ---------------------------------------------------------------------------

func TestResolveUnregisteredErrorFormat(t *testing.T) {
	c := NewContainer()
	_, err := c.Resolve("does-not-exist")
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "does-not-exist") {
		t.Errorf("error should include component name, got %q", msg)
	}
	if !strings.Contains(msg, "not registered") {
		t.Errorf("error should include 'not registered', got %q", msg)
	}
}

// ---------------------------------------------------------------------------
// 8. Registrations with mixed types and introspection
// ---------------------------------------------------------------------------

func TestRegistrationsMixedTypes(t *testing.T) {
	c := NewContainer()
	_ = c.RegisterSingleton("s1", "value1")
	_ = c.RegisterSingleton("s2", 42)
	_ = c.RegisterLazy("l1", func() string { return "lazy1" },
		WithRetryPolicy(&RetryPolicy{MaxAttempts: 1, InitialBackoffMs: 1, MaxBackoffMs: 1, BackoffMultiplier: 1}))
	_ = c.RegisterEager("e1", func() int { return 99 })

	regs := c.Registrations()
	if len(regs) != 4 {
		t.Fatalf("expected 4 registrations, got %d", len(regs))
	}

	regMap := make(map[string]RegistrationInfo)
	for _, r := range regs {
		regMap[r.Key] = r
	}

	if r := regMap["s1"]; r.Mode != Singleton || !r.Initialized {
		t.Errorf("s1: expected Singleton+initialized, got mode=%d init=%v", r.Mode, r.Initialized)
	}
	if r := regMap["l1"]; r.Mode != Lazy || r.Initialized {
		t.Errorf("l1: expected Lazy+uninitialized, got mode=%d init=%v", r.Mode, r.Initialized)
	}
	if r := regMap["e1"]; r.Mode != Eager || !r.Initialized {
		t.Errorf("e1: expected Eager+initialized, got mode=%d init=%v", r.Mode, r.Initialized)
	}
}

// ---------------------------------------------------------------------------
// 9. Registrations ordering consistency
// ---------------------------------------------------------------------------

func TestRegistrationsOrderingConsistency(t *testing.T) {
	c := NewContainer()
	keys := []string{"alpha", "beta", "gamma", "delta", "epsilon"}
	for _, k := range keys {
		_ = c.RegisterSingleton(k, k+"-value")
	}

	// Run multiple times to check consistency
	for i := 0; i < 5; i++ {
		regs := c.Registrations()
		if len(regs) != len(keys) {
			t.Fatalf("iteration %d: expected %d registrations, got %d", i, len(keys), len(regs))
		}
		// Sort to verify all keys are present (maps are unordered)
		gotKeys := make([]string, len(regs))
		for j, r := range regs {
			gotKeys[j] = r.Key
		}
		sort.Strings(gotKeys)
		sort.Strings(keys)
		for j := range keys {
			if gotKeys[j] != keys[j] {
				t.Errorf("iteration %d: key mismatch at %d: %q vs %q", i, j, gotKeys[j], keys[j])
			}
		}
	}
}

// ---------------------------------------------------------------------------
// 10. Large container (100+ registrations) performance
// ---------------------------------------------------------------------------

func TestLargeContainer(t *testing.T) {
	c := NewContainer()

	for i := 0; i < 150; i++ {
		key := fmt.Sprintf("component-%d", i)
		val := fmt.Sprintf("value-%d", i)
		_ = c.RegisterSingleton(key, val)
	}

	regs := c.Registrations()
	if len(regs) != 150 {
		t.Fatalf("expected 150 registrations, got %d", len(regs))
	}

	// Resolve all of them
	for i := 0; i < 150; i++ {
		key := fmt.Sprintf("component-%d", i)
		val, err := c.Resolve(key)
		if err != nil {
			t.Fatalf("Resolve(%q) failed: %v", key, err)
		}
		expected := fmt.Sprintf("value-%d", i)
		if val != expected {
			t.Errorf("Resolve(%q) = %v, want %v", key, val, expected)
		}
	}
}

// ---------------------------------------------------------------------------
// 11. Concurrent Register and Resolve (race detection)
// ---------------------------------------------------------------------------

func TestConcurrentRegisterAndResolve(t *testing.T) {
	c := NewContainer()
	_ = c.RegisterSingleton("base", "base-value")

	var wg sync.WaitGroup
	// Writers: register new components
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := fmt.Sprintf("concurrent-%d", idx)
			_ = c.RegisterSingleton(key, fmt.Sprintf("val-%d", idx))
		}(i)
	}
	// Readers: resolve existing component
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = c.Resolve("base")
		}()
	}
	wg.Wait()
}

// ---------------------------------------------------------------------------
// 12. Lazy component with retry — constructor fails then succeeds
// ---------------------------------------------------------------------------

func TestLazyRetryFailThenSucceed(t *testing.T) {
	c := NewContainer()
	var attempts atomic.Int32

	_ = c.RegisterLazy("flaky", func() (string, error) {
		n := attempts.Add(1)
		if n < 3 {
			return "", fmt.Errorf("attempt %d failed", n)
		}
		return "finally", nil
	}, WithRetryPolicy(&RetryPolicy{
		MaxAttempts:       5,
		InitialBackoffMs:  1,
		MaxBackoffMs:      5,
		BackoffMultiplier: 1.0,
	}))

	val, err := c.Resolve("flaky")
	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if val != "finally" {
		t.Errorf("expected 'finally', got %v", val)
	}
}

// ---------------------------------------------------------------------------
// 13. Circuit breaker blocks resolves after failures
// ---------------------------------------------------------------------------

func TestCircuitBreakerBlocksResolve(t *testing.T) {
	c := NewContainer()

	_ = c.RegisterLazy("always-fails", func() (string, error) {
		return "", fmt.Errorf("fail")
	}, WithRetryPolicy(&RetryPolicy{
		MaxAttempts:       1,
		InitialBackoffMs:  1,
		MaxBackoffMs:      1,
		BackoffMultiplier: 1.0,
	}), WithCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold:  2,
		RecoveryTimeoutMs: 60000,
		HalfOpenRequests:  1,
	}))

	// First resolve fails, records failure
	_, _ = c.Resolve("always-fails")
	// Second resolve fails, records failure → circuit opens
	_, _ = c.Resolve("always-fails")

	// Third resolve should hit open circuit
	_, err := c.Resolve("always-fails")
	if err == nil {
		t.Fatal("expected error due to open circuit breaker")
	}
	if !strings.Contains(err.Error(), "circuit breaker open") {
		t.Errorf("expected circuit breaker error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// 14. Circuit breaker state transitions
// ---------------------------------------------------------------------------

func TestCircuitBreakerStateTransitions(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold:  2,
		RecoveryTimeoutMs: 10, // 10ms recovery
		HalfOpenRequests:  1,
	})

	// Starts closed
	if cb.IsOpen() {
		t.Error("should start closed")
	}

	// Two failures → open
	cb.RecordFailure()
	cb.RecordFailure()
	if !cb.IsOpen() {
		t.Error("should be open after threshold")
	}

	// Wait for recovery
	time.Sleep(15 * time.Millisecond)
	if cb.IsOpen() {
		t.Error("should transition to half-open")
	}

	// Verify state is half-open
	cb.mutex.RLock()
	state := cb.state
	cb.mutex.RUnlock()
	if state != CircuitHalfOpen {
		t.Errorf("expected CircuitHalfOpen, got %d", state)
	}

	// Success resets to closed
	cb.RecordSuccess()
	cb.mutex.RLock()
	state = cb.state
	cb.mutex.RUnlock()
	if state != CircuitClosed {
		t.Errorf("expected CircuitClosed after success, got %d", state)
	}
}

// ---------------------------------------------------------------------------
// 15. Constructor returning only instance (no error)
// ---------------------------------------------------------------------------

func TestConstructorSingleReturn(t *testing.T) {
	c := NewContainer()
	_ = c.RegisterEager("single-ret", func() int { return 42 })
	val, err := c.Resolve("single-ret")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if val != 42 {
		t.Errorf("expected 42, got %v", val)
	}
}

// ---------------------------------------------------------------------------
// 16. Constructor returning instance and nil error
// ---------------------------------------------------------------------------

func TestConstructorWithNilError(t *testing.T) {
	c := NewContainer()
	_ = c.RegisterEager("dual-ret", func() (string, error) {
		return "ok", nil
	})
	val, err := c.Resolve("dual-ret")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if val != "ok" {
		t.Errorf("expected 'ok', got %v", val)
	}
}

// ---------------------------------------------------------------------------
// 17. InvalidateCache then re-resolve triggers new constructor call
// ---------------------------------------------------------------------------

func TestInvalidateCacheReResolve(t *testing.T) {
	c := NewContainer()
	var count atomic.Int32

	_ = c.RegisterLazy("counter", func() int {
		return int(count.Add(1))
	}, WithRetryPolicy(&RetryPolicy{MaxAttempts: 1, InitialBackoffMs: 1, MaxBackoffMs: 1, BackoffMultiplier: 1}))

	v1, _ := c.Resolve("counter")
	if v1 != 1 {
		t.Errorf("first resolve: expected 1, got %v", v1)
	}

	_ = c.InvalidateCache("counter")
	v2, _ := c.Resolve("counter")
	if v2 != 2 {
		t.Errorf("after invalidation: expected 2, got %v", v2)
	}
}

// ---------------------------------------------------------------------------
// 18. ResolveTyped with various types
// ---------------------------------------------------------------------------

type myService struct {
	Name string
}

func TestResolveTypedInterface(t *testing.T) {
	c := NewContainer()
	svc := &myService{Name: "test-svc"}
	_ = c.RegisterSingleton("svc", svc)

	resolved, err := ResolveTyped[*myService](c, "svc")
	if err != nil {
		t.Fatalf("ResolveTyped failed: %v", err)
	}
	if resolved.Name != "test-svc" {
		t.Errorf("expected 'test-svc', got %q", resolved.Name)
	}
}

func TestResolveTypedMismatch(t *testing.T) {
	c := NewContainer()
	_ = c.RegisterSingleton("num", 42)

	_, err := ResolveTyped[string](c, "num")
	if err == nil {
		t.Fatal("expected type mismatch error")
	}
	if !strings.Contains(err.Error(), "type mismatch") {
		t.Errorf("expected 'type mismatch' in error, got %q", err.Error())
	}
}

// ---------------------------------------------------------------------------
// 19. GetTypedResolver concurrent access
// ---------------------------------------------------------------------------

func TestGetTypedResolverConcurrent(t *testing.T) {
	c := NewContainer()
	_ = c.RegisterSingleton("val", 42)

	resolver := GetTypedResolver[int](c, "val")
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := resolver()
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if v != 42 {
				t.Errorf("expected 42, got %d", v)
			}
		}()
	}
	wg.Wait()
}

// ---------------------------------------------------------------------------
// 20. TryResolve returns false for type mismatch
// ---------------------------------------------------------------------------

func TestTryResolveTypeMismatch(t *testing.T) {
	c := NewContainer()
	_ = c.RegisterSingleton("num", 42)

	_, ok := TryResolve[string](c, "num")
	if ok {
		t.Error("expected TryResolve to return false for type mismatch")
	}
}

// ---------------------------------------------------------------------------
// 21. RegisterEager with error-returning constructor
// ---------------------------------------------------------------------------

func TestRegisterEagerErrorConstructor(t *testing.T) {
	c := NewContainer()
	err := c.RegisterEager("fail", func() (string, error) {
		return "", fmt.Errorf("init boom")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "init boom") {
		t.Errorf("expected 'init boom' in error, got %q", err.Error())
	}
}

// ---------------------------------------------------------------------------
// 22. Singleton overrides component on Resolve
// ---------------------------------------------------------------------------

func TestSingletonPriority(t *testing.T) {
	c := NewContainer()
	_ = c.RegisterLazy("item", func() string { return "from-lazy" },
		WithRetryPolicy(&RetryPolicy{MaxAttempts: 1, InitialBackoffMs: 1, MaxBackoffMs: 1, BackoffMultiplier: 1}))
	_ = c.RegisterSingleton("item", "from-singleton")

	val, err := c.Resolve("item")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if val != "from-singleton" {
		t.Errorf("expected 'from-singleton', got %v", val)
	}
}

// ---------------------------------------------------------------------------
// 23. Refresh on lazy component
// ---------------------------------------------------------------------------

func TestRefreshLazy(t *testing.T) {
	c := NewContainer()
	var count atomic.Int32

	_ = c.RegisterLazy("refreshable", func() int {
		return int(count.Add(1))
	}, WithRetryPolicy(&RetryPolicy{MaxAttempts: 1, InitialBackoffMs: 1, MaxBackoffMs: 1, BackoffMultiplier: 1}))

	v1, _ := c.Resolve("refreshable")
	v2, err := c.Refresh("refreshable")
	if err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}
	if v2 == v1 {
		t.Errorf("expected different value after refresh, both were %v", v1)
	}
}

// ---------------------------------------------------------------------------
// 24. Close with lazy closeable (resolved)
// ---------------------------------------------------------------------------

func TestCloseLazyResolved(t *testing.T) {
	c := NewContainer()
	closed := false

	_ = c.RegisterLazy("closer", func() interface{} {
		return &mockCloser{onClose: func() error {
			closed = true
			return nil
		}}
	}, WithRetryPolicy(&RetryPolicy{MaxAttempts: 1, InitialBackoffMs: 1, MaxBackoffMs: 1, BackoffMultiplier: 1}))

	// Resolve to initialize
	_, _ = c.Resolve("closer")

	if err := c.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if !closed {
		t.Error("expected lazy closeable to be closed after resolve")
	}
}

// ---------------------------------------------------------------------------
// 25. Close with lazy closeable (NOT resolved — should not close)
// ---------------------------------------------------------------------------

func TestCloseLazyUnresolved(t *testing.T) {
	c := NewContainer()
	closed := false

	_ = c.RegisterLazy("closer", func() interface{} {
		return &mockCloser{onClose: func() error {
			closed = true
			return nil
		}}
	}, WithRetryPolicy(&RetryPolicy{MaxAttempts: 1, InitialBackoffMs: 1, MaxBackoffMs: 1, BackoffMultiplier: 1}))

	// Do NOT resolve
	if err := c.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if closed {
		t.Error("lazy component that was never resolved should NOT be closed")
	}
}

// ---------------------------------------------------------------------------
// 26. Constructor with too many parameters
// ---------------------------------------------------------------------------

func TestConstructorTooManyParams(t *testing.T) {
	c := NewContainer()
	err := c.RegisterEager("bad", func(a, b string) string { return a + b })
	if err == nil {
		t.Fatal("expected error for constructor with 2 params")
	}
}

// ---------------------------------------------------------------------------
// 27. Concurrent lazy initialization — double-check locking
// ---------------------------------------------------------------------------

func TestConcurrentLazyDoubleCheckLocking(t *testing.T) {
	c := NewContainer()
	var callCount atomic.Int32

	_ = c.RegisterLazy("once", func() string {
		callCount.Add(1)
		time.Sleep(20 * time.Millisecond)
		return "initialized"
	}, WithRetryPolicy(&RetryPolicy{MaxAttempts: 1, InitialBackoffMs: 1, MaxBackoffMs: 1, BackoffMultiplier: 1}))

	var wg sync.WaitGroup
	results := make([]string, 30)
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			val, err := c.Resolve("once")
			if err != nil {
				t.Errorf("goroutine %d error: %v", idx, err)
				return
			}
			results[idx] = val.(string)
		}(i)
	}
	wg.Wait()

	for i, r := range results {
		if r != "initialized" {
			t.Errorf("goroutine %d: expected 'initialized', got %q", i, r)
		}
	}
}

// ---------------------------------------------------------------------------
// 28. Resolve generic helpers with unregistered key
// ---------------------------------------------------------------------------

func TestResolveGenericUnregistered(t *testing.T) {
	c := NewContainer()

	_, err := Resolve[string](c, "missing")
	if err == nil {
		t.Error("expected error")
	}

	_, ok := TryResolve[string](c, "missing")
	if ok {
		t.Error("expected false from TryResolve")
	}
}

// ---------------------------------------------------------------------------
// 29. Large container with lazy components (100+)
// ---------------------------------------------------------------------------

func TestLargeContainerLazy(t *testing.T) {
	c := NewContainer()

	for i := 0; i < 100; i++ {
		idx := i
		_ = c.RegisterLazy(fmt.Sprintf("lazy-%d", idx), func() string {
			return fmt.Sprintf("value-%d", idx)
		}, WithRetryPolicy(&RetryPolicy{MaxAttempts: 1, InitialBackoffMs: 1, MaxBackoffMs: 1, BackoffMultiplier: 1}))
	}

	regs := c.Registrations()
	if len(regs) != 100 {
		t.Fatalf("expected 100, got %d", len(regs))
	}

	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("lazy-%d", i)
		val, err := c.Resolve(key)
		if err != nil {
			t.Fatalf("Resolve(%q) failed: %v", key, err)
		}
		expected := fmt.Sprintf("value-%d", i)
		if val != expected {
			t.Errorf("Resolve(%q) = %v, want %v", key, val, expected)
		}
	}
}

// ---------------------------------------------------------------------------
// 30. Empty container operations
// ---------------------------------------------------------------------------

func TestEmptyContainerOperations(t *testing.T) {
	c := NewContainer()

	// Registrations on empty
	regs := c.Registrations()
	if len(regs) != 0 {
		t.Errorf("expected 0 registrations, got %d", len(regs))
	}

	// Resolve on empty
	_, err := c.Resolve("anything")
	if err == nil {
		t.Error("expected error on empty container resolve")
	}

	// Close on empty
	if err := c.Close(); err != nil {
		t.Fatalf("Close on empty container should succeed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// 31. Concurrent Close calls
// ---------------------------------------------------------------------------

func TestConcurrentClose(t *testing.T) {
	c := NewContainer()
	var closeCount atomic.Int32
	_ = c.RegisterSingleton("svc", &mockCloser{onClose: func() error {
		closeCount.Add(1)
		return nil
	}})

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = c.Close()
		}()
	}
	wg.Wait()
	// At least one close should have run
	if closeCount.Load() == 0 {
		t.Error("expected at least one close call")
	}
}

// ---------------------------------------------------------------------------
// 32. Registrations reflects lazy initialization state change
// ---------------------------------------------------------------------------

func TestRegistrationsReflectsStateChange(t *testing.T) {
	c := NewContainer()
	_ = c.RegisterLazy("lazy", func() string { return "val" },
		WithRetryPolicy(&RetryPolicy{MaxAttempts: 1, InitialBackoffMs: 1, MaxBackoffMs: 1, BackoffMultiplier: 1}))

	// Before resolve — not initialized
	for _, r := range c.Registrations() {
		if r.Key == "lazy" && r.Initialized {
			t.Error("lazy should not be initialized before resolve")
		}
	}

	_, _ = c.Resolve("lazy")

	// After resolve — initialized
	found := false
	for _, r := range c.Registrations() {
		if r.Key == "lazy" {
			found = true
			if !r.Initialized {
				t.Error("lazy should be initialized after resolve")
			}
		}
	}
	if !found {
		t.Error("lazy registration not found")
	}
}
