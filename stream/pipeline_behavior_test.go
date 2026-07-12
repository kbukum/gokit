package stream

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// 1. Error Recovery – Map then Collect
// ---------------------------------------------------------------------------

func TestErrorRecovery_MapThenCollect(t *testing.T) {
	p := FromSlice([]int{1, 2, 3, 4, 5})
	mapped := Map(p, func(_ context.Context, v int) (int, error) {
		if v == 3 {
			return 0, fmt.Errorf("boom on %d", v)
		}
		return v * 10, nil
	})

	got, err := Collect(context.Background(), mapped)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// Items before the failing one should have been collected.
	want := []int{10, 20}
	if !intSliceEqual(got, want) {
		t.Fatalf("partial results = %v, want %v", got, want)
	}
}

// ---------------------------------------------------------------------------
// 2. Error Recovery – Parallel errors
// ---------------------------------------------------------------------------

func TestErrorRecovery_ParallelErrors(t *testing.T) {
	items := make([]int, 20)
	for i := range items {
		items[i] = i
	}
	p := FromSlice(items)
	par := Parallel(p, 4, func(_ context.Context, v int) (int, error) {
		if v%5 == 0 && v != 0 {
			return 0, fmt.Errorf("fail on %d", v)
		}
		return v, nil
	})

	_, err := Collect(context.Background(), par)
	if err == nil {
		t.Fatal("expected at least one error from Parallel")
	}
}

// ---------------------------------------------------------------------------
// 3. Context cancellation propagates through Map→Filter→Tap with Buffer
// ---------------------------------------------------------------------------

func TestContext_CancellationPropagatesThroughChain(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Infinite source
	src := FromFunc(func(ctx context.Context) Iterator[int] {
		ch := make(chan result[int], 1)
		go func() {
			defer close(ch)
			i := 0
			for {
				select {
				case <-ctx.Done():
					return
				case ch <- result[int]{val: i, ok: true}:
					i++
				}
			}
		}()
		return &channelIter[int]{ch: ch}
	})

	var count atomic.Int64
	chain := Buffer(
		Tap(
			Filter(
				Map(src, func(_ context.Context, v int) (int, error) { return v * 2, nil }),
				func(v int) bool { return v < 1000 },
			),
			func(_ context.Context, _ int) error {
				count.Add(1)
				return nil
			},
		),
		4,
	)

	iter := chain.Iter(ctx)
	defer iter.Close()

	// Pull a few values, then cancel.
	for i := 0; i < 5; i++ {
		_, ok, err := iter.Next(ctx)
		if err != nil {
			t.Fatalf("unexpected error on item %d: %v", i, err)
		}
		if !ok {
			break
		}
	}
	cancel()

	// After cancel, Next should eventually return !ok or a context error.
	for {
		_, ok, err := iter.Next(ctx)
		if !ok || errors.Is(err, context.Canceled) {
			break
		}
	}

	if count.Load() < 5 {
		t.Fatalf("expected at least 5 taps, got %d", count.Load())
	}
}

// ---------------------------------------------------------------------------
// 4. Context cancellation during Parallel with slow workers
// ---------------------------------------------------------------------------

func TestContext_CancellationDuringParallel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	items := make([]int, 100)
	for i := range items {
		items[i] = i
	}

	p := FromSlice(items)
	slow := Parallel(p, 4, func(ctx context.Context, v int) (int, error) {
		select {
		case <-time.After(500 * time.Millisecond):
			return v, nil
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	})

	_, err := Collect(ctx, slow)
	if err == nil {
		t.Fatal("expected context deadline/cancellation error")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected error type: %v", err)
	}
}

// ---------------------------------------------------------------------------
// 5. Empty slice through Map → Filter → Reduce
// ---------------------------------------------------------------------------

func TestEmpty_MapFilterReduce(t *testing.T) {
	p := FromSlice([]int{})
	chain := Reduce(
		Filter(
			Map(p, func(_ context.Context, v int) (int, error) { return v * 2, nil }),
			func(v int) bool { return v > 0 },
		),
		42, // init
		func(acc, v int) int { return acc + v },
	)

	got, err := Collect(context.Background(), chain)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0] != 42 {
		t.Fatalf("got %v, want [42]", got)
	}
}

// ---------------------------------------------------------------------------
// 6. Concat of multiple empty pipelines
// ---------------------------------------------------------------------------

func TestEmpty_ConcatAllEmpty(t *testing.T) {
	a := FromSlice([]int{})
	b := FromSlice([]int{})
	c := FromSlice([]int{})

	got, err := Collect(context.Background(), Concat(a, b, c))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %v, want empty slice", got)
	}
}

// ---------------------------------------------------------------------------
// 7. Merge of multiple empty pipelines
// ---------------------------------------------------------------------------

func TestEmpty_MergeAllEmpty(t *testing.T) {
	a := FromSlice([]int{})
	b := FromSlice([]int{})
	c := FromSlice([]int{})

	got, err := Collect(context.Background(), Merge(a, b, c))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %v, want empty slice", got)
	}
}

// ---------------------------------------------------------------------------
// 8. Large stream – 10K items through Map → Filter → Reduce
// ---------------------------------------------------------------------------

