package stateful

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestAccumulator_BasicAppendAndFlush(t *testing.T) {
	store := NewMemoryStore[int]()
	acc := NewAccumulator(store, Config[int]{})

	ctx := context.Background()

	// Append values
	if err := acc.Append(ctx, 1); err != nil {
		t.Fatalf("append failed: %v", err)
	}
	if err := acc.Append(ctx, 2); err != nil {
		t.Fatalf("append failed: %v", err)
	}
	if err := acc.Append(ctx, 3); err != nil {
		t.Fatalf("append failed: %v", err)
	}

	// Check size
	size, err := acc.Size(ctx)
	if err != nil {
		t.Fatalf("size failed: %v", err)
	}
	if size != 3 {
		t.Errorf("expected size 3, got %d", size)
	}

	// Flush
	values, err := acc.Flush(ctx)
	if err != nil {
		t.Fatalf("flush failed: %v", err)
	}
	if len(values) != 3 {
		t.Errorf("expected 3 values, got %d", len(values))
	}

	// Should be empty after flush
	size, _ = acc.Size(ctx)
	if size != 0 {
		t.Errorf("expected size 0 after flush, got %d", size)
	}
}

func TestAccumulator_SizeTrigger(t *testing.T) {
	store := NewMemoryStore[int]()
	flushedCount := 0

	acc := NewAccumulator(store, Config[int]{
		Triggers: []Trigger[int]{
			SizeTrigger[int](3),
		},
		OnFlush: func(ctx context.Context, values []int) error {
			flushedCount++
			if len(values) != 3 {
				t.Errorf("expected 3 values in flush, got %d", len(values))
			}
			return nil
		},
	})

	ctx := context.Background()

	// Append 2 values - should not trigger
	acc.Append(ctx, 1)
	acc.Append(ctx, 2)

	if flushedCount != 0 {
		t.Errorf("expected no flush yet, got %d flushes", flushedCount)
	}

	// Append 3rd value - should trigger
	acc.Append(ctx, 3)

	if flushedCount != 1 {
		t.Errorf("expected 1 flush, got %d", flushedCount)
	}

	// Should be empty after auto-flush
	size, _ := acc.Size(ctx)
	if size != 0 {
		t.Errorf("expected size 0 after auto-flush, got %d", size)
	}
}

func TestAccumulator_TimeTrigger(t *testing.T) {
	store := NewMemoryStore[int]()
	flushedCount := 0

	acc := NewAccumulator(store, Config[int]{
		Triggers: []Trigger[int]{
			TimeTrigger[int](100 * time.Millisecond),
		},
		OnFlush: func(ctx context.Context, values []int) error {
			flushedCount++
			return nil
		},
	})

	ctx := context.Background()

	// Append a value
	acc.Append(ctx, 1)

	// Wait less than trigger time - no flush yet
	time.Sleep(50 * time.Millisecond)
	acc.Append(ctx, 2)

	if flushedCount != 0 {
		t.Errorf("expected no flush yet, got %d flushes", flushedCount)
	}

	// Wait for trigger time
	time.Sleep(100 * time.Millisecond)
	acc.Append(ctx, 3) // This append should trigger flush

	if flushedCount != 1 {
		t.Errorf("expected 1 flush, got %d", flushedCount)
	}
}

func TestAccumulator_FIFO_Eviction(t *testing.T) {
	store := NewMemoryStore[int]()
	var evicted []int

	acc := NewAccumulator(store, Config[int]{
		MaxSize: 3,
		OnEvict: func(ctx context.Context, values []int) {
			evicted = append(evicted, values...)
		},
	})

	ctx := context.Background()

	// Append 3 values - no eviction yet
	acc.Append(ctx, 1)
	acc.Append(ctx, 2)
	acc.Append(ctx, 3)

	if len(evicted) != 0 {
		t.Errorf("expected no eviction yet, got %v", evicted)
	}

	// Append 4th value - should evict oldest (1)
	acc.Append(ctx, 4)

	if len(evicted) != 1 || evicted[0] != 1 {
		t.Errorf("expected eviction of [1], got %v", evicted)
	}

	// Append 5th value - should evict 2
	acc.Append(ctx, 5)

	if len(evicted) != 2 || evicted[1] != 2 {
		t.Errorf("expected eviction of [1, 2], got %v", evicted)
	}

	// Check remaining values
	values, _ := store.Get(ctx)
	expected := []int{3, 4, 5}
	if len(values) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, values)
	}
	for i, v := range expected {
		if values[i] != v {
			t.Errorf("at index %d: expected %d, got %d", i, v, values[i])
		}
	}
}

