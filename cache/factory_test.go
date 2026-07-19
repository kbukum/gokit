package cache

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kbukum/gokit/logging"
)

func TestNewRejectsInvalidInputs(t *testing.T) {
	t.Parallel()

	if _, err := New(nil, Config{Provider: ProviderMemory}, nil, logging.NewDefault("test")); err == nil {
		t.Fatal("New accepted nil registry")
	}
	if _, err := New(NewFactoryRegistry(), Config{Provider: "missing"}, nil, logging.NewDefault("test")); err == nil {
		t.Fatal("New accepted unregistered provider")
	}
	if _, err := New(NewFactoryRegistry(), Config{Provider: "missing", DefaultTTL: -1}, nil, logging.NewDefault("test")); err == nil {
		t.Fatal("New accepted invalid config")
	}
}

func TestNewPropagatesFactoryError(t *testing.T) {
	t.Parallel()

	factoryErr := errors.New("factory failed")
	reg := NewFactoryRegistry()
	if err := reg.Register("test", func(Config, any, *logging.Logger) (Store, error) {
		return nil, factoryErr
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if _, err := New(reg, Config{Provider: "test"}, nil, logging.NewDefault("test")); !errors.Is(err, factoryErr) {
		t.Fatalf("New error = %v, want %v", err, factoryErr)
	}
}

func TestRegisterMemoryProviderConfig(t *testing.T) {
	t.Parallel()

	reg := NewFactoryRegistry()
	if err := RegisterMemory(reg); err != nil {
		t.Fatalf("RegisterMemory: %v", err)
	}
	store, err := New(reg, Config{Provider: ProviderMemory, DefaultTTL: time.Hour}, &MemoryConfig{DefaultTTL: time.Second}, logging.NewDefault("test"))
	if err != nil {
		t.Fatalf("New memory: %v", err)
	}
	if err := store.Set(context.Background(), "k", []byte("v"), 0); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if _, err := New(reg, Config{Provider: ProviderMemory}, "bad", logging.NewDefault("test")); err == nil {
		t.Fatal("New memory accepted wrong provider config type")
	}
}
