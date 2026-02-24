package pipeline

import (
	"context"
	"testing"
	"time"
)

// --- Throttle tests ---

func TestThrottle_DropsRapidValues(t *testing.T) {
	p := FromSlice([]int{1, 2, 3, 4, 5})
	// With a very large interval, only the first value should pass
	throttled := Throttle(p, time.Hour)
	got, err := Collect(context.Background(), throttled)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != 1 {
		t.Errorf("expected [1], got %v", got)
	}
}

func TestThrottle_AllPassWithZeroInterval(t *testing.T) {
	p := FromSlice([]int{1, 2, 3})
	throttled := Throttle(p, 0)
	got, err := Collect(context.Background(), throttled)
	if err != nil {
		t.Fatal(err)
	}
	if !intSliceEqual(got, []int{1, 2, 3}) {
		t.Errorf("expected [1 2 3], got %v", got)
	}
}

func TestThrottle_Empty(t *testing.T) {
	p := FromSlice([]int{})
	throttled := Throttle(p, time.Second)
	got, err := Collect(context.Background(), throttled)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

// --- Batch tests ---

func TestBatch_BySize(t *testing.T) {
	p := FromSlice([]int{1, 2, 3, 4, 5})
	batched := Batch(p, 2, 0)
	got, err := Collect(context.Background(), batched)
	if err != nil {
		t.Fatal(err)
	}
	// Expect: [1,2], [3,4], [5]
	if len(got) != 3 {
		t.Fatalf("expected 3 batches, got %d: %v", len(got), got)
	}
	if !intSliceEqual(got[0], []int{1, 2}) {
		t.Errorf("batch 0: expected [1 2], got %v", got[0])
	}
	if !intSliceEqual(got[1], []int{3, 4}) {
		t.Errorf("batch 1: expected [3 4], got %v", got[1])
	}
	if !intSliceEqual(got[2], []int{5}) {
		t.Errorf("batch 2: expected [5], got %v", got[2])
	}
}

func TestBatch_ExactMultiple(t *testing.T) {
	p := FromSlice([]int{1, 2, 3, 4})
	batched := Batch(p, 2, 0)
	got, err := Collect(context.Background(), batched)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 batches, got %d: %v", len(got), got)
	}
}

func TestBatch_SizeOne(t *testing.T) {
	p := FromSlice([]int{1, 2, 3})
	batched := Batch(p, 1, 0)
	got, err := Collect(context.Background(), batched)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 batches, got %d", len(got))
	}
	for i, b := range got {
		if len(b) != 1 || b[0] != i+1 {
			t.Errorf("batch %d: expected [%d], got %v", i, i+1, b)
		}
	}
}

func TestBatch_Empty(t *testing.T) {
	p := FromSlice([]int{})
	batched := Batch(p, 3, 0)
	got, err := Collect(context.Background(), batched)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestBatch_DefaultsOnZeroZero(t *testing.T) {
	p := FromSlice([]int{1, 2, 3})
	batched := Batch(p, 0, 0) // defaults to size=1
	got, err := Collect(context.Background(), batched)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 batches (size=1 default), got %d", len(got))
	}
}

// --- Debounce tests ---

