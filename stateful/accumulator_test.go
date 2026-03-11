package stateful

import (
	"context"
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