func TestLargeStream_10K(t *testing.T) {
	const n = 10_000
	items := make([]int, n)
	for i := range items {
		items[i] = i
	}

	p := FromSlice(items)
	chain := Reduce(
		Filter(
			Map(p, func(_ context.Context, v int) (int, error) { return v * 2, nil }),
			func(v int) bool { return v%4 == 0 }, // keeps even-indexed originals
		),
		0,
		func(acc, v int) int { return acc + v },
	)

	got, err := Collect(context.Background(), chain)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Sum of v*2 for all even v in [0..9999] = 2 * sum(0,2,4,...,9998)
	// = 2 * 2 * (0+1+2+...+4999) = 4 * (4999*5000/2) = 4 * 12497500 = 49990000
	want := 49990000
	if len(got) != 1 || got[0] != want {
		t.Fatalf("got %v, want [%d]", got, want)
	}
}

// ---------------------------------------------------------------------------
// 9. Large stream – 10K items through Parallel
// ---------------------------------------------------------------------------

func TestLargeStream_ParallelProcessing(t *testing.T) {
	const n = 10_000
	items := make([]int, n)
	for i := range items {
		items[i] = i
	}

	p := FromSlice(items)
	doubled := Parallel(p, 8, func(_ context.Context, v int) (int, error) {
		return v * 2, nil
	})

	got, err := Collect(context.Background(), doubled)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != n {
		t.Fatalf("got %d items, want %d", len(got), n)
	}

	sort.Ints(got)
	for i, v := range got {
		if v != i*2 {
			t.Fatalf("got[%d] = %d, want %d", i, v, i*2)
			break
		}
	}
}

// ---------------------------------------------------------------------------
// 10. Complex chain – 6 operators
// ---------------------------------------------------------------------------

func TestComplexChain_SixOperators(t *testing.T) {
	p := FromSlice([]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})

	// Map: double
	step1 := Map(p, func(_ context.Context, v int) (int, error) { return v * 2, nil })
	// Filter: keep <= 12
	step2 := Filter(step1, func(v int) bool { return v <= 12 })
	// FlatMap: each value → [v, v+1]
	step3 := FlatMap(step2, func(_ context.Context, v int) (Iterator[int], error) {
		return &sliceIter[int]{items: []int{v, v + 1}}, nil
	})
	// Tap: count items
	var tapCount atomic.Int64
	step4 := Tap(step3, func(_ context.Context, _ int) error {
		tapCount.Add(1)
		return nil
	})
	// Reduce: sum
	step5 := Reduce(step4, 0, func(acc, v int) int { return acc + v })

	got, err := Collect(context.Background(), step5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Map: [2,4,6,8,10,12,14,16,18,20]
	// Filter(<=12): [2,4,6,8,10,12]
	// FlatMap: [2,3, 4,5, 6,7, 8,9, 10,11, 12,13]  (12 items)
	// Tap: count=12
	// Reduce(sum): 2+3+4+5+6+7+8+9+10+11+12+13 = 90
	if len(got) != 1 || got[0] != 90 {
		t.Fatalf("got %v, want [90]", got)
	}
	if tapCount.Load() != 12 {
		t.Fatalf("tap count = %d, want 12", tapCount.Load())
	}
}

// ---------------------------------------------------------------------------
// 11. Complex chain – Parallel → FanOut → TapEach → Collect
// ---------------------------------------------------------------------------

func TestComplexChain_ParallelFanOutTapReduce(t *testing.T) {
	p := FromSlice([]int{1, 2, 3, 4, 5})

	par := Parallel(p, 2, func(_ context.Context, v int) (int, error) {
		return v * 10, nil
	})

	fan := FanOut(par,
		func(_ context.Context, v int) (int, error) { return v + 1, nil },
		func(_ context.Context, v int) (int, error) { return v + 2, nil },
	)

	var tapCount atomic.Int64
	tapFn := func(_ context.Context, v int) error {
		tapCount.Add(1)
		return nil
	}
	tapped := TapEach(fan, tapFn, tapFn) //nolint:gocritic // intentional: testing with duplicate argument

	got, err := Collect(context.Background(), tapped)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(got) != 5 {
		t.Fatalf("got %d slices, want 5", len(got))
	}

	// Each slice has 2 elements: [v*10+1, v*10+2].
	// Collect sums to verify all values present.
	sums := make([]int, 0, len(got))
	for _, pair := range got {
		if len(pair) != 2 {
			t.Fatalf("expected pair of 2, got %v", pair)
		}
		sums = append(sums, pair[0]+pair[1])
	}
	sort.Ints(sums)
	// For input v: sum = (v*10+1)+(v*10+2) = 20v+3
	// v=1→23, v=2→43, v=3→63, v=4→83, v=5→103
	want := []int{23, 43, 63, 83, 103}
	if !intSliceEqual(sums, want) {
		t.Fatalf("sums = %v, want %v", sums, want)
	}

	// TapEach should have been called once per function per element = 5 * 1 fn = 5
	// Actually TapEach applies each fn to each element of the slice, so 5 slices * 2 elements * 1 fn = 10
	if tapCount.Load() != 10 {
		t.Fatalf("tap count = %d, want 10", tapCount.Load())
	}
}

// ---------------------------------------------------------------------------
// 12. Complex chain – Merge(Buffer(a), Buffer(b)) → Filter → Collect
// ---------------------------------------------------------------------------

func TestComplexChain_BufferMergeFilter(t *testing.T) {
	a := FromSlice([]int{1, 3, 5, 7, 9})
	b := FromSlice([]int{2, 4, 6, 8, 10})

	merged := Merge(Buffer(a, 2), Buffer(b, 2))
	filtered := Filter(merged, func(v int) bool { return v > 5 })

	got, err := Collect(context.Background(), filtered)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sort.Ints(got) // Merge order is non-deterministic.
	want := []int{6, 7, 8, 9, 10}
	if !intSliceEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
