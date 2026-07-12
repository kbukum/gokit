package stream

import (
	"context"
	"testing"
	"time"
)

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
