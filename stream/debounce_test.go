package stream

import (
	"context"
	"testing"
	"time"
)

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
