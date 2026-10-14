package storage

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kbukum/gokit/logging"
)

type closingStorage struct {
	stubStorage
	closed atomic.Int64
	err    error
}

func (s *closingStorage) Close(context.Context) error {
	s.closed.Add(1)
	return s.err
}

func TestComponentStopClosesOwnedBackend(t *testing.T) {
	t.Parallel()
	want := errors.New("close failed")
	backend := &closingStorage{err: want}
	c := NewComponent(stubRegistry(t, backend), Config{Provider: ProviderLocal, Enabled: true}, nil)
	if err := c.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := c.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := c.Stop(t.Context()); !errors.Is(err, want) {
		t.Fatalf("close failure lost: %v", err)
	}
	if _, err := c.Source(); err == nil {
		t.Fatal("closed backend remained available")
	}
	if err := c.Stop(t.Context()); !errors.Is(err, want) || backend.closed.Load() != 2 {
		t.Fatalf("repeated close: %v, %d", err, backend.closed.Load())
	}
}

func TestComponentStopCancelsAndDrainsInitialization(t *testing.T) {
	t.Parallel()
	entered := make(chan struct{})
	backend := &closingStorage{}
	registry := NewFactoryRegistry()
	if err := registry.Register("waiting", func(ctx context.Context, _ Config, _ *logging.Logger) (Storage, error) {
		close(entered)
		<-ctx.Done()
		return backend, nil
	}); err != nil {
		t.Fatal(err)
	}
	c := NewComponent(registry, Config{Enabled: true, Provider: "waiting"}, nil)
	started := make(chan error, 1)
	go func() { started <- c.Start(t.Context()) }()
	<-entered
	if err := c.Start(t.Context()); err == nil {
		t.Error("overlapping initialization succeeded")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	if err := c.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-started; !errors.Is(err, context.Canceled) || backend.closed.Load() != 1 {
		t.Fatalf("initialization was not drained: %v, %d", err, backend.closed.Load())
	}
}
