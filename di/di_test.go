package di

import (
	"strings"
	"testing"
	"time"
)

func TestNewContainer(t *testing.T) {
	c := NewContainer()
	if c == nil {
		t.Fatal("expected non-nil container")
	}
}

func TestRegisterAndResolve(t *testing.T) {
	c := NewContainer()

	err := c.Register("greeting", func() string {
		return "hello"
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	val, err := c.Resolve("greeting")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if val != "hello" {
		t.Errorf("expected 'hello', got %v", val)
	}
}

func TestResolveNotRegistered(t *testing.T) {
	c := NewContainer()
	_, err := c.Resolve("nonexistent")
	if err == nil {
		t.Error("expected error for unregistered component")
	}
	if !strings.Contains(err.Error(), "not registered") {
		t.Errorf("expected 'not registered' in error, got %q", err.Error())
	}
}

func TestRegisterSingleton(t *testing.T) {
	c := NewContainer()
	instance := "singleton-value"

	err := c.RegisterSingleton("single", instance)
	if err != nil {
		t.Fatalf("RegisterSingleton failed: %v", err)
	}

	val, err := c.Resolve("single")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if val != instance {
		t.Errorf("expected %q, got %v", instance, val)
	}
}

func TestRegisterEager(t *testing.T) {
	c := NewContainer()
	called := false
	err := c.RegisterEager("eager", func() string {
		called = true
		return "eager-value"
	})
	if err != nil {
		t.Fatalf("RegisterEager failed: %v", err)
	}
	if !called {
		t.Error("expected constructor to be called immediately for eager registration")
	}

	val, err := c.Resolve("eager")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if val != "eager-value" {
		t.Errorf("expected 'eager-value', got %v", val)
	}
}

func TestRegisterEagerWithError(t *testing.T) {
	c := NewContainer()
	err := c.RegisterEager("bad", func() (string, error) {
		return "", &testErr{msg: "init failed"}
	})
	if err == nil {
		t.Error("expected error for failed eager initialization")
	}
}

type testErr struct{ msg string }

func (e *testErr) Error() string { return e.msg }

func TestRegisterLazy(t *testing.T) {
	c := NewContainer()
	callCount := 0

	err := c.RegisterLazy("lazy", func() string {
		callCount++
		return "lazy-value"
	})
	if err != nil {
		t.Fatalf("RegisterLazy failed: %v", err)
	}
	if callCount != 0 {
		t.Error("expected constructor not to be called until resolve")
	}

	val, err := c.Resolve("lazy")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if val != "lazy-value" {
		t.Errorf("expected 'lazy-value', got %v", val)
	}
	if callCount != 1 {
		t.Errorf("expected constructor called once, got %d", callCount)
	}

	// Resolve again should use cached value
	_, err = c.Resolve("lazy")
	if err != nil {
		t.Fatalf("second Resolve failed: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected constructor still called once after cache, got %d", callCount)
	}
}

func TestMustResolveSuccess(t *testing.T) {
	c := NewContainer()
	c.RegisterSingleton("val", 42)

	val := c.MustResolve("val")
	if val != 42 {
		t.Errorf("expected 42, got %v", val)
	}
}

func TestMustResolvePanics(t *testing.T) {
	c := NewContainer()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected MustResolve to panic for unregistered component")
		}
	}()
	c.MustResolve("missing")
}

func TestSingletonPriorityOverComponent(t *testing.T) {
	c := NewContainer()
	c.RegisterSingleton("item", "from-singleton")
	c.Register("item", func() string { return "from-constructor" })

	val, err := c.Resolve("item")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if val != "from-singleton" {
		t.Errorf("expected singleton to take priority, got %v", val)
	}
}

func TestClose(t *testing.T) {
	c := NewContainer()
	closed := false
	closeable := &mockCloser{onClose: func() error {
		closed = true
		return nil
	}}

	c.RegisterSingleton("closer", closeable)
	if err := c.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if !closed {
		t.Error("expected Close to call closer on singleton")
	}
}

type mockCloser struct {
	onClose func() error
}

func (m *mockCloser) Close() error { return m.onClose() }

func TestInvalidateCache(t *testing.T) {
	c := NewContainer()
	callCount := 0
	c.Register("svc", func() string {
		callCount++
		return "value"
	})

	c.Resolve("svc")
	if callCount != 1 {
		t.Fatalf("expected 1 call, got %d", callCount)
	}

	c.InvalidateCache("svc")
	c.Resolve("svc")
	if callCount != 2 {
		t.Errorf("expected 2 calls after invalidation, got %d", callCount)
	}
}

func TestInvalidateCacheNotRegistered(t *testing.T) {
	c := NewContainer()
	err := c.InvalidateCache("missing")
	if err == nil {
		t.Error("expected error for invalidating unregistered component")
	}
}

func TestRefresh(t *testing.T) {
	c := NewContainer()
	counter := 0
	c.Register("counter", func() int {
		counter++
		return counter
	})

	c.Resolve("counter")
	val, err := c.Refresh("counter")
	if err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}
	if val != 2 {
		t.Errorf("expected 2 after refresh, got %v", val)
	}
}

