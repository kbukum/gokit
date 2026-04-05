package stateful

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Empty MemoryStore operations
// ---------------------------------------------------------------------------

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

func TestAccumulator_AutoFlush_OnFlushError(t *testing.T) {
	store := NewMemoryStore[int]()
	flushErr := errors.New("processing failed")
	flushedCount := 0

	acc := NewAccumulator(store, Config[int]{
		Triggers: []Trigger[int]{SizeTrigger[int](2)},
		OnFlush: func(_ context.Context, _ []int) error {
			flushedCount++
			return flushErr
		},
	})

	ctx := context.Background()

	_ = acc.Append(ctx, 1)
	err := acc.Append(ctx, 2) // triggers auto-flush which fails

	if err == nil {
		t.Fatal("expected Append to propagate OnFlush error")
	}
	if !errors.Is(err, flushErr) {
		t.Errorf("expected wrapped flushErr, got %v", err)
	}
	if flushedCount != 1 {
		t.Errorf("expected 1 flush attempt, got %d", flushedCount)
	}
}

// ---------------------------------------------------------------------------
// Accumulator: double flush — second returns empty
// ---------------------------------------------------------------------------

func TestAccumulator_DoubleFlush(t *testing.T) {
	store := NewMemoryStore[string]()
	acc := NewAccumulator(store, Config[string]{})
	ctx := context.Background()

	_ = acc.Append(ctx, "a")
	_ = acc.Append(ctx, "b")

	first, err := acc.Flush(ctx)
	if err != nil {
		t.Fatalf("first flush: %v", err)
	}
	if len(first) != 2 {
		t.Errorf("expected 2 values, got %d", len(first))
	}

	second, err := acc.Flush(ctx)
	if err != nil {
		t.Fatalf("second flush: %v", err)
	}
	if len(second) != 0 {
		t.Errorf("expected 0 values on second flush, got %d", len(second))
	}
}

// ---------------------------------------------------------------------------
// Accumulator: append after flush still works
// ---------------------------------------------------------------------------

func TestAccumulator_AppendAfterFlush(t *testing.T) {
	store := NewMemoryStore[int]()
	acc := NewAccumulator(store, Config[int]{})
	ctx := context.Background()

	_ = acc.Append(ctx, 1)
	_, _ = acc.Flush(ctx)

	_ = acc.Append(ctx, 2)
	_ = acc.Append(ctx, 3)

	size, _ := acc.Size(ctx)
	if size != 2 {
		t.Errorf("expected size 2, got %d", size)
	}

	vals, _ := acc.Flush(ctx)
	if len(vals) != 2 || vals[0] != 2 || vals[1] != 3 {
		t.Errorf("expected [2, 3], got %v", vals)
	}
}

// ---------------------------------------------------------------------------
// Manager: concurrent GetOrCreate for same key (race condition)
// ---------------------------------------------------------------------------

func TestManager_ConcurrentGetOrCreate(t *testing.T) {
	mgr := NewManager(
		func(key string) *Accumulator[int] {
			return NewAccumulator(NewMemoryStore[int](), Config[int]{})
		},
		5*time.Minute,
	)
	defer mgr.Close()

	const goroutines = 50
	results := make([]*Accumulator[int], goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			results[idx] = mgr.GetOrCreate("shared-key")
		}(i)
	}

	wg.Wait()

	// All goroutines must receive the same accumulator instance
	for i := 1; i < goroutines; i++ {
		if results[i] != results[0] {
			t.Fatalf("goroutine %d got different accumulator instance", i)
		}
	}
}

// ---------------------------------------------------------------------------
// Manager: Keys() accuracy during concurrent modifications
// ---------------------------------------------------------------------------

