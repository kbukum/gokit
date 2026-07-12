package di_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/kbukum/gokit/di"
)

type svc struct{ n int }

func TestRegister_Eager_ReturnsSameValue(t *testing.T) {
	t.Parallel()
	c := di.NewContainer()
	want := &svc{n: 42}
	if err := di.Register(c, want); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got, err := di.Resolve[*svc](context.Background(), c)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != want {
		t.Fatalf("expected same pointer, got %p want %p", got, want)
	}
}

func TestRegisterSingleton_InvokesFactoryOnce(t *testing.T) {
	t.Parallel()
	c := di.NewContainer()
	var calls int32
	if err := di.RegisterSingleton(c, func(context.Context) (*svc, error) {
		atomic.AddInt32(&calls, 1)
		return &svc{n: 1}, nil
	}); err != nil {
		t.Fatalf("RegisterSingleton: %v", err)
	}

	first, err := di.Resolve[*svc](context.Background(), c)
	if err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	second, err := di.Resolve[*svc](context.Background(), c)
	if err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	if first != second {
		t.Fatal("singleton must return the same instance")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("factory called %d times, want 1", got)
	}
}

func TestRegisterSingleton_LazyUntilResolved(t *testing.T) {
	t.Parallel()
	c := di.NewContainer()
	var calls int32
	_ = di.RegisterSingleton(c, func(context.Context) (*svc, error) {
		atomic.AddInt32(&calls, 1)
		return &svc{}, nil
	})
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Fatalf("factory ran before resolve: %d", got)
	}
	_, _ = di.Resolve[*svc](context.Background(), c)
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("factory calls = %d, want 1", got)
	}
}

func TestRegisterTransient_InvokesFactoryEachTime(t *testing.T) {
	t.Parallel()
	c := di.NewContainer()
	var calls int32
	if err := di.RegisterTransient(c, func(context.Context) (*svc, error) {
		return &svc{n: int(atomic.AddInt32(&calls, 1))}, nil
	}); err != nil {
		t.Fatalf("RegisterTransient: %v", err)
	}

	first, _ := di.Resolve[*svc](context.Background(), c)
	second, _ := di.Resolve[*svc](context.Background(), c)
	if first == second {
		t.Fatal("transient must return distinct instances")
	}
	if first.n == second.n {
		t.Fatalf("expected distinct values, got %d and %d", first.n, second.n)
	}
}

func TestResolve_Unregistered_ReturnsError(t *testing.T) {
	t.Parallel()
	c := di.NewContainer()
	_, err := di.Resolve[*svc](context.Background(), c)
	if err == nil {
		t.Fatal("expected error for unregistered type")
	}
}

func TestResolve_FactoryError_Propagates(t *testing.T) {
	t.Parallel()
	c := di.NewContainer()
	sentinel := errors.New("boom")
	_ = di.RegisterSingleton(c, func(context.Context) (*svc, error) { return nil, sentinel })

	_, err := di.Resolve[*svc](context.Background(), c)
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
}

