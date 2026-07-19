package cache

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/logging"
)

func TestComponentLifecycle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := &componentTestStore{exists: true}
	reg := NewFactoryRegistry()
	if err := reg.Register("test", func(Config, any, *logging.Logger) (Store, error) {
		return store, nil
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	cmp := NewComponent(reg, Config{Provider: "test", Enabled: true}, nil, logging.NewDefault("test"))

	if cmp.Name() != "cache" {
		t.Fatalf("Name = %q", cmp.Name())
	}
	if cmp.Store() != nil {
		t.Fatal("Store before Start is not nil")
	}
	if health := cmp.Health(ctx); health.Status != component.StatusUnhealthy {
		t.Fatalf("Health before Start = %+v", health)
	}
	if err := cmp.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if cmp.Store() != Store(store) {
		t.Fatal("Store after Start did not return constructed store")
	}
	if health := cmp.Health(ctx); health.Status != component.StatusHealthy {
		t.Fatalf("Health after Start = %+v", health)
	}
	if desc := cmp.Describe(); desc.Name != "Cache" || desc.Type != "cache" || desc.Details != "provider=test" {
		t.Fatalf("Describe = %+v", desc)
	}
	if err := cmp.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if !store.closed {
		t.Fatal("Stop did not close store")
	}
	if cmp.Store() != nil {
		t.Fatal("Store after Stop is not nil")
	}
}

func TestComponentDisabled(t *testing.T) {
	t.Parallel()

	cmp := NewComponent(NewFactoryRegistry(), Config{Enabled: false}, nil, logging.NewDefault("test"))
	if err := cmp.Start(context.Background()); err != nil {
		t.Fatalf("Start disabled: %v", err)
	}
	if cmp.Store() != nil {
		t.Fatal("disabled component constructed a store")
	}
	if health := cmp.Health(context.Background()); health.Status != component.StatusHealthy || health.Message != "disabled" {
		t.Fatalf("Health disabled = %+v", health)
	}
}

func TestComponentReportsStartAndHealthErrors(t *testing.T) {
	t.Parallel()

	startErr := errors.New("boom")
	reg := NewFactoryRegistry()
	if err := reg.Register("test", func(Config, any, *logging.Logger) (Store, error) {
		return nil, startErr
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	cmp := NewComponent(reg, Config{Provider: "test", Enabled: true}, nil, logging.NewDefault("test"))
	if err := cmp.Start(context.Background()); !errors.Is(err, startErr) {
		t.Fatalf("Start error = %v, want %v", err, startErr)
	}

	existsErr := errors.New("exists failed")
	cmp = NewComponent(reg, Config{Provider: "test", Enabled: true}, nil, logging.NewDefault("test"))
	cmp.store = &componentTestStore{existsErr: existsErr}
	if health := cmp.Health(context.Background()); health.Status != component.StatusUnhealthy || health.Message == "" {
		t.Fatalf("Health with exists error = %+v", health)
	}
}

func TestComponentStopPropagatesCloseError(t *testing.T) {
	t.Parallel()

	closeErr := errors.New("close failed")
	cmp := NewComponent(NewFactoryRegistry(), Config{Enabled: true}, nil, logging.NewDefault("test"))
	cmp.store = &componentTestStore{closeErr: closeErr}
	if err := cmp.Stop(context.Background()); !errors.Is(err, closeErr) {
		t.Fatalf("Stop error = %v, want %v", err, closeErr)
	}
}

type componentTestStore struct {
	exists    bool
	existsErr error
	closeErr  error
	closed    bool
}

func (s *componentTestStore) Get(context.Context, string) (value []byte, found bool, err error) {
	return nil, false, nil
}
func (s *componentTestStore) Set(context.Context, string, []byte, time.Duration) error { return nil }
func (s *componentTestStore) Delete(context.Context, string) error                     { return nil }
func (s *componentTestStore) Exists(context.Context, string) (bool, error) {
	return s.exists, s.existsErr
}

func (s *componentTestStore) Close() error {
	s.closed = true
	return s.closeErr
}
