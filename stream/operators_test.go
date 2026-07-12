package stream

import (
	"context"
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