func TestGetResolver(t *testing.T) {
	c := NewContainer()
	c.RegisterSingleton("val", "test")

	resolver := c.GetResolver("val")
	val, err := resolver()
	if err != nil {
		t.Fatalf("resolver failed: %v", err)
	}
	if val != "test" {
		t.Errorf("expected 'test', got %v", val)
	}
}

func TestResolveTypedGeneric(t *testing.T) {
	c := NewContainer()
	c.RegisterSingleton("num", 42)

	val, err := ResolveTyped[int](c, "num")
	if err != nil {
		t.Fatalf("ResolveTyped failed: %v", err)
	}
	if val != 42 {
		t.Errorf("expected 42, got %d", val)
	}
}

func TestGenericMustResolve(t *testing.T) {
	c := NewContainer()
	c.RegisterSingleton("str", "hello")

	val := MustResolve[string](c, "str")
	if val != "hello" {
		t.Errorf("expected 'hello', got %q", val)
	}
}

func TestGenericMustResolvePanicsOnMissing(t *testing.T) {
	c := NewContainer()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic")
		}
	}()
	MustResolve[string](c, "missing")
}

func TestGenericMustResolvePanicsOnTypeMismatch(t *testing.T) {
	c := NewContainer()
	c.RegisterSingleton("num", 42)
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on type mismatch")
		}
	}()
	MustResolve[string](c, "num")
}

func TestGenericResolveTypeMismatch(t *testing.T) {
	c := NewContainer()
	c.RegisterSingleton("num", 42)

	_, err := Resolve[string](c, "num")
	if err == nil {
		t.Error("expected error on type mismatch")
	}
}

func TestTryResolve(t *testing.T) {
	c := NewContainer()
	c.RegisterSingleton("str", "hello")

	val, ok := TryResolve[string](c, "str")
	if !ok {
		t.Error("expected TryResolve to succeed")
	}
	if val != "hello" {
		t.Errorf("expected 'hello', got %q", val)
	}

	_, ok = TryResolve[string](c, "missing")
	if ok {
		t.Error("expected TryResolve to return false for missing key")
	}

	_, ok = TryResolve[string](c, "num")
	// "num" is not registered as string, but it's not registered at all in this container
	if ok {
		t.Error("expected TryResolve to return false")
	}
}

func TestConstructorWithErrorReturn(t *testing.T) {
	c := NewContainer()
	c.Register("good", func() (string, error) {
		return "value", nil
	})

	val, err := c.Resolve("good")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if val != "value" {
		t.Errorf("expected 'value', got %v", val)
	}
}

func TestRegistrations(t *testing.T) {
	c := NewContainer()
	c.RegisterSingleton("singleton-val", "hello")
	c.Register("lazy-val", func() string { return "world" })
	c.RegisterEager("eager-val", func() string { return "eager" })

	regs := c.Registrations()
	if len(regs) != 3 {
		t.Fatalf("expected 3 registrations, got %d", len(regs))
	}

	regMap := make(map[string]RegistrationInfo)
	for _, r := range regs {
		regMap[r.Key] = r
	}

	if r, ok := regMap["singleton-val"]; !ok {
		t.Error("expected singleton-val in registrations")
	} else if r.Mode != Singleton || !r.Initialized {
		t.Errorf("singleton: mode=%d init=%v", r.Mode, r.Initialized)
	}

	if r, ok := regMap["lazy-val"]; !ok {
		t.Error("expected lazy-val in registrations")
	} else if r.Mode != Lazy || r.Initialized {
		t.Errorf("lazy: mode=%d init=%v", r.Mode, r.Initialized)
	}

	if r, ok := regMap["eager-val"]; !ok {
		t.Error("expected eager-val in registrations")
	} else if r.Mode != Eager || !r.Initialized {
		t.Errorf("eager: mode=%d init=%v", r.Mode, r.Initialized)
	}
}

func TestCircuitBreakerClosedByDefault(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold:  3,
		RecoveryTimeoutMs: 1000,
		HalfOpenRequests:  1,
	})

	if cb.IsOpen() {
		t.Error("circuit breaker should start closed")
	}
}

func TestCircuitBreakerOpensAfterThreshold(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold:  3,
		RecoveryTimeoutMs: 60000,
		HalfOpenRequests:  1,
	})

	cb.RecordFailure()
	cb.RecordFailure()
	if cb.IsOpen() {
		t.Error("should not be open after 2 failures (threshold is 3)")
	}

	cb.RecordFailure()
	if !cb.IsOpen() {
		t.Error("should be open after 3 failures")
	}
}

func TestCircuitBreakerResetsOnSuccess(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold:  2,
		RecoveryTimeoutMs: 60000,
		HalfOpenRequests:  1,
	})

	cb.RecordFailure()
	cb.RecordSuccess()

	// After success, failure count resets
	cb.RecordFailure()
	if cb.IsOpen() {
		t.Error("should not be open after reset and single failure")
	}
}

