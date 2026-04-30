package pipeline

import (
	"context"
	"errors"
	"testing"
)

func TestDistinct(t *testing.T) {
	t.Parallel()

	got, err := Collect(context.Background(), Distinct(FromSlice([]int{1, 1, 2, 3, 2, 4, 4})))
	if err != nil {
		t.Fatal(err)
	}
	if !intSliceEqual(got, []int{1, 2, 3, 4}) {
		t.Fatalf("got %v, want [1 2 3 4]", got)
	}
}

func TestTake(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		n    int
		want []int
	}{
		{name: "positive", n: 3, want: []int{1, 2, 3}},
		{name: "zero", n: 0, want: nil},
		{name: "negative", n: -1, want: nil},
		{name: "overflow", n: 10, want: []int{1, 2, 3, 4, 5}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := Collect(context.Background(), Take(FromSlice([]int{1, 2, 3, 4, 5}), tt.n))
			if err != nil {
				t.Fatal(err)
			}
			if !intSliceEqual(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSkip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		n    int
		want []int
	}{
		{name: "positive", n: 2, want: []int{3, 4, 5}},
		{name: "zero", n: 0, want: []int{1, 2, 3, 4, 5}},
		{name: "negative", n: -3, want: []int{1, 2, 3, 4, 5}},
		{name: "overflow", n: 10, want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := Collect(context.Background(), Skip(FromSlice([]int{1, 2, 3, 4, 5}), tt.n))
			if err != nil {
				t.Fatal(err)
			}
			if !intSliceEqual(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPartition(t *testing.T) {
	t.Parallel()

	even, odd := Partition(FromSlice([]int{1, 2, 3, 4, 5, 6}), func(v int) bool { return v%2 == 0 })

	evens, err := Collect(context.Background(), even)
	if err != nil {
		t.Fatal(err)
	}
	odds, err := Collect(context.Background(), odd)
	if err != nil {
		t.Fatal(err)
	}

	if !intSliceEqual(evens, []int{2, 4, 6}) {
		t.Fatalf("evens = %v, want [2 4 6]", evens)
	}
	if !intSliceEqual(odds, []int{1, 3, 5}) {
		t.Fatalf("odds = %v, want [1 3 5]", odds)
	}
}

func TestPartition_PropagatesSourceError(t *testing.T) {
	t.Parallel()

	expected := errors.New("boom")
	src := FromFunc(func(ctx context.Context) Iterator[int] {
		return &channelIter[int]{ch: channelWithError(expected, 1, 2)}
	})

	matching, rejected := Partition(src, func(v int) bool { return v%2 == 0 })

	evens, err := Collect(context.Background(), matching)
	if !errors.Is(err, expected) {
		t.Fatalf("expected error %v, got %v", expected, err)
	}
	if !intSliceEqual(evens, []int{2}) {
		t.Fatalf("evens = %v, want [2]", evens)
	}

	odds, err := Collect(context.Background(), rejected)
	if !errors.Is(err, expected) {
		t.Fatalf("expected error %v, got %v", expected, err)
	}
	if !intSliceEqual(odds, []int{1}) {
		t.Fatalf("odds = %v, want [1]", odds)
	}
}

func channelWithError(err error, values ...int) <-chan result[int] {
	ch := make(chan result[int], len(values)+1)
	for _, value := range values {
		ch <- result[int]{val: value, ok: true}
	}
	ch <- result[int]{err: err}
	close(ch)
	return ch
}