func TestManager_KeysDuringConcurrentMods(t *testing.T) {
	mgr := NewManager(
		func(key string) *Accumulator[int] {
			return NewAccumulator(NewMemoryStore[int](), Config[int]{})
		},
		5*time.Minute,
	)
	defer mgr.Close()

	ctx := context.Background()
	const n = 20
	var wg sync.WaitGroup

	// Concurrently create accumulators
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			_ = mgr.Append(ctx, "key-"+string(rune('A'+i)), i)
		}(i)
	}
	wg.Wait()

	keys := mgr.List()
	if len(keys) != n {
		t.Errorf("expected %d keys, got %d", n, len(keys))
	}
}

// ---------------------------------------------------------------------------
// Manager: Flush/Size/Measure on non-existent key
// ---------------------------------------------------------------------------

func TestManager_NonExistentKeyOps(t *testing.T) {
	mgr := NewManager(
		func(key string) *Accumulator[int] {
			return NewAccumulator(NewMemoryStore[int](), Config[int]{})
		},
		5*time.Minute,
	)
	defer mgr.Close()

	ctx := context.Background()

	// Flush non-existent → nil, nil
	vals, err := mgr.Flush(ctx, "ghost")
	if err != nil || vals != nil {
		t.Errorf("Flush non-existent: expected (nil, nil), got (%v, %v)", vals, err)
	}

	// Size non-existent → 0, nil
	size, err := mgr.Size(ctx, "ghost")
	if err != nil || size != 0 {
		t.Errorf("Size non-existent: expected (0, nil), got (%d, %v)", size, err)
	}

	// Measure non-existent → 0, nil
	m, err := mgr.Measure(ctx, "ghost")
	if err != nil || m != 0 {
		t.Errorf("Measure non-existent: expected (0, nil), got (%d, %v)", m, err)
	}
}

// ---------------------------------------------------------------------------
// Manager: Close is idempotent
// ---------------------------------------------------------------------------

func TestManager_CloseIdempotent(t *testing.T) {
	mgr := NewManager(
		func(key string) *Accumulator[int] {
			return NewAccumulator(NewMemoryStore[int](), Config[int]{})
		},
		5*time.Minute,
	)

	ctx := context.Background()
	_ = mgr.Append(ctx, "k", 1)

	// Close twice should not panic
	if err := mgr.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := mgr.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Concurrent Append + Flush
// ---------------------------------------------------------------------------

func TestAccumulator_ConcurrentAppendAndFlush(t *testing.T) {
	store := NewMemoryStore[int]()
	var mu sync.Mutex
	var allFlushed []int

	acc := NewAccumulator(store, Config[int]{
		OnFlush: func(_ context.Context, values []int) error {
			mu.Lock()
			allFlushed = append(allFlushed, values...)
			mu.Unlock()
			return nil
		},
	})

	ctx := context.Background()
	const appends = 100
	var wg sync.WaitGroup

	// Goroutine: append values
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < appends; i++ {
			_ = acc.Append(ctx, i)
		}
	}()

	// Goroutine: periodic flushes
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			acc.Flush(ctx)
			time.Sleep(time.Millisecond)
		}
	}()

	wg.Wait()

	// Final flush to capture remaining
	acc.Flush(ctx)

	mu.Lock()
	total := len(allFlushed)
	mu.Unlock()

	remaining, _ := acc.Size(ctx)
	if total+remaining != appends {
		t.Errorf("expected %d total (flushed+remaining), got flushed=%d remaining=%d",
			appends, total, remaining)
	}
}

// ---------------------------------------------------------------------------
// ByteSizeMeasurer: accuracy for varied-length byte slices
// ---------------------------------------------------------------------------

func TestByteSizeMeasurer_Accuracy(t *testing.T) {
	m := ByteSizeMeasurer()
	ctx := context.Background()

	values := [][]byte{
		[]byte(""),          // 0 bytes
		[]byte("hello"),     // 5 bytes
		[]byte("world!!!!"), // 9 bytes
	}

	result := m.Measure(ctx, values)
	if result != 14 {
		t.Errorf("expected 14 bytes, got %d", result)
	}

	// Empty slice → 0
	if m.Measure(ctx, nil) != 0 {
		t.Error("expected 0 for nil values")
	}
}