func TestCircuitBreakerRecoveryTimeout(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold:  1,
		RecoveryTimeoutMs: 10, // 10ms recovery
		HalfOpenRequests:  1,
	})

	cb.RecordFailure()
	if !cb.IsOpen() {
		t.Error("should be open after failure")
	}

	// Wait for recovery timeout
	time.Sleep(20 * time.Millisecond)
	if cb.IsOpen() {
		t.Error("should transition to half-open after recovery timeout")
	}
}

func TestWithRetryPolicyOption(t *testing.T) {
	c := NewContainer()
	policy := &RetryPolicy{
		MaxAttempts:       5,
		InitialBackoffMs:  100,
		MaxBackoffMs:      5000,
		BackoffMultiplier: 1.5,
	}

	err := c.RegisterLazy("svc", func() string { return "val" }, WithRetryPolicy(policy))
	if err != nil {
		t.Fatalf("RegisterLazy with retry policy failed: %v", err)
	}

	val, err := c.Resolve("svc")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if val != "val" {
		t.Errorf("expected 'val', got %v", val)
	}
}

func TestWithCircuitBreakerOption(t *testing.T) {
	c := NewContainer()
	cbConfig := &CircuitBreakerConfig{
		FailureThreshold:  10,
		RecoveryTimeoutMs: 30000,
		HalfOpenRequests:  5,
	}

	err := c.RegisterLazy("svc", func() string { return "val" }, WithCircuitBreaker(cbConfig))
	if err != nil {
		t.Fatalf("RegisterLazy with circuit breaker failed: %v", err)
	}

	val, err := c.Resolve("svc")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if val != "val" {
		t.Errorf("expected 'val', got %v", val)
	}
}

func TestGetTypedResolver(t *testing.T) {
	c := NewContainer()
	c.RegisterSingleton("num", 42)

	resolver := GetTypedResolver[int](c, "num")
	val, err := resolver()
	if err != nil {
		t.Fatalf("typed resolver failed: %v", err)
	}
	if val != 42 {
		t.Errorf("expected 42, got %d", val)
	}
}

func TestGetTypedResolverMissing(t *testing.T) {
	c := NewContainer()
	resolver := GetTypedResolver[string](c, "missing")
	_, err := resolver()
	if err == nil {
		t.Error("expected error for missing component")
	}
}

func TestNewSimpleContainer(t *testing.T) {
	c := NewSimpleContainer()
	if c == nil {
		t.Fatal("expected non-nil container from NewSimpleContainer")
	}

	// Verify it works like NewContainer
	c.RegisterSingleton("val", "test")
	v, err := c.Resolve("val")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if v != "test" {
		t.Errorf("expected 'test', got %v", v)
	}
}

func TestCloseWithLazyComponent(t *testing.T) {
	c := NewContainer()
	closed := false
	closeable := &mockCloser{onClose: func() error {
		closed = true
		return nil
	}}

	c.Register("lazy-closer", func() interface{} { return closeable })
	c.Resolve("lazy-closer") // Initialize it

	if err := c.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if !closed {
		t.Error("expected Close to call closer on lazy component")
	}
}

func TestInvalidateCacheSingleton(t *testing.T) {
	c := NewContainer()
	c.RegisterSingleton("item", "value")

	err := c.InvalidateCache("item")
	if err != nil {
		t.Fatalf("InvalidateCache singleton failed: %v", err)
	}

	// After invalidation, the singleton should be gone
	_, err = c.Resolve("item")
	if err == nil {
		t.Error("expected error after invalidating singleton")
	}
}

func TestConstructorNotAFunction(t *testing.T) {
	c := NewContainer()
	err := c.RegisterEager("bad", "not-a-function")
	if err == nil {
		t.Error("expected error for non-function constructor")
	}
	if !strings.Contains(err.Error(), "constructor must be a function") {
		t.Errorf("expected 'constructor must be a function' in error, got %q", err.Error())
	}
}

func TestRefreshNotRegistered(t *testing.T) {
	c := NewContainer()
	_, err := c.Refresh("missing")
	if err == nil {
		t.Error("expected error for refreshing unregistered component")
	}
}

func TestPkgNames(t *testing.T) {
	if Pkg.Config != "config" {
		t.Errorf("expected 'config', got %q", Pkg.Config)
	}
	if Pkg.Database != "database" {
		t.Errorf("expected 'database', got %q", Pkg.Database)
	}
	if Pkg.HTTPServer != "http_server" {
		t.Errorf("expected 'http_server', got %q", Pkg.HTTPServer)
	}
}

func TestRegistrationModeConstants(t *testing.T) {
	if Eager != 0 {
		t.Errorf("expected Eager=0, got %d", Eager)
	}
	if Lazy != 1 {
		t.Errorf("expected Lazy=1, got %d", Lazy)
	}
	if Singleton != 2 {
		t.Errorf("expected Singleton=2, got %d", Singleton)
	}
}