func TestResolve_CancelledContext_ReturnsError(t *testing.T) {
	t.Parallel()
	c := di.NewContainer()
	var calls int32
	_ = di.RegisterSingleton(c, func(context.Context) (*svc, error) {
		atomic.AddInt32(&calls, 1)
		return &svc{}, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := di.Resolve[*svc](ctx, c)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Fatalf("factory ran despite canceled context: %d", got)
	}
}

func TestResolve_NamedInstancesOfSameType(t *testing.T) {
	t.Parallel()
	c := di.NewContainer()
	_ = di.Register(c, &svc{n: 1}, di.WithName("a"))
	_ = di.Register(c, &svc{n: 2}, di.WithName("b"))

	a, err := di.Resolve[*svc](context.Background(), c, di.WithName("a"))
	if err != nil {
		t.Fatalf("resolve a: %v", err)
	}
	b, err := di.Resolve[*svc](context.Background(), c, di.WithName("b"))
	if err != nil {
		t.Fatalf("resolve b: %v", err)
	}
	if a.n != 1 || b.n != 2 {
		t.Fatalf("named instances crossed: a=%d b=%d", a.n, b.n)
	}
}

func TestResolve_TypeMismatch_ReturnsError(t *testing.T) {
	t.Parallel()
	// Register two different types under the same name; resolving the wrong
	// type must fail rather than silently downcast.
	c := di.NewContainer()
	_ = di.Register(c, "hello", di.WithName("x"))
	_, err := di.Resolve[int](context.Background(), c, di.WithName("x"))
	if err == nil {
		t.Fatal("expected type-mismatch error")
	}
}

func TestNilContainer_Errors(t *testing.T) {
	t.Parallel()
	if err := di.Register(nil, 1); err == nil {
		t.Error("Register on nil container should error")
	}
	if err := di.RegisterSingleton(nil, func(context.Context) (int, error) { return 0, nil }); err == nil {
		t.Error("RegisterSingleton on nil container should error")
	}
	if err := di.RegisterTransient(nil, func(context.Context) (int, error) { return 0, nil }); err == nil {
		t.Error("RegisterTransient on nil container should error")
	}
	if _, err := di.Resolve[int](context.Background(), nil); err == nil {
		t.Error("Resolve on nil container should error")
	}
}

func TestRegister_NilConstructor_Errors(t *testing.T) {
	t.Parallel()
	c := di.NewContainer()
	if err := di.RegisterSingleton[int](c, nil); err == nil {
		t.Error("nil singleton constructor should error")
	}
	if err := di.RegisterTransient[int](c, nil); err == nil {
		t.Error("nil transient constructor should error")
	}
}

func TestRegister_NilConstructor_ErrorIncludesName(t *testing.T) {
	t.Parallel()
	c := di.NewContainer()
	err := di.RegisterSingleton[int](c, nil, di.WithName("primary"))
	if err == nil {
		t.Fatal("nil singleton constructor should error")
	}
	if !strings.Contains(err.Error(), "primary") {
		t.Errorf("error should name the WithName qualifier, got %q", err.Error())
	}
}

func TestMustResolve_Success(t *testing.T) {
	t.Parallel()
	c := di.NewContainer()
	_ = di.Register(c, 7)
	if got := di.MustResolve[int](context.Background(), c); got != 7 {
		t.Fatalf("got %d, want 7", got)
	}
}

func TestMustResolve_PanicsOnMissing(t *testing.T) {
	t.Parallel()
	c := di.NewContainer()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	_ = di.MustResolve[int](context.Background(), c)
}

func TestTryResolve(t *testing.T) {
	t.Parallel()
	c := di.NewContainer()
	_ = di.Register(c, "value")

	if v, ok := di.TryResolve[string](context.Background(), c); !ok || v != "value" {
		t.Fatalf("TryResolve = %q,%v", v, ok)
	}
	if _, ok := di.TryResolve[int](context.Background(), c); ok {
		t.Fatal("TryResolve should fail for unregistered type")
	}
}

type closer struct {
	closed *int32
	err    error
}

func (c closer) Close() error {
	atomic.AddInt32(c.closed, 1)
	return c.err
}

func TestClose_RunsDisposerOnce(t *testing.T) {
	t.Parallel()
	c := di.NewContainer()
	var closed int32
	_ = di.RegisterCloseable(c, &svc{}, func(context.Context, *svc) error {
		atomic.AddInt32(&closed, 1)
		return nil
	})

	if err := c.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := c.Close(context.Background()); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if got := atomic.LoadInt32(&closed); got != 1 {
		t.Fatalf("disposer called %d times, want 1", got)
	}
}

func TestClose_PlainRegisterNotClosed(t *testing.T) {
	t.Parallel()
	c := di.NewContainer()
	var closed int32
	// A plain Register value that also happens to implement Close() must NOT be
	// closed by the container — the caller owns it.
	_ = di.Register(c, closer{closed: &closed})

	if err := c.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := atomic.LoadInt32(&closed); got != 0 {
		t.Fatalf("plain Register value was closed %d times, want 0", got)
	}
}

func TestClose_UnresolvedSingletonNotConstructed(t *testing.T) {
	t.Parallel()
	c := di.NewContainer()
	var constructed, closed int32
	_ = di.RegisterSingletonCloseable(c, func(context.Context) (*svc, error) {
		atomic.AddInt32(&constructed, 1)
		return &svc{}, nil
	}, func(context.Context, *svc) error {
		atomic.AddInt32(&closed, 1)
		return nil
	})
	if err := c.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := atomic.LoadInt32(&constructed); got != 0 {
		t.Fatalf("Close constructed an unresolved singleton: %d", got)
	}
	if got := atomic.LoadInt32(&closed); got != 0 {
		t.Fatalf("Close ran disposer for unresolved singleton: %d", got)
	}
}

func TestClose_ResolvedSingletonClosed(t *testing.T) {
	t.Parallel()
	c := di.NewContainer()
	var closed int32
	_ = di.RegisterSingletonCloseable(c, func(context.Context) (*svc, error) {
		return &svc{}, nil
	}, func(context.Context, *svc) error {
		atomic.AddInt32(&closed, 1)
		return nil
	})
	if _, err := di.Resolve[*svc](context.Background(), c); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if err := c.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := atomic.LoadInt32(&closed); got != 1 {
		t.Fatalf("resolved singleton disposer called %d times, want 1", got)
	}
}

func TestClose_ReverseOrder(t *testing.T) {
	t.Parallel()
	c := di.NewContainer()
	var order []string
	var mu sync.Mutex
	record := func(name string) di.Disposer[*svc] {
		return func(context.Context, *svc) error {
			mu.Lock()
			order = append(order, name)
			mu.Unlock()
			return nil
		}
	}
	_ = di.RegisterCloseable(c, &svc{}, record("a"), di.WithName("a"))
	_ = di.RegisterCloseable(c, &svc{}, record("b"), di.WithName("b"))
	_ = di.RegisterCloseable(c, &svc{}, record("c"), di.WithName("c"))

	if err := c.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if want := []string{"c", "b", "a"}; !slices.Equal(order, want) {
		t.Fatalf("close order = %v, want %v (LIFO)", order, want)
	}
}

func TestClose_ReregisteredValueStillClosed(t *testing.T) {
	t.Parallel()
	c := di.NewContainer()
	var oldClosed, newClosed int32
	_ = di.RegisterCloseable(c, &svc{}, func(context.Context, *svc) error {
		atomic.AddInt32(&oldClosed, 1)
		return nil
	}, di.WithName("db"))
	// Replace the same key — the previously registered resource must still be
	// closed by the container that owns it.
	_ = di.RegisterCloseable(c, &svc{}, func(context.Context, *svc) error {
		atomic.AddInt32(&newClosed, 1)
		return nil
	}, di.WithName("db"))

	if err := c.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := atomic.LoadInt32(&oldClosed); got != 1 {
		t.Fatalf("replaced resource closed %d times, want 1", got)
	}
	if got := atomic.LoadInt32(&newClosed); got != 1 {
		t.Fatalf("current resource closed %d times, want 1", got)
	}
}

func TestClose_JoinsErrors(t *testing.T) {
	t.Parallel()
	c := di.NewContainer()
	boom := errors.New("close failed")
	var ranSecond int32
	_ = di.RegisterCloseable(c, &svc{}, func(context.Context, *svc) error {
		atomic.AddInt32(&ranSecond, 1)
		return nil
	}, di.WithName("first"))
	_ = di.RegisterCloseable(c, &svc{}, func(context.Context, *svc) error {
		return boom
	}, di.WithName("second"))

	err := c.Close(context.Background())
	if !errors.Is(err, boom) {
		t.Fatalf("expected joined close error, got %v", err)
	}
	// A failing disposer must not stop the rest from running.
	if got := atomic.LoadInt32(&ranSecond); got != 1 {
		t.Fatalf("remaining disposer ran %d times, want 1", got)
	}
}

func TestResolve_SelfCycle_Detected(t *testing.T) {
	t.Parallel()
	c := di.NewContainer()
	_ = di.RegisterSingleton(c, func(ctx context.Context) (*svc, error) {
		// Re-resolving the same type mid-construction is a cycle.
		return di.Resolve[*svc](ctx, c)
	})
	_, err := di.Resolve[*svc](context.Background(), c)
	if err == nil {
		t.Fatal("expected circular dependency error")
	}
}

type (
	a struct{}
	b struct{}
)

func TestResolve_CrossTypeCycle_Detected(t *testing.T) {
	t.Parallel()
	c := di.NewContainer()
	_ = di.RegisterSingleton(c, func(ctx context.Context) (*a, error) {
		if _, err := di.Resolve[*b](ctx, c); err != nil {
			return nil, err
		}
		return &a{}, nil
	})
	_ = di.RegisterSingleton(c, func(ctx context.Context) (*b, error) {
		if _, err := di.Resolve[*a](ctx, c); err != nil {
			return nil, err
		}
		return &b{}, nil
	})

	_, err := di.Resolve[*a](context.Background(), c)
	if err == nil {
		t.Fatal("expected cross-type circular dependency error")
	}
}

func TestResolve_ConcurrentResolvesOfSameTransient_NoFalseCycle(t *testing.T) {
	t.Parallel()
	// The resolution chain is per-call (carried in context), not per-type, so
	// many goroutines resolving the same transient concurrently must not be
	// mistaken for a cycle.
	c := di.NewContainer()
	_ = di.RegisterTransient(c, func(context.Context) (*svc, error) { return &svc{n: 1}, nil })

	const n = 64
	var wg sync.WaitGroup
	errs := make([]error, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = di.Resolve[*svc](context.Background(), c)
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: unexpected error %v", i, err)
		}
	}
}

