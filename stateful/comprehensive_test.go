package stateful

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// Test MemoryStore directly
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
func TestAccumulator_KeepAlive_TTL(t *testing.T) {
	store := NewMemoryStore[int]()
	acc := NewAccumulator(store, Config[int]{
		TTL:       100 * time.Millisecond,
		KeepAlive: true,
	})

	ctx := context.Background()

	// Append value
	acc.Append(ctx, 1)

	// Not expired yet
	if acc.IsExpired(ctx) {
		t.Error("should not be expired yet")
	}

	// Wait half TTL
	time.Sleep(60 * time.Millisecond)

	// Append again - this should reset TTL
	acc.Append(ctx, 2)

	// Wait another 60ms (total 120ms from first append, but only 60ms from second)
	time.Sleep(60 * time.Millisecond)

	// Should NOT be expired because keep-alive reset the TTL
	if acc.IsExpired(ctx) {
		t.Error("should not be expired due to keep-alive")
	}

	// Wait for TTL to expire
	time.Sleep(60 * time.Millisecond)

	// Now should be expired
	if !acc.IsExpired(ctx) {
		t.Error("should be expired now")
	}
}

// Test absolute TTL (no keep-alive)
func TestAccumulator_Absolute_TTL(t *testing.T) {
	store := NewMemoryStore[int]()
	acc := NewAccumulator(store, Config[int]{
		TTL:       100 * time.Millisecond,
		KeepAlive: false, // Absolute TTL
	})

	ctx := context.Background()

	creationTime := acc.created

	// Append value
	acc.Append(ctx, 1)

	// Wait half TTL
	time.Sleep(60 * time.Millisecond)

	// Append again - this should NOT reset TTL (no keep-alive)
	acc.Append(ctx, 2)

	// Check creation time hasn't changed
	if acc.created != creationTime {
		t.Error("creation time should not change")
	}

	// Wait another 60ms (total 120ms from creation)
	time.Sleep(60 * time.Millisecond)

	// Should be expired (absolute TTL from creation)
	if !acc.IsExpired(ctx) {
		t.Error("should be expired with absolute TTL")
	}
}

// Test TriggerMode ANY (default)
func TestAccumulator_TriggerMode_ANY(t *testing.T) {
	store := NewMemoryStore[int]()
	flushedCount := 0

	acc := NewAccumulator(store, Config[int]{
		TriggerMode: TriggerAny, // OR logic
		Triggers: []Trigger[int]{
			TimeTrigger[int](1 * time.Second),  // Long time
			SizeTrigger[int](3),                 // 3 items
		},
		OnFlush: func(ctx context.Context, values []int) error {
			flushedCount++
			return nil
		},
	})

	ctx := context.Background()

	// Trigger by size (before time trigger)
	acc.Append(ctx, 1)
	acc.Append(ctx, 2)
	acc.Append(ctx, 3) // Should trigger by size

	if flushedCount != 1 {
		t.Errorf("expected 1 flush (ANY trigger), got %d", flushedCount)
	}
}

// Test TriggerMode ALL
func TestAccumulator_TriggerMode_ALL(t *testing.T) {
	store := NewMemoryStore[int]()
	flushedCount := 0

	acc := NewAccumulator(store, Config[int]{
		TriggerMode: TriggerAll, // AND logic
		Triggers: []Trigger[int]{
			TimeTrigger[int](100 * time.Millisecond),
			SizeTrigger[int](3),
		},
		OnFlush: func(ctx context.Context, values []int) error {
			flushedCount++
			return nil
		},
	})

	ctx := context.Background()

	// Add 3 items (size trigger met, but not time)
	acc.Append(ctx, 1)
	acc.Append(ctx, 2)
	acc.Append(ctx, 3)

	if flushedCount != 0 {
		t.Error("should not flush yet (ALL triggers not met)")
	}

	// Wait for time trigger
	time.Sleep(150 * time.Millisecond)

	// Now append - both triggers met
	acc.Append(ctx, 4)

	if flushedCount != 1 {
		t.Errorf("expected 1 flush (ALL triggers met), got %d", flushedCount)
	}
}