func TestDebounce_EmitsAfterQuiet(t *testing.T) {
	// Use a channel-based source to control timing
	ch := make(chan result[int], 10)
	ch <- result[int]{val: 1, ok: true}
	ch <- result[int]{val: 2, ok: true}
	ch <- result[int]{val: 3, ok: true}
	close(ch)

	src := FromFunc(func(ctx context.Context) Iterator[int] {
		return &channelIter[int]{ch: ch}
	})

	debounced := Debounce(src, 50*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	got, err := Collect(ctx, debounced)
	if err != nil {
		t.Fatal(err)
	}
	// Should emit only the last value (3) after quiet period
	if len(got) != 1 || got[0] != 3 {
		t.Errorf("expected [3] (last after debounce), got %v", got)
	}
}

func TestDebounce_Empty(t *testing.T) {
	ch := make(chan result[int])
	close(ch)

	src := FromFunc(func(ctx context.Context) Iterator[int] {
		return &channelIter[int]{ch: ch}
	})

	debounced := Debounce(src, 50*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	got, err := Collect(ctx, debounced)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

// --- TumblingWindow tests ---

func TestTumblingWindow_GroupsByTime(t *testing.T) {
	ch := make(chan result[int], 10)

	src := FromFunc(func(ctx context.Context) Iterator[int] {
		return &channelIter[int]{ch: ch}
	})

	windowed := TumblingWindow(src, 100*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Send values in rapid succession, then close
	go func() {
		ch <- result[int]{val: 1, ok: true}
		ch <- result[int]{val: 2, ok: true}
		ch <- result[int]{val: 3, ok: true}
		time.Sleep(150 * time.Millisecond)
		ch <- result[int]{val: 4, ok: true}
		ch <- result[int]{val: 5, ok: true}
		close(ch)
	}()

	got, err := Collect(ctx, windowed)
	if err != nil {
		t.Fatal(err)
	}

	if len(got) < 1 {
		t.Fatal("expected at least 1 window")
	}

	// Total values should be 5
	total := 0
	for _, w := range got {
		total += len(w)
	}
	if total != 5 {
		t.Errorf("expected 5 total values across windows, got %d", total)
	}
}

func TestTumblingWindow_Empty(t *testing.T) {
	ch := make(chan result[int])
	close(ch)

	src := FromFunc(func(ctx context.Context) Iterator[int] {
		return &channelIter[int]{ch: ch}
	})

	windowed := TumblingWindow(src, 50*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	got, err := Collect(ctx, windowed)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

// --- SlidingWindow tests ---

type timedValue struct {
	val int
	ts  time.Time
}

func TestSlidingWindow_Overlapping(t *testing.T) {
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	items := []timedValue{
		{val: 1, ts: base},
		{val: 2, ts: base.Add(10 * time.Millisecond)},
		{val: 3, ts: base.Add(20 * time.Millisecond)},
		{val: 4, ts: base.Add(30 * time.Millisecond)},
		{val: 5, ts: base.Add(40 * time.Millisecond)},
	}

	p := FromSlice(items)
	windowed := SlidingWindow(p,
		func(v timedValue) time.Time { return v.ts },
		30*time.Millisecond, // window size
		10*time.Millisecond, // slide by
	)

	got, err := Collect(context.Background(), windowed)
	if err != nil {
		t.Fatal(err)
	}

	if len(got) == 0 {
		t.Fatal("expected at least 1 window")
	}

	// First window [0ms, 30ms) should contain val 1, 2, 3
	firstVals := extractVals(got[0])
	if !intSliceEqual(firstVals, []int{1, 2, 3}) {
		t.Errorf("first window: expected [1 2 3], got %v", firstVals)
	}
}

func TestSlidingWindow_Empty(t *testing.T) {
	p := FromSlice([]timedValue{})
	windowed := SlidingWindow(p,
		func(v timedValue) time.Time { return v.ts },
		time.Second,
		500*time.Millisecond,
	)
	got, err := Collect(context.Background(), windowed)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func extractVals(items []timedValue) []int {
	vals := make([]int, len(items))
	for i, item := range items {
		vals[i] = item.val
	}
	return vals
}

// --- Additional coverage tests ---

func TestBatch_WithTimeout(t *testing.T) {
	// Use a slow source to trigger the timeout path
	ch := make(chan result[int], 10)
	go func() {
		ch <- result[int]{val: 1, ok: true}
		ch <- result[int]{val: 2, ok: true}
		// Don't send more — let timeout fire
		time.Sleep(200 * time.Millisecond)
		ch <- result[int]{val: 3, ok: true}
		close(ch)
	}()

	src := FromFunc(func(ctx context.Context) Iterator[int] {
		return &channelIter[int]{ch: ch}
	})

	batched := Batch(src, 100, 80*time.Millisecond) // large size, small timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	got, err := Collect(ctx, batched)
	if err != nil {
		t.Fatal(err)
	}
	// Should get at least 1 batch from timeout
	if len(got) < 1 {
		t.Fatalf("expected at least 1 batch, got %d", len(got))
	}
	// Total values should be 3
	total := 0
	for _, b := range got {
		total += len(b)
	}
	if total != 3 {
		t.Errorf("expected 3 total values, got %d", total)
	}
}

func TestBatch_OnlyTimeout(t *testing.T) {
	// size=0 means collect until timeout only
	ch := make(chan result[int], 10)
	go func() {
		ch <- result[int]{val: 1, ok: true}
		ch <- result[int]{val: 2, ok: true}
		ch <- result[int]{val: 3, ok: true}
		close(ch)
	}()

	src := FromFunc(func(ctx context.Context) Iterator[int] {
		return &channelIter[int]{ch: ch}
	})

	batched := Batch(src, 0, 200*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	got, err := Collect(ctx, batched)
	if err != nil {
		t.Fatal(err)
	}
	// All values should arrive in batch(es)
	total := 0
	for _, b := range got {
		total += len(b)
	}
	if total != 3 {
		t.Errorf("expected 3 total values, got %d", total)
	}
}

func TestThrottle_SingleValue(t *testing.T) {
	p := FromSlice([]int{42})
	throttled := Throttle(p, time.Hour)
	got, err := Collect(context.Background(), throttled)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != 42 {
		t.Errorf("expected [42], got %v", got)
	}
}

func TestDebounce_ContextCancelled(t *testing.T) {
	// Source that blocks forever
	ch := make(chan result[int])
	src := FromFunc(func(ctx context.Context) Iterator[int] {
		return &channelIter[int]{ch: ch}
	})

	debounced := Debounce(src, 50*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := Collect(ctx, debounced)
	if err == nil {
		t.Fatal("expected context error")
	}
}

func TestTumblingWindow_ContextCancelled(t *testing.T) {
	ch := make(chan result[int])
	src := FromFunc(func(ctx context.Context) Iterator[int] {
		return &channelIter[int]{ch: ch}
	})

	windowed := TumblingWindow(src, time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := Collect(ctx, windowed)
	if err == nil {
		t.Fatal("expected context error")
	}
}

func TestDebounce_SingleValue(t *testing.T) {
	ch := make(chan result[string], 1)
	ch <- result[string]{val: "only", ok: true}
	close(ch)

	src := FromFunc(func(ctx context.Context) Iterator[string] {
		return &channelIter[string]{ch: ch}
	})

	debounced := Debounce(src, 50*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	got, err := Collect(ctx, debounced)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "only" {
		t.Errorf("expected [only], got %v", got)
	}
}

func TestTumblingWindow_SingleBurst(t *testing.T) {
	ch := make(chan result[int], 5)
	for i := 1; i <= 5; i++ {
		ch <- result[int]{val: i, ok: true}
	}
	close(ch)

	src := FromFunc(func(ctx context.Context) Iterator[int] {
		return &channelIter[int]{ch: ch}
	})

	windowed := TumblingWindow(src, time.Hour) // huge window
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	got, err := Collect(ctx, windowed)
	if err != nil {
		t.Fatal(err)
	}
	// All values should be in one window (source closes before window expires)
	if len(got) != 1 {
		t.Fatalf("expected 1 window, got %d", len(got))
	}
	if !intSliceEqual(got[0], []int{1, 2, 3, 4, 5}) {
		t.Errorf("expected [1 2 3 4 5], got %v", got[0])
	}
}
