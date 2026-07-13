package storage

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/logging"
)

// stubStorage is a minimal in-memory Storage for component/factory tests.
type stubStorage struct {
	urlErr error
}

func (s *stubStorage) Upload(context.Context, string, io.Reader) error { return nil }
func (s *stubStorage) Download(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (s *stubStorage) Delete(context.Context, string) error         { return nil }
func (s *stubStorage) Exists(context.Context, string) (bool, error) { return false, nil }
func (s *stubStorage) URL(context.Context, string) (string, error) {
	return "https://stub/x", s.urlErr
}
func (s *stubStorage) List(context.Context, string) ([]FileInfo, error) { return nil, nil }

func stubRegistry(t *testing.T, s Storage) *FactoryRegistry {
	t.Helper()
	reg := NewFactoryRegistry()
	if err := reg.Register(ProviderLocal, func(Config, *logging.Logger) (Storage, error) {
		return s, nil
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	return reg
}

func TestNewLooksUpRegisteredProvider(t *testing.T) {
	t.Parallel()
	reg := stubRegistry(t, &stubStorage{})
	got, err := New(reg, Config{Provider: ProviderLocal, Enabled: true}, nil)
	if err != nil || got == nil {
		t.Fatalf("New = %v, %v", got, err)
	}
}

func TestNewNilRegistry(t *testing.T) {
	t.Parallel()
	if _, err := New(nil, Config{Provider: ProviderLocal}, nil); err == nil {
		t.Fatal("expected nil-registry error")
	}
}

func TestNewUnsupportedProvider(t *testing.T) {
	t.Parallel()
	reg := NewFactoryRegistry()
	if _, err := New(reg, Config{Provider: "nope"}, nil); err == nil {
		t.Fatal("expected unsupported-provider error")
	}
}

func TestComponentLifecycle(t *testing.T) {
	t.Parallel()
	reg := stubRegistry(t, &stubStorage{})
	c := NewComponent(reg, Config{Provider: ProviderLocal, Enabled: true}, nil)
	if c.Name() != "storage" {
		t.Fatalf("Name = %q", c.Name())
	}
	if c.Storage() != nil {
		t.Fatal("storage should be nil before Start")
	}
	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if c.Storage() == nil {
		t.Fatal("storage nil after Start")
	}
	if !c.IsAvailable(ctx) {
		t.Fatal("IsAvailable should be true")
	}
	if h := c.Health(ctx); h.Status != component.StatusHealthy {
		t.Fatalf("Health = %+v", h)
	}
	if err := c.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if c.IsAvailable(ctx) {
		t.Fatal("IsAvailable should be false after Stop")
	}
}

func TestComponentDisabled(t *testing.T) {
	t.Parallel()
	reg := stubRegistry(t, &stubStorage{})
	c := NewComponent(reg, Config{Provider: ProviderLocal, Enabled: false}, nil)
	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if c.Storage() != nil {
		t.Fatal("disabled component should not init storage")
	}
	if h := c.Health(ctx); h.Status != component.StatusHealthy || h.Message != "disabled" {
		t.Fatalf("Health = %+v", h)
	}
}

func TestComponentHealthUninitialized(t *testing.T) {
	t.Parallel()
	c := NewComponent(stubRegistry(t, &stubStorage{}), Config{Provider: ProviderLocal, Enabled: true}, nil)
	if h := c.Health(context.Background()); h.Status != component.StatusUnhealthy {
		t.Fatalf("Health = %+v", h)
	}
}

func TestComponentHealthProbeFailure(t *testing.T) {
	t.Parallel()
	reg := stubRegistry(t, &stubStorage{urlErr: errors.New("probe boom")})
	c := NewComponent(reg, Config{Provider: ProviderLocal, Enabled: true}, nil)
	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if h := c.Health(ctx); h.Status != component.StatusUnhealthy {
		t.Fatalf("Health = %+v", h)
	}
}

func TestComponentStartError(t *testing.T) {
	t.Parallel()
	reg := NewFactoryRegistry() // no provider registered
	c := NewComponent(reg, Config{Provider: "missing", Enabled: true}, nil)
	if err := c.Start(context.Background()); err == nil {
		t.Fatal("expected start error")
	}
}

func TestComponentDescribe(t *testing.T) {
	t.Parallel()
	c := NewComponent(stubRegistry(t, &stubStorage{}), Config{Provider: ProviderLocal}, nil)
	d := c.Describe()
	if d.Type != "storage" || !strings.Contains(d.Details, ProviderLocal) {
		t.Fatalf("Describe = %+v", d)
	}
}

func TestNewRejectsInvalidConfig(t *testing.T) {
	t.Parallel()
	if _, err := New(NewFactoryRegistry(), Config{Provider: ""}, nil); err == nil {
		t.Fatal("expected validation error for empty provider")
	}
}
