package stream

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// errCloseIter is a test iterator that yields items and returns a sentinel
// error from Close, used to verify that flatMap surfaces inner close errors.
type errCloseIter[T any] struct {
	items    []T
	index    int
	closeErr error
	nextErr  error
	closed   int
}

func (it *errCloseIter[T]) Next(_ context.Context) (value T, ok bool, err error) {
	if it.nextErr != nil && it.index >= len(it.items) {
		var zero T
		return zero, false, it.nextErr
	}
	if it.index >= len(it.items) {
		var zero T
		return zero, false, nil
	}
	v := it.items[it.index]
	it.index++
	return v, true, nil
}

func (it *errCloseIter[T]) Close() error {
	it.closed++
	return it.closeErr
}

func TestFlatMap_InnerCloseErrorPropagatedOnFinalClose(t *testing.T) {
	t.Parallel()

	errInner1 := errors.New("inner-1 close failed")
	errInner2 := errors.New("inner-2 close failed")
	inners := map[int]*errCloseIter[int]{
		1: {items: []int{10}, closeErr: errInner1},
		2: {items: []int{20}, closeErr: errInner2},
	}

	p := FlatMap(FromSlice([]int{1, 2}), func(_ context.Context, n int) (Iterator[int], error) {
		return inners[n], nil
	})

	ctx := context.Background()
	iter := p.Iter(ctx)

	var got []int
	for {
		v, ok, err := iter.Next(ctx)
		if err != nil {
			t.Fatalf("unexpected Next error: %v", err)
		}
		if !ok {
			break
		}
		got = append(got, v)
	}
	if !intSliceEqual(got, []int{10, 20}) {
		t.Fatalf("got %v, want [10 20]", got)
	}

	err := iter.Close()
	if !errors.Is(err, errInner1) {
		t.Fatalf("Close() = %v, want it to surface errInner1", err)
	}
	if !errors.Is(err, errInner2) {
		t.Fatalf("Close() = %v, want it to surface errInner2", err)
	}
}

func TestFlatMap_CloseJoinsCurrentInnerAndSourceErrors(t *testing.T) {
	t.Parallel()

	errSource := errors.New("source close failed")
	errInner := errors.New("inner close failed")
	source := &errCloseIter[int]{items: []int{1}, closeErr: errSource}
	inner := &errCloseIter[int]{items: []int{10, 11}, closeErr: errInner}

	p := FlatMap(From[int](source), func(_ context.Context, _ int) (Iterator[int], error) {
		return inner, nil
	})

	ctx := context.Background()
	iter := p.Iter(ctx)

	// Pull one value so the current inner iterator is live (not yet exhausted).
	if _, ok, err := iter.Next(ctx); err != nil || !ok {
		t.Fatalf("Next() = _, %v, %v; want _, true, nil", ok, err)
	}

	err := iter.Close()
	if !errors.Is(err, errInner) {
		t.Fatalf("Close() = %v, want it to surface the current inner close error", err)
	}
	if !errors.Is(err, errSource) {
		t.Fatalf("Close() = %v, want it to surface the source close error", err)
	}
}

func TestFlatMap_SourceErrorNotMaskedByCloseError(t *testing.T) {
	t.Parallel()

	errData := errors.New("source data error")
	errInner := errors.New("inner close failed")
	source := &errCloseIter[int]{items: []int{1}, nextErr: errData}
	inner := &errCloseIter[int]{items: []int{10}, closeErr: errInner}

	p := FlatMap(From[int](source), func(_ context.Context, _ int) (Iterator[int], error) {
		return inner, nil
	})

	ctx := context.Background()
	iter := p.Iter(ctx)

	// First value from the inner iterator.
	if _, ok, err := iter.Next(ctx); err != nil || !ok {
		t.Fatalf("Next() = _, %v, %v; want _, true, nil", ok, err)
	}
	// Inner exhausts (accumulating its close error), then the source returns a
	// data error which must surface unmasked from Next.
	if _, _, err := iter.Next(ctx); !errors.Is(err, errData) {
		t.Fatalf("Next() = _, _, %v; want the source data error", err)
	}
	// The accumulated inner close error is still surfaced on Close, without
	// having masked the source data error above.
	if err := iter.Close(); !errors.Is(err, errInner) {
		t.Fatalf("Close() = %v, want it to surface the accumulated inner close error", err)
	}
}

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

// FlatMap bounds retained inner close errors: beyond the cap, further failures
// are dropped rather than accumulated for the lifetime of the stream.
func TestFlatMap_InnerCloseErrorsAreBounded(t *testing.T) {
	t.Parallel()

	const n = maxRetainedInnerCloseErrors + 4
	errs := make([]error, n)
	inners := make(map[int]*errCloseIter[int], n)
	seeds := make([]int, n)
	for i := 0; i < n; i++ {
		errs[i] = fmt.Errorf("inner-%d close failed", i)
		inners[i] = &errCloseIter[int]{items: []int{i}, closeErr: errs[i]}
		seeds[i] = i
	}

	p := FlatMap(FromSlice(seeds), func(_ context.Context, k int) (Iterator[int], error) {
		return inners[k], nil
	})

	ctx := context.Background()
	iter := p.Iter(ctx)
	for {
		if _, ok, err := iter.Next(ctx); err != nil {
			t.Fatalf("unexpected Next error: %v", err)
		} else if !ok {
			break
		}
	}

	err := iter.Close()
	// The first cap errors are retained and surfaced.
	for i := 0; i < maxRetainedInnerCloseErrors; i++ {
		if !errors.Is(err, errs[i]) {
			t.Fatalf("Close() = %v, want it to surface errs[%d]", err, i)
		}
	}
	// Errors beyond the cap are dropped, keeping retention bounded.
	for i := maxRetainedInnerCloseErrors; i < n; i++ {
		if errors.Is(err, errs[i]) {
			t.Fatalf("Close() surfaced errs[%d] beyond the retention cap", i)
		}
	}
}