func TestResolve_ConcurrentSingleton_InitializesOnce(t *testing.T) {
	t.Parallel()
	c := di.NewContainer()
	var calls int32
	_ = di.RegisterSingleton(c, func(context.Context) (*svc, error) {
		atomic.AddInt32(&calls, 1)
		return &svc{}, nil
	})

	const n = 50
	var wg sync.WaitGroup
	results := make([]*svc, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			v, err := di.Resolve[*svc](context.Background(), c)
			if err != nil {
				t.Errorf("resolve: %v", err)
				return
			}
			results[idx] = v
		}(i)
	}
	wg.Wait()

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("singleton factory ran %d times under concurrency, want 1", got)
	}
	for i := 1; i < n; i++ {
		if results[i] != results[0] {
			t.Fatal("concurrent resolves returned different singletons")
		}
	}
}

func TestConstructorInjection_ResolvesDependency(t *testing.T) {
	t.Parallel()
	c := di.NewContainer()
	_ = di.Register(c, &svc{n: 5})
	type consumer struct{ dep *svc }
	_ = di.RegisterSingleton(c, func(ctx context.Context) (*consumer, error) {
		dep, err := di.Resolve[*svc](ctx, c)
		if err != nil {
			return nil, err
		}
		return &consumer{dep: dep}, nil
	})

	got, err := di.Resolve[*consumer](context.Background(), c)
	if err != nil {
		t.Fatalf("resolve consumer: %v", err)
	}
	if got.dep.n != 5 {
		t.Fatalf("dependency not injected: %d", got.dep.n)
	}
}
