package stream

import (
	"context"
	"testing"
	"time"
)

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
