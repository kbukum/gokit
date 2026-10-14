package storage

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kbukum/gokit/logging"
)

type drainingStorage struct {
	stubStorage
	entered chan struct{}
	release chan struct{}
}

type retryingStorage struct {
	stubStorage
	calls atomic.Int64
}

func (s *retryingStorage) Close(context.Context) error {
	if s.calls.Add(1) == 1 {
		return context.DeadlineExceeded
	}
	return nil
}

func TestComponentRetainsFailedTeardownUntilRetryCompletes(t *testing.T) {
	t.Parallel()
	for _, failedStart := range []bool{false, true} {
		t.Run(map[bool]string{false: "stop", true: "initialization"}[failedStart], func(t *testing.T) {
			t.Parallel()
			backend := &retryingStorage{}
			registry := NewFactoryRegistry()
			if err := registry.Register("retry", func(context.Context, Config, *logging.Logger) (Storage, error) {
				if failedStart {
					return backend, errors.New("initialization failed")
				}
				return backend, nil
			}); err != nil {
				t.Fatal(err)
			}
			c := NewComponent(registry, Config{Provider: "retry", Enabled: true}, nil)
			startErr := c.Start(t.Context())
			if failedStart {
				if !errors.Is(startErr, context.DeadlineExceeded) {
					t.Fatalf("failed initialization lost cleanup failure: %v", startErr)
				}
			} else {
				if startErr != nil {
					t.Fatal(startErr)
				}
				if err := c.Stop(t.Context()); !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("failed teardown lost cause: %v", err)
				}
			}
			if _, err := c.Source(); err == nil {
				t.Fatal("backend with pending cleanup remained available")
			}
			if err := c.Start(t.Context()); err == nil {
				t.Fatal("restart accepted before cleanup completed")
			}
			if err := c.Stop(t.Context()); err != nil || backend.calls.Load() != 2 {
				t.Fatalf("cleanup retry: %v, calls=%d", err, backend.calls.Load())
			}
			if err := c.Stop(t.Context()); err != nil || backend.calls.Load() != 2 {
				t.Fatalf("completed cleanup repeated: %v, calls=%d", err, backend.calls.Load())
			}
		})
	}
}

func (s *drainingStorage) Close(ctx context.Context) error {
	close(s.entered)
	select {
	case <-s.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestComponentRejectsStartupWhileBackendDrains(t *testing.T) {
	t.Parallel()
	backend := &drainingStorage{entered: make(chan struct{}), release: make(chan struct{})}
	var calls atomic.Int64
	registry := NewFactoryRegistry()
	if err := registry.Register("draining", func(context.Context, Config, *logging.Logger) (Storage, error) {
		calls.Add(1)
		return backend, nil
	}); err != nil {
		t.Fatal(err)
	}
	c := NewComponent(registry, Config{Provider: "draining", Enabled: true}, nil)
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	if err := c.Start(ctx); err != nil {
		t.Fatal(err)
	}
	stopped := make(chan error, 1)
	go func() { stopped <- c.Stop(ctx) }()
	select {
	case <-backend.entered:
	case <-ctx.Done():
		t.Fatal("teardown did not begin")
	}
	err := c.Start(ctx)
	_, sourceErr := c.Source()
	close(backend.release)
	if stopErr := <-stopped; stopErr != nil {
		t.Fatal(stopErr)
	}
	if err == nil || calls.Load() != 1 || sourceErr == nil {
		t.Fatalf("startup overlapped teardown: start=%v source=%v factories=%d", err, sourceErr, calls.Load())
	}
}

func TestComponentConcurrentStopWaitsForOwnedTeardown(t *testing.T) {
	t.Parallel()
	backend := &drainingStorage{entered: make(chan struct{}), release: make(chan struct{})}
	c := NewComponent(stubRegistry(t, backend), Config{Provider: ProviderLocal, Enabled: true}, nil)
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	if err := c.Start(ctx); err != nil {
		t.Fatal(err)
	}
	stopped := make(chan error, 1)
	go func() { stopped <- c.Stop(ctx) }()
	select {
	case <-backend.entered:
	case <-ctx.Done():
		t.Fatal("teardown did not begin")
	}
	wait, cancelWait := context.WithCancel(ctx)
	cancelWait()
	err := c.Stop(wait)
	close(backend.release)
	if stopErr := <-stopped; stopErr != nil {
		t.Fatal(stopErr)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("concurrent stop reported completion before draining: %v", err)
	}
}

func TestComponentClosesBackendReturnedWithFactoryError(t *testing.T) {
	t.Parallel()
	factoryErr, closeErr := errors.New("initialization failed"), errors.New("cleanup failed")
	backend := &closingStorage{err: closeErr}
	registry := NewFactoryRegistry()
	if err := registry.Register("failed", func(context.Context, Config, *logging.Logger) (Storage, error) {
		return backend, factoryErr
	}); err != nil {
		t.Fatal(err)
	}
	c := NewComponent(registry, Config{Provider: "failed", Enabled: true}, nil)
	err := c.Start(t.Context())
	if !errors.Is(err, factoryErr) || !errors.Is(err, closeErr) || backend.closed.Load() != 1 {
		t.Fatalf("failed initialization leaked ownership or errors: %v, closes=%d", err, backend.closed.Load())
	}
	if _, err := c.Source(); err == nil {
		t.Fatal("failed backend was published")
	}
}
