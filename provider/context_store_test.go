package provider

import (
	"context"
	"testing"
	"time"
)

func TestMemoryStore_SaveAndLoad(t *testing.T) {
	store := NewMemoryStore[string]()
	ctx := context.Background()

	val := "hello"
	if err := store.Save(ctx, "k1", &val, 0); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	got, err := store.Load(ctx, "k1")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if got == nil || *got != "hello" {
		t.Fatalf("expected 'hello', got %v", got)
	}
}

func TestMemoryStore_LoadMissing(t *testing.T) {
	store := NewMemoryStore[string]()
	got, err := store.Load(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for missing key, got %v", *got)
	}
}

func TestMemoryStore_Delete(t *testing.T) {
	store := NewMemoryStore[int]()
	ctx := context.Background()

	val := 42
	store.Save(ctx, "k1", &val, 0)
	if err := store.Delete(ctx, "k1"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	got, err := store.Load(ctx, "k1")
	if err != nil {
		t.Fatalf("Load after delete failed: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil after delete, got %v", *got)
	}
}

func TestMemoryStore_TTLExpiration(t *testing.T) {
	store := NewMemoryStore[string]()
	ctx := context.Background()

	val := "ephemeral"
	if err := store.Save(ctx, "k1", &val, 50*time.Millisecond); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Immediately available
	got, err := store.Load(ctx, "k1")
	if err != nil || got == nil || *got != "ephemeral" {
		t.Fatalf("expected value immediately after save, got %v, err %v", got, err)
	}

	// Wait for expiration
	time.Sleep(60 * time.Millisecond)

	got, err = store.Load(ctx, "k1")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil after TTL expiration, got %v", *got)
	}
}

func TestMemoryStore_NoTTL(t *testing.T) {
	store := NewMemoryStore[string]()
	ctx := context.Background()

	val := "persistent"
	store.Save(ctx, "k1", &val, 0)

	got, _ := store.Load(ctx, "k1")
	if got == nil || *got != "persistent" {
		t.Fatalf("expected 'persistent', got %v", got)
	}
}

func TestMemoryStore_Overwrite(t *testing.T) {
	store := NewMemoryStore[string]()
	ctx := context.Background()

	v1 := "first"
	v2 := "second"
	store.Save(ctx, "k1", &v1, 0)
	store.Save(ctx, "k1", &v2, 0)

	got, _ := store.Load(ctx, "k1")
	if got == nil || *got != "second" {
		t.Fatalf("expected 'second', got %v", got)
	}
}

func TestMemoryStore_Len(t *testing.T) {
	store := NewMemoryStore[int]()
	ctx := context.Background()

	v1, v2 := 1, 2
	store.Save(ctx, "a", &v1, 0)
	store.Save(ctx, "b", &v2, 0)

	if store.Len() != 2 {
		t.Fatalf("expected len 2, got %d", store.Len())
	}

	store.Delete(ctx, "a")
	if store.Len() != 1 {
		t.Fatalf("expected len 1 after delete, got %d", store.Len())
	}
}

func TestMemoryStore_StructValue(t *testing.T) {
	type State struct {
		Count   int
		History []string
	}
	store := NewMemoryStore[State]()
	ctx := context.Background()

	s := State{Count: 3, History: []string{"a", "b", "c"}}
	if err := store.Save(ctx, "session-1", &s, 0); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	got, err := store.Load(ctx, "session-1")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil state")
	}
	if got.Count != 3 || len(got.History) != 3 {
		t.Fatalf("expected Count=3, History len=3, got %+v", got)
	}
}
