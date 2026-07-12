package stateful

import (
	"context"
	"sync"
	"testing"
	"time"
)

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

	// Manually cleanup. Background cleanup may already have removed expired accumulators.
	count := mgr.Cleanup()
	if count > 2 {
		t.Errorf("expected cleanup count <= 2, got %d", count)
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