// Test MinSize requirement
func TestAccumulator_MinSize_Requirement(t *testing.T) {
	store := NewMemoryStore[int]()
	flushedCount := 0

	acc := NewAccumulator(store, Config[int]{
		MinSize: 5, // Need at least 5 items
		Triggers: []Trigger[int]{
			SizeTrigger[int](3), // Would trigger at 3
		},
		OnFlush: func(ctx context.Context, values []int) error {
			flushedCount++
			return nil
		},
	})

	ctx := context.Background()

	// Add 3 items - trigger fires but MinSize not met
	acc.Append(ctx, 1)
	acc.Append(ctx, 2)
	acc.Append(ctx, 3)

	if flushedCount != 0 {
		t.Error("should not flush (MinSize not met)")
	}

	// Add 2 more - now MinSize met
	acc.Append(ctx, 4)
	acc.Append(ctx, 5)

	if flushedCount != 1 {
		t.Errorf("expected 1 flush (MinSize met), got %d", flushedCount)
	}
}

// Test OnEvict callback
func TestAccumulator_OnEvict_Callback(t *testing.T) {
	store := NewMemoryStore[int]()
	var evictedValues []int
	evictCallCount := 0

	acc := NewAccumulator(store, Config[int]{
		MaxSize: 3,
		OnEvict: func(ctx context.Context, evicted []int) {
			evictCallCount++
			evictedValues = append(evictedValues, evicted...)
		},
	})

	ctx := context.Background()

	// Fill to max
	acc.Append(ctx, 1)
	acc.Append(ctx, 2)
	acc.Append(ctx, 3)

	// Evict oldest
	acc.Append(ctx, 4)
	acc.Append(ctx, 5)

	if evictCallCount != 2 {
		t.Errorf("expected 2 evict calls, got %d", evictCallCount)
	}

	expected := []int{1, 2}
	if len(evictedValues) != len(expected) {
		t.Fatalf("expected %v evicted, got %v", expected, evictedValues)
	}
	for i, v := range expected {
		if evictedValues[i] != v {
			t.Errorf("at index %d: expected %d, got %d", i, v, evictedValues[i])
		}
	}
}

// Test Manager cleanup
func TestManager_Cleanup_Expiration(t *testing.T) {
	expiredKeys := make(map[string]bool)
	var mu sync.Mutex

	mgr := NewManager(
		func(key string) *Accumulator[int] {
			return NewAccumulator(
				NewMemoryStore[int](),
				Config[int]{
					TTL:       50 * time.Millisecond,
					KeepAlive: false,
					OnExpire: func(ctx context.Context, k string) {
						mu.Lock()
						expiredKeys[k] = true
						mu.Unlock()
					},
				},
			)
		},
		50*time.Millisecond,
	)
	defer mgr.Close()

	ctx := context.Background()

	// Create accumulators
	mgr.Append(ctx, "user1", 1)
	mgr.Append(ctx, "user2", 2)

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	// Manually cleanup
	count := mgr.Cleanup()

	if count != 2 {
		t.Errorf("expected 2 cleanups, got %d", count)
	}

	mu.Lock()
	if !expiredKeys["user1"] || !expiredKeys["user2"] {
		t.Errorf("expected both users expired, got %v", expiredKeys)
	}
	mu.Unlock()
}

// Test Manager operations
func TestManager_Operations(t *testing.T) {
	mgr := NewManager(
		func(key string) *Accumulator[int] {
			return NewAccumulator(
				NewMemoryStore[int](),
				Config[int]{},
			)
		},
		5*time.Minute,
	)
	defer mgr.Close()

	ctx := context.Background()

	// Get non-existent
	if acc := mgr.Get("nonexistent"); acc != nil {
		t.Error("expected nil for non-existent key")
	}

	// GetOrCreate
	acc1 := mgr.GetOrCreate("user1")
	if acc1 == nil {
		t.Fatal("expected accumulator")
	}

	// Get existing
	acc2 := mgr.Get("user1")
	if acc1 != acc2 {
		t.Error("expected same accumulator instance")
	}

	// Append
	if err := mgr.Append(ctx, "user1", 10); err != nil {
		t.Errorf("append failed: %v", err)
	}

	// Size
	size, err := mgr.Size(ctx, "user1")
	if err != nil || size != 1 {
		t.Errorf("expected size 1, got %d, err %v", size, err)
	}

	// Measure
	measured, err := mgr.Measure(ctx, "user1")
	if err != nil || measured != 1 {
		t.Errorf("expected measure 1, got %d, err %v", measured, err)
	}

	// List
	keys := mgr.List()
	if len(keys) != 1 || keys[0] != "user1" {
		t.Errorf("expected [user1], got %v", keys)
	}

	// Flush
	values, err := mgr.Flush(ctx, "user1")
	if err != nil || len(values) != 1 {
		t.Errorf("flush failed: %v, err %v", values, err)
	}

	// Delete
	if !mgr.Delete("user1") {
		t.Error("delete failed")
	}

	// Delete non-existent
	if mgr.Delete("user1") {
		t.Error("should return false for non-existent")
	}
}

