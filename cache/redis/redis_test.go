package redis

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/kbukum/gokit/cache"
	"github.com/kbukum/gokit/logger"
)

func newTestClient(t *testing.T) (*Client, *miniredis.Miniredis) {
	t.Helper()
	mini, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mini.Close)

	client, err := New(Config{Enabled: true, Addr: mini.Addr()}, logger.NewDefault("test"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client, mini
}

func TestRegisterRequiresExplicitCall(t *testing.T) {
	t.Parallel()

	reg := cache.NewFactoryRegistry()
	if _, ok := reg.Get(cache.ProviderRedis); ok {
		t.Fatal("redis registered without explicit Register call")
	}
	if err := Register(reg); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if _, ok := reg.Get(cache.ProviderRedis); !ok {
		t.Fatal("redis not registered")
	}
}

func TestClientStoreOperations(t *testing.T) {
	t.Parallel()

	client, _ := newTestClient(t)
	ctx := context.Background()

	if err := client.Set(ctx, "k", []byte("v"), 0); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, ok, err := client.Get(ctx, "k")
	if err != nil || !ok {
		t.Fatalf("Get ok=%v err=%v", ok, err)
	}
	if !bytes.Equal(got, []byte("v")) {
		t.Fatalf("Get = %q", got)
	}
	exists, err := client.Exists(ctx, "k")
	if err != nil || !exists {
		t.Fatalf("Exists=%v err=%v", exists, err)
	}
	if err := client.Delete(ctx, "k"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok, err := client.Get(ctx, "k"); err != nil || ok {
		t.Fatalf("expected miss after delete, ok=%v err=%v", ok, err)
	}
}

func TestClientTTL(t *testing.T) {
	t.Parallel()

	client, mini := newTestClient(t)
	ctx := context.Background()
	if err := client.Set(ctx, "k", []byte("v"), time.Second); err != nil {
		t.Fatalf("Set: %v", err)
	}
	mini.FastForward(time.Second)
	if _, ok, err := client.Get(ctx, "k"); err != nil || ok {
		t.Fatalf("expected expired key, ok=%v err=%v", ok, err)
	}
}
