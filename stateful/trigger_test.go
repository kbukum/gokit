package stateful

import (
	"context"
	"testing"
	"time"
)

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