func TestAccumulator_MinInterval_RateLimiting(t *testing.T) {
	store := NewMemoryStore[int]()
	flushedCount := 0

	acc := NewAccumulator(store, Config[int]{
		MinInterval: 200 * time.Millisecond,
		Triggers: []Trigger[int]{
			SizeTrigger[int](1), // Trigger on every append
		},
		OnFlush: func(ctx context.Context, values []int) error {
			flushedCount++
			return nil
		},
	})

	ctx := context.Background()

	// First append - should flush
	acc.Append(ctx, 1)
	if flushedCount != 1 {
		t.Errorf("expected 1 flush, got %d", flushedCount)
	}

	// Immediate append - should NOT flush (rate limited)
	acc.Append(ctx, 2)
	if flushedCount != 1 {
		t.Errorf("expected still 1 flush (rate limited), got %d", flushedCount)
	}

	// Wait for min interval
	time.Sleep(250 * time.Millisecond)

	// Now should flush
	acc.Append(ctx, 3)
	if flushedCount != 2 {
		t.Errorf("expected 2 flushes, got %d", flushedCount)
	}
}

func TestAccumulator_ByteSizeMeasurer(t *testing.T) {
	store := NewMemoryStore[[]byte]()
	flushedCount := 0

	acc := NewAccumulator(
		store,
		Config[[]byte]{
			Triggers: []Trigger[[]byte]{
				SizeTrigger[[]byte](10), // 10 bytes
			},
			OnFlush: func(ctx context.Context, values [][]byte) error {
				flushedCount++
				return nil
			},
		},
		WithMeasurer(ByteSizeMeasurer()),
	)

	ctx := context.Background()

	// Append 5 bytes
	acc.Append(ctx, []byte("12345"))
	if flushedCount != 0 {
		t.Errorf("expected no flush, got %d", flushedCount)
	}

	// Append 5 more bytes - total 10, should trigger
	acc.Append(ctx, []byte("67890"))
	if flushedCount != 1 {
		t.Errorf("expected 1 flush, got %d", flushedCount)
	}
}

func TestAccumulator_CustomMeasurer(t *testing.T) {
	type Event struct {
		Text string
	}

	store := NewMemoryStore[Event]()
	flushedCount := 0

	// Character-based measurer
	charMeasurer := CustomMeasurer(func(ctx context.Context, events []Event) int {
		total := 0
		for _, e := range events {
			total += len(e.Text)
		}
		return total
	})

	acc := NewAccumulator(
		store,
		Config[Event]{
			MinSize: 15, // 15 characters
			Triggers: []Trigger[Event]{
				SizeTrigger[Event](15),
			},
			OnFlush: func(ctx context.Context, values []Event) error {
				flushedCount++
				return nil
			},
		},
		WithMeasurer(charMeasurer),
	)

	ctx := context.Background()

	// Append events totaling 10 chars
	acc.Append(ctx, Event{Text: "hello"}) // 5
	acc.Append(ctx, Event{Text: "world"}) // 5
	if flushedCount != 0 {
		t.Errorf("expected no flush, got %d", flushedCount)
	}

	// Append 5 more chars - total 15, should trigger
	acc.Append(ctx, Event{Text: "!"})
	if flushedCount != 0 {
		t.Errorf("MinSize not met, should not flush yet")
	}

	// Append more to exceed MinSize
	acc.Append(ctx, Event{Text: "12345"})
	if flushedCount != 1 {
		t.Errorf("expected 1 flush, got %d", flushedCount)
	}
}

func TestManager_MultiTenant(t *testing.T) {
	flushedKeys := make(map[string][]int)

	mgr := NewManager(
		func(key string) *Accumulator[int] {
			return NewAccumulator(
				NewMemoryStore[int](),
				Config[int]{
					Triggers: []Trigger[int]{
						SizeTrigger[int](3),
					},
					OnFlush: func(ctx context.Context, values []int) error {
						flushedKeys[key] = values
						return nil
					},
				},
			)
		},
		5*time.Minute,
	)
	defer mgr.Close()

	ctx := context.Background()

	// Append to user1
	mgr.Append(ctx, "user1", 1)
	mgr.Append(ctx, "user1", 2)
	mgr.Append(ctx, "user1", 3) // Should trigger flush

	// Append to user2
	mgr.Append(ctx, "user2", 10)
	mgr.Append(ctx, "user2", 20)

	// Check user1 flushed
	if vals, ok := flushedKeys["user1"]; !ok || len(vals) != 3 {
		t.Errorf("expected user1 flush with 3 values, got %v", vals)
	}

	// Check user2 NOT flushed yet
	if _, ok := flushedKeys["user2"]; ok {
		t.Errorf("user2 should not have flushed yet")
	}

	// Complete user2 flush
	mgr.Append(ctx, "user2", 30)

	if vals, ok := flushedKeys["user2"]; !ok || len(vals) != 3 {
		t.Errorf("expected user2 flush with 3 values, got %v", vals)
	}
}

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
			TimeTrigger[int](1 * time.Second), // Long time
			SizeTrigger[int](3),               // 3 items
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

	// Manually flush remaining — OnFlush captures them into flushedValues
	acc.Flush(ctx)

	flushedMu.Lock()
	total := len(flushedValues)
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
