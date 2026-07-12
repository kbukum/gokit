package cache

import (
	"bytes"
	"context"
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
