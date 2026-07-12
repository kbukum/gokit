package stateful

import (
	"context"
	"testing"
	"time"
)

func TestEmptyStore_GetFlushSize(t *testing.T) {
	store := NewMemoryStore[int]()
	ctx := context.Background()

	// Get on empty store returns empty slice, no error
	vals, err := store.Get(ctx)
	if err != nil {
		t.Fatalf("Get on empty store errored: %v", err)
	}
	if len(vals) != 0 {
		t.Errorf("expected 0 values, got %d", len(vals))
	}

	// Flush on empty store returns empty/nil, no error
	flushed, err := store.Flush(ctx)
	if err != nil {
		t.Fatalf("Flush on empty store errored: %v", err)
	}
	if len(flushed) != 0 {
		t.Errorf("expected 0 flushed values, got %d", len(flushed))
	}

	// Size on empty store returns 0
	size, err := store.Size(ctx)
	if err != nil {
		t.Fatalf("Size on empty store errored: %v", err)
	}
	if size != 0 {
		t.Errorf("expected size 0, got %d", size)
	}
}

// ---------------------------------------------------------------------------
// MemoryStore: Close() then operations
// ---------------------------------------------------------------------------

func TestMemoryStore_OperationsAfterClose(t *testing.T) {
	store := NewMemoryStore[string]()
	ctx := context.Background()

	_ = store.Append(ctx, "before-close")
	if err := store.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// After Close, values is nil. Append creates a new slice (Go append on nil).
	// Verify operations don't panic.
	err := store.Append(ctx, "after-close")
	if err != nil {
		t.Fatalf("Append after close errored: %v", err)
	}

	// Size should reflect the post-close append
	size, err := store.Size(ctx)
	if err != nil {
		t.Fatalf("Size after close errored: %v", err)
	}
	if size != 1 {
		t.Errorf("expected size 1 after post-close append, got %d", size)
	}
}

// ---------------------------------------------------------------------------
// MemoryStore: Get returns a defensive copy
// ---------------------------------------------------------------------------

func TestMemoryStore_GetReturnsCopy(t *testing.T) {
	store := NewMemoryStore[int]()
	ctx := context.Background()

	_ = store.Append(ctx, 1)
	_ = store.Append(ctx, 2)

	vals, _ := store.Get(ctx)
	vals[0] = 999 // mutate the returned slice

	// Original store should be unaffected
	original, _ := store.Get(ctx)
	if original[0] != 1 {
		t.Errorf("Get should return a copy; store was mutated: %v", original)
	}
}

// ---------------------------------------------------------------------------
// Accumulator: trigger fires but auto-flush OnFlush returns error
// ---------------------------------------------------------------------------

func TestMemoryStore_Operations(t *testing.T) {
	store := NewMemoryStore[string]()
	ctx := context.Background()

	// Append
	if err := store.Append(ctx, "a"); err != nil {
		t.Fatalf("append failed: %v", err)
	}
	if err := store.Append(ctx, "b"); err != nil {
		t.Fatalf("append failed: %v", err)
	}

	// Size
	size, err := store.Size(ctx)
	if err != nil || size != 2 {
		t.Errorf("expected size 2, got %d, err %v", size, err)
	}

	// Get
	values, err := store.Get(ctx)
	if err != nil || len(values) != 2 {
		t.Errorf("get failed: got %d values, err %v", len(values), err)
	}

	// Flush
	flushed, err := store.Flush(ctx)
	if err != nil || len(flushed) != 2 {
		t.Errorf("flush failed: got %d values, err %v", len(flushed), err)
	}

	// Should be empty after flush
	size, _ = store.Size(ctx)
	if size != 0 {
		t.Errorf("expected size 0 after flush, got %d", size)
	}

	// Close
	if err := store.Close(); err != nil {
		t.Errorf("close failed: %v", err)
	}
}

func TestMemoryStore_Touch_LastActivity(t *testing.T) {
	store := NewMemoryStore[int]()
	ctx := context.Background()

	// Check initial activity
	activity, err := store.LastActivity(ctx)
	if err != nil {
		t.Fatalf("last activity failed: %v", err)
	}
	if activity.IsZero() {
		t.Error("expected non-zero initial activity")
	}

	initialActivity := activity
	time.Sleep(10 * time.Millisecond)

	// Touch
	if err := store.Touch(ctx); err != nil {
		t.Fatalf("touch failed: %v", err)
	}

	// Activity should be updated
	activity, _ = store.LastActivity(ctx)
	if !activity.After(initialActivity) {
		t.Error("expected activity to be updated after touch")
	}
}

func TestMemoryStore_AppendFIFO_NoLimit(t *testing.T) {
	store := NewMemoryStore[int]()
	ctx := context.Background()

	// With maxSize=0, should behave like normal Append
	evicted, err := store.AppendFIFO(ctx, 1, 0)
	if err != nil {
		t.Fatalf("append fifo failed: %v", err)
	}
	if len(evicted) != 0 {
		t.Errorf("expected no eviction, got %v", evicted)
	}

	evicted, _ = store.AppendFIFO(ctx, 2, -1)
	if len(evicted) != 0 {
		t.Errorf("expected no eviction with negative limit, got %v", evicted)
	}
}

// Test KeepAlive TTL
