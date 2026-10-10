package cache

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/util"
)

func TestMemoryConfigValidate(t *testing.T) {
	t.Parallel()

	for name, cfg := range map[string]MemoryConfig{
		"negative ttl":         {DefaultTTL: -time.Second},
		"negative max entries": {MaxEntries: -1},
	} {
		if _, err := NewMemoryStore(cfg); err == nil {
			t.Errorf("%s: NewMemoryStore accepted %+v", name, cfg)
		}
	}
	reg := NewFactoryRegistry()
	if err := RegisterMemory(reg); err != nil {
		t.Fatal(err)
	}
	if _, err := New(reg, Config{Provider: ProviderMemory}, &MemoryConfig{MaxEntries: -1}, logging.NewDefault("test")); err == nil {
		t.Fatal("factory accepted a negative bound")
	}
}

func TestMemoryStoreEvictsLeastRecentlyUsed(t *testing.T) {
	t.Parallel()

	store := mustMemory(t, MemoryConfig{MaxEntries: 3})
	ctx := context.Background()
	for i := range 3 {
		if err := store.Set(ctx, fmt.Sprint(i), []byte{byte(i)}, 0); err != nil {
			t.Fatal(err)
		}
	}
	// Reading 0 makes 1 the least recently used.
	if _, ok, _ := store.Get(ctx, "0"); !ok {
		t.Fatal("0 missing")
	}
	// Replacing an existing key never evicts.
	if err := store.Set(ctx, "2", []byte("two"), 0); err != nil {
		t.Fatal(err)
	}
	if got := store.Len(); got != 3 {
		t.Fatalf("Len = %d, want 3", got)
	}
	if err := store.Set(ctx, "3", []byte{3}, 0); err != nil {
		t.Fatal(err)
	}
	if got := store.Len(); got != 3 {
		t.Fatalf("Len = %d, want 3 after eviction", got)
	}
	for key, want := range map[string]bool{"0": true, "1": false, "2": true, "3": true} {
		if ok, _ := store.Exists(ctx, key); ok != want {
			t.Errorf("Exists(%s) = %v, want %v", key, ok, want)
		}
	}
}

func TestMemoryStoreUsesInjectedClock(t *testing.T) {
	t.Parallel()

	clock := util.NewFakeClock(time.Unix(100, 0))
	store := mustMemory(t, MemoryConfig{DefaultTTL: time.Second, Clock: clock})
	ctx := context.Background()
	if err := store.Set(ctx, "k", []byte("v"), 0); err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	if _, ok, _ := store.Get(ctx, "k"); ok {
		t.Fatal("entry outlived the default TTL on the injected clock")
	}
	if got := store.Len(); got != 0 {
		t.Fatalf("expired entry kept: Len = %d", got)
	}
}

func TestMemoryStoreExpiredReadKeepsNewerValue(t *testing.T) {
	t.Parallel()

	clock := util.NewFakeClock(time.Unix(100, 0))
	store := mustMemory(t, MemoryConfig{Clock: clock})
	ctx := context.Background()
	if err := store.Set(ctx, "k", []byte("old"), time.Second); err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	if err := store.Set(ctx, "k", []byte("new"), 0); err != nil {
		t.Fatal(err)
	}
	if got, ok, _ := store.Get(ctx, "k"); !ok || string(got) != "new" {
		t.Fatalf("Get = %q, %v; want the newer value", got, ok)
	}
}

func mustMemory(t *testing.T, cfg MemoryConfig) *MemoryStore {
	t.Helper()
	store, err := NewMemoryStore(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

// clockFunc adapts a closure to util.Clock for tests that move time by reassignment.
type clockFunc func() time.Time

func (f clockFunc) Now() time.Time { return f() }