// ---------------------------------------------------------------------------
// Manager: TTL expiration during active use (keep-alive keeps it alive)
// ---------------------------------------------------------------------------

func TestManager_TTL_KeepAlive_DuringActiveUse(t *testing.T) {
	mgr := NewManager(
		func(key string) *Accumulator[int] {
			return NewAccumulator(NewMemoryStore[int](), Config[int]{
				TTL:       80 * time.Millisecond,
				KeepAlive: true,
			})
		},
		80*time.Millisecond,
	)
	defer mgr.Close()

	ctx := context.Background()
	_ = mgr.Append(ctx, "active", 1)

	// Keep using it — should stay alive
	for i := 0; i < 5; i++ {
		time.Sleep(30 * time.Millisecond)
		_ = mgr.Append(ctx, "active", i+2)
	}

	// Should NOT be cleaned up because keep-alive resets TTL
	cleaned := mgr.Cleanup()
	if cleaned != 0 {
		t.Errorf("expected 0 cleanups (keep-alive active), got %d", cleaned)
	}

	acc := mgr.Get("active")
	if acc == nil {
		t.Fatal("accumulator should still exist")
	}

	size, _ := acc.Size(ctx)
	if size != 6 {
		t.Errorf("expected size 6, got %d", size)
	}
}

// ---------------------------------------------------------------------------
// Custom trigger that uses accumulator state
// ---------------------------------------------------------------------------

func TestCustomTrigger_WithAccumulatorState(t *testing.T) {
	store := NewMemoryStore[string]()
	flushedCount := 0

	// Trigger: flush when any item is longer than 10 chars
	longItemTrigger := CustomTrigger("long-item", func(ctx context.Context, acc *Accumulator[string]) bool {
		values, _ := acc.store.Get(ctx)
		for _, v := range values {
			if len(v) > 10 {
				return true
			}
		}
		return false
	})

	acc := NewAccumulator(store, Config[string]{
		Triggers: []Trigger[string]{longItemTrigger},
		OnFlush: func(_ context.Context, _ []string) error {
			flushedCount++
			return nil
		},
	})

	ctx := context.Background()

	_ = acc.Append(ctx, "short")
	_ = acc.Append(ctx, "also short")
	if flushedCount != 0 {
		t.Error("should not flush for short items")
	}

	_ = acc.Append(ctx, "this is a long item!") // > 10 chars
	if flushedCount != 1 {
		t.Errorf("expected 1 flush for long item, got %d", flushedCount)
	}
}

// ---------------------------------------------------------------------------
// Multiple FIFO evictions in a single large append burst
// ---------------------------------------------------------------------------

func TestAccumulator_BulkFIFO_Eviction(t *testing.T) {
	store := NewMemoryStore[int]()
	var allEvicted []int

	acc := NewAccumulator(store, Config[int]{
		MaxSize: 5,
		OnEvict: func(_ context.Context, evicted []int) {
			allEvicted = append(allEvicted, evicted...)
		},
	})

	ctx := context.Background()

	// Append 10 items with max 5 → items 0-4 should be evicted one by one
	for i := 0; i < 10; i++ {
		_ = acc.Append(ctx, i)
	}

	// Store should contain [5,6,7,8,9]
	vals, _ := store.Get(ctx)
	if len(vals) != 5 {
		t.Fatalf("expected 5 remaining, got %d", len(vals))
	}
	for i, v := range vals {
		expected := i + 5
		if v != expected {
			t.Errorf("vals[%d] = %d, want %d", i, v, expected)
		}
	}

	// Evicted should be [0,1,2,3,4]
	if len(allEvicted) != 5 {
		t.Fatalf("expected 5 evicted, got %d: %v", len(allEvicted), allEvicted)
	}
	for i, v := range allEvicted {
		if v != i {
			t.Errorf("evicted[%d] = %d, want %d", i, v, i)
		}
	}
}