// Test concurrent appends
func TestAccumulator_Concurrent_Appends(t *testing.T) {
	store := NewMemoryStore[int]()
	var flushedMu sync.Mutex
	var flushedValues []int

	acc := NewAccumulator(store, Config[int]{
		Triggers: []Trigger[int]{
			SizeTrigger[int](50), // Flush at 50
		},
		OnFlush: func(ctx context.Context, values []int) error {
			flushedMu.Lock()
			flushedValues = append(flushedValues, values...)
			flushedMu.Unlock()
			return nil
		},
	})

	ctx := context.Background()
	const goroutines = 10
	const appendsPerGoroutine = 20
	const expected = goroutines * appendsPerGoroutine

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < appendsPerGoroutine; j++ {
				acc.Append(ctx, id*100+j)
			}
		}(i)
	}

	wg.Wait()

	// Manually flush remaining
	remaining, _ := acc.Flush(ctx)
	
	flushedMu.Lock()
	total := len(flushedValues) + len(remaining)
	flushedMu.Unlock()

	if total != expected {
		t.Errorf("expected %d total items, got %d", expected, total)
	}
}

// Test error handling in OnFlush
func TestAccumulator_OnFlush_Error(t *testing.T) {
	store := NewMemoryStore[int]()
	flushErr := errors.New("flush error")

	acc := NewAccumulator(store, Config[int]{
		OnFlush: func(ctx context.Context, values []int) error {
			return flushErr
		},
	})

	ctx := context.Background()

	// Append a value first
	acc.Append(ctx, 42)

	// Manual flush should return error
	values, err := acc.Flush(ctx)
	if err == nil {
		t.Fatal("expected error from flush")
	}
	if !errors.Is(err, flushErr) {
		t.Errorf("expected flush error in chain, got %v", err)
	}
	// Values should still be returned even with error
	if len(values) != 1 {
		t.Errorf("expected 1 value returned despite error, got %d", len(values))
	}
}

// Test CustomTrigger
func TestCustomTrigger(t *testing.T) {
	store := NewMemoryStore[int]()
	flushedCount := 0

	// Custom trigger: flush when sum > 10
	sumTrigger := CustomTrigger("sum>10", func(ctx context.Context, acc *Accumulator[int]) bool {
		values, _ := acc.store.Get(ctx)
		sum := 0
		for _, v := range values {
			sum += v
		}
		return sum > 10
	})

	acc := NewAccumulator(store, Config[int]{
		Triggers: []Trigger[int]{sumTrigger},
		OnFlush: func(ctx context.Context, values []int) error {
			flushedCount++
			return nil
		},
	})

	ctx := context.Background()

	// Add values
	acc.Append(ctx, 3) // sum=3
	acc.Append(ctx, 4) // sum=7
	acc.Append(ctx, 5) // sum=12 -> triggers!

	if flushedCount != 1 {
		t.Errorf("expected 1 flush, got %d", flushedCount)
	}

	// Test Name
	if name := sumTrigger.Name(); name != "custom:sum>10" {
		t.Errorf("expected name 'custom:sum>10', got %q", name)
	}
}

// Test trigger Name methods
func TestTrigger_Names(t *testing.T) {
	timeTrig := TimeTrigger[int](5 * time.Second)
	if name := timeTrig.Name(); name != "time:5s" {
		t.Errorf("expected 'time:5s', got %q", name)
	}

	sizeTrig := SizeTrigger[int](100)
	name := sizeTrig.Name()
	if name == "" {
		t.Error("size trigger name should not be empty")
	}
}

// Test Touch method
func TestAccumulator_Touch(t *testing.T) {
	store := NewMemoryStore[int]()
	acc := NewAccumulator(store, Config[int]{
		TTL:       100 * time.Millisecond,
		KeepAlive: true,
	})

	ctx := context.Background()

	// Touch
	if err := acc.Touch(ctx); err != nil {
		t.Errorf("touch failed: %v", err)
	}

	// Check activity updated
	activity, _ := store.LastActivity(ctx)
	if activity.IsZero() {
		t.Error("expected non-zero activity after touch")
	}
}

// Test zero TTL (never expires)
func TestAccumulator_ZeroTTL_NeverExpires(t *testing.T) {
	store := NewMemoryStore[int]()
	acc := NewAccumulator(store, Config[int]{
		TTL: 0, // Never expires
	})

	ctx := context.Background()

	acc.Append(ctx, 1)

	time.Sleep(100 * time.Millisecond)

	if acc.IsExpired(ctx) {
		t.Error("should never expire with TTL=0")
	}
}
