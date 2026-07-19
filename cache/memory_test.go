package cache

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/kbukum/gokit/logging"
)

func TestRegistryRequiresExplicitMemoryRegistration(t *testing.T) {
	t.Parallel()

	reg := NewFactoryRegistry()
	if _, ok := reg.Get(ProviderMemory); ok {
		t.Fatal("memory backend registered without explicit RegisterMemory call")
	}

	if err := RegisterMemory(reg); err != nil {
		t.Fatalf("RegisterMemory: %v", err)
	}
	if _, ok := reg.Get(ProviderMemory); !ok {
		t.Fatal("memory backend not registered")
	}
}

func TestMemoryStoreTTLBoundary(t *testing.T) {
	t.Parallel()

	now := time.Unix(100, 0)
	store := newMemoryStore(MemoryConfig{}, func() time.Time { return now })
	ctx := context.Background()

	if err := store.Set(ctx, "k", []byte("v"), time.Second); err != nil {
		t.Fatalf("Set: %v", err)
	}

	now = now.Add(time.Second - time.Nanosecond)
	if _, ok, err := store.Get(ctx, "k"); err != nil || !ok {
		t.Fatalf("expected present before boundary, ok=%v err=%v", ok, err)
	}

	now = now.Add(time.Nanosecond)
	if _, ok, err := store.Get(ctx, "k"); err != nil || ok {
		t.Fatalf("expected expired at boundary, ok=%v err=%v", ok, err)
	}
}

func TestMemoryStoreCopiesValues(t *testing.T) {
	t.Parallel()

	store := NewMemoryStore(MemoryConfig{})
	ctx := context.Background()
	value := []byte("secret")
	if err := store.Set(ctx, "k", value, 0); err != nil {
		t.Fatalf("Set: %v", err)
	}
	value[0] = 'x'

	got, ok, err := store.Get(ctx, "k")
	if err != nil || !ok {
		t.Fatalf("Get ok=%v err=%v", ok, err)
	}
	if !bytes.Equal(got, []byte("secret")) {
		t.Fatalf("stored value mutated: %q", got)
	}
	got[0] = 'x'
	gotAgain, _, _ := store.Get(ctx, "k")
	if !bytes.Equal(gotAgain, []byte("secret")) {
		t.Fatalf("returned value aliased storage: %q", gotAgain)
	}
}

func TestNewUsesRegisteredProvider(t *testing.T) {
	t.Parallel()

	reg := NewFactoryRegistry()
	if err := RegisterMemory(reg); err != nil {
		t.Fatalf("RegisterMemory: %v", err)
	}
	store, err := New(reg, Config{Provider: ProviderMemory}, nil, logging.NewDefault("test"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if store == nil {
		t.Fatal("expected store")
	}
}

func TestTypedStoreLoadSaveDelete(t *testing.T) {
	t.Parallel()

	store := NewMemoryStore(MemoryConfig{})
	typed := NewTypedStore[testState](store, "prefix")
	ctx := context.Background()

	state := testState{Count: 7}
	if err := typed.Save(ctx, "k", &state, 0); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := typed.Load(ctx, "k")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got == nil || got.Count != 7 {
		t.Fatalf("Load got %+v", got)
	}
	if deleteErr := typed.Delete(ctx, "k"); deleteErr != nil {
		t.Fatalf("Delete: %v", deleteErr)
	}
	got, err = typed.Load(ctx, "k")
	if err != nil {
		t.Fatalf("Load after delete: %v", err)
	}
	if got != nil {
		t.Fatalf("expected miss, got %+v", got)
	}
}

type testState struct {
	Count int `json:"count"`
}

func TestMemoryStoreExistsAndGetMany(t *testing.T) {
	t.Parallel()

	store := NewMemoryStore(MemoryConfig{})
	ctx := context.Background()
	if exists, err := store.Exists(ctx, "missing"); err != nil || exists {
		t.Fatalf("Exists missing = %v, %v", exists, err)
	}
	if err := store.Set(ctx, "a", []byte("A"), 0); err != nil {
		t.Fatalf("Set a: %v", err)
	}
	if err := store.Set(ctx, "b", nil, 0); err != nil {
		t.Fatalf("Set b: %v", err)
	}
	if exists, err := store.Exists(ctx, "a"); err != nil || !exists {
		t.Fatalf("Exists a = %v, %v", exists, err)
	}
	got, err := store.GetMany(ctx, []string{"a", "missing", "b"})
	if err != nil {
		t.Fatalf("GetMany: %v", err)
	}
	if string(got["a"]) != "A" {
		t.Fatalf("GetMany a = %q", got["a"])
	}
	if _, ok := got["missing"]; ok {
		t.Fatal("GetMany returned missing key")
	}
	if got["b"] != nil {
		t.Fatalf("GetMany b = %#v, want nil value", got["b"])
	}
}

func TestMemoryStoreGetManySkipsExpired(t *testing.T) {
	t.Parallel()

	now := time.Unix(200, 0)
	store := newMemoryStore(MemoryConfig{}, func() time.Time { return now })
	ctx := context.Background()
	if err := store.Set(ctx, "expired", []byte("old"), time.Second); err != nil {
		t.Fatalf("Set expired: %v", err)
	}
	if err := store.Set(ctx, "fresh", []byte("new"), 0); err != nil {
		t.Fatalf("Set fresh: %v", err)
	}
	now = now.Add(time.Second)
	got, err := store.GetMany(ctx, []string{"expired", "fresh"})
	if err != nil {
		t.Fatalf("GetMany: %v", err)
	}
	if _, ok := got["expired"]; ok {
		t.Fatal("GetMany returned expired key")
	}
	if string(got["fresh"]) != "new" {
		t.Fatalf("GetMany fresh = %q", got["fresh"])
	}
}

func TestMemoryStoreConcurrentAccess(t *testing.T) {
	t.Parallel()

	store := NewMemoryStore(MemoryConfig{})
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("k-%d", i%4)
			if err := store.Set(ctx, key, []byte(key), 0); err != nil {
				t.Errorf("Set %s: %v", key, err)
				return
			}
			if _, _, err := store.Get(ctx, key); err != nil {
				t.Errorf("Get %s: %v", key, err)
			}
			if _, err := store.Exists(ctx, key); err != nil {
				t.Errorf("Exists %s: %v", key, err)
			}
		}(i)
	}
	wg.Wait()
}
