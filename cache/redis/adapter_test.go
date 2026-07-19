package redis

import (
	"context"
	"testing"

	"github.com/kbukum/gokit/cache"
	"github.com/kbukum/gokit/logging"
)

func TestRegisterCreatesRedisStore(t *testing.T) {
	t.Parallel()

	_, mini := newTestClient(t)
	reg := cache.NewFactoryRegistry()
	if err := Register(reg); err != nil {
		t.Fatalf("Register: %v", err)
	}
	store, err := cache.New(reg, cache.Config{Provider: cache.ProviderRedis}, &Config{Enabled: true, Addr: mini.Addr(), Name: "redis-test"}, logging.NewDefault("test"))
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	client, ok := store.(*Client)
	if !ok {
		t.Fatalf("store type = %T", store)
	}
	t.Cleanup(func() { _ = client.Close() })
	if client.Name() != "redis-test" {
		t.Fatalf("Name = %q", client.Name())
	}
	if !client.IsAvailable(context.Background()) {
		t.Fatal("client should be available")
	}
}

func TestRegisterRejectsInvalidConfig(t *testing.T) {
	t.Parallel()

	reg := cache.NewFactoryRegistry()
	if err := Register(reg); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if _, err := cache.New(reg, cache.Config{Provider: cache.ProviderRedis}, nil, logging.NewDefault("test")); err == nil {
		t.Fatal("cache.New accepted nil redis config")
	}
	if _, err := cache.New(reg, cache.Config{Provider: cache.ProviderRedis}, &Config{Enabled: true}, logging.NewDefault("test")); err == nil {
		t.Fatal("cache.New accepted invalid redis config")
	}
}

func TestIsAvailableFalseAfterClose(t *testing.T) {
	t.Parallel()

	client, _ := newTestClient(t)
	if err := client.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if client.IsAvailable(context.Background()) {
		t.Fatal("closed client reported available")
	}
}
