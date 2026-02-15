package di

import (
	"strings"
	"testing"
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
