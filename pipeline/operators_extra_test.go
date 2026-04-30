package pipeline

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
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
	evens, odds, err := collectPartitionPair(context.Background(), even, odd)
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

func TestPartitionLargeStreamBoundedTee(t *testing.T) {
	t.Parallel()

	items := make([]int, 10_000)
	for i := range items {
		items[i] = i
	}
	matching, rejected := Partition(FromSlice(items), func(v int) bool { return v%2 == 0 })

	evens, odds, err := collectPartitionPair(context.Background(), matching, rejected)
	if err != nil {
		t.Fatal(err)
	}
	if len(evens) != 5_000 || len(odds) != 5_000 {
		t.Fatalf("len(evens)=%d len(odds)=%d, want 5000/5000", len(evens), len(odds))
	}
	for i, v := range evens {
		if want := i * 2; v != want {
			t.Fatalf("evens[%d]=%d, want %d", i, v, want)
		}
	}
	for i, v := range odds {
		if want := i*2 + 1; v != want {
			t.Fatalf("odds[%d]=%d, want %d", i, v, want)
		}
	}
}

func TestPartitionCloseOneBranchKeepsOtherBranchAlive(t *testing.T) {
	t.Parallel()

	items := make([]int, 10_000)
	for i := range items {
		items[i] = i
	}
	matching, rejected := Partition(FromSlice(items), func(v int) bool { return v%2 == 0 })
	ctx := context.Background()
	rejectedIter := rejected.Iter(ctx)
	if err := rejectedIter.Close(); err != nil {
		t.Fatalf("close rejected branch: %v", err)
	}

	evens, err := Collect(ctx, matching)
	if err != nil {
		t.Fatal(err)
	}
	if len(evens) != 5_000 {
		t.Fatalf("len(evens)=%d, want 5000", len(evens))
	}
}

func TestPartitionPropagatesSourceErrorOnceToBothBranches(t *testing.T) {
	t.Parallel()

	expected := errors.New("boom")
	src := FromFunc(func(ctx context.Context) Iterator[int] {
		return &channelIter[int]{ch: channelWithError(expected, 1, 2, 3, 4)}
	})

	matching, rejected := Partition(src, func(v int) bool { return v%2 == 0 })
	evens, odds, err := collectPartitionPair(context.Background(), matching, rejected)
	if !errors.Is(err, expected) {
		t.Fatalf("expected error %v, got %v", expected, err)
	}
	if !intSliceEqual(evens, []int{2, 4}) {
		t.Fatalf("evens = %v, want [2 4]", evens)
	}
	if !intSliceEqual(odds, []int{1, 3}) {
		t.Fatalf("odds = %v, want [1 3]", odds)
	}

	iter := matching.Iter(context.Background())
	defer iter.Close() //nolint:errcheck // test cleanup
	_, ok, err := iter.Next(context.Background())
	if ok || err != nil {
		t.Fatalf("terminal error repeated: ok=%v err=%v", ok, err)
	}
}

func TestPartitionCreateContextCancelTerminatesBranches(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	matching, rejected := Partition(FromFunc(func(context.Context) Iterator[int] {
		return &countingIter{}
	}), func(v int) bool { return v%2 == 0 })

	errCh := make(chan error, 2)
	go func() { _, err := Collect(ctx, matching); errCh <- err }()
	go func() { _, err := Collect(ctx, rejected); errCh <- err }()
	cancel()

	for range 2 {
		select {
		case err := <-errCh:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("expected context.Canceled, got %v", err)
			}
		case <-afterTestTimeout():
			t.Fatal("partition branches did not terminate after context cancel")
		}
	}
}

func TestPartitionPredicatePanicSurfacesToBothBranches(t *testing.T) {
	t.Parallel()

	matching, rejected := Partition(FromSlice([]int{1, 2, 3}), func(v int) bool {
		if v == 2 {
			panic("bad predicate")
		}
		return v%2 == 0
	})
	_, _, err := collectPartitionPair(context.Background(), matching, rejected)
	if err == nil || !strings.Contains(err.Error(), "bad predicate") {
		t.Fatalf("expected predicate panic error, got %v", err)
	}
}

func TestPartitionClosingBothBranchesClosesUpstream(t *testing.T) {
	t.Parallel()

	src := &blockingCloseIter{started: make(chan struct{}), closed: make(chan struct{})}
	matching, rejected := Partition(From(src), func(v int) bool { return v%2 == 0 })
	ctx := context.Background()
	left := matching.Iter(ctx)
	right := rejected.Iter(ctx)
	go func() {
		_, _, _ = left.Next(ctx)
	}()
	<-src.started
	if err := left.Close(); err != nil {
		t.Fatalf("close left: %v", err)
	}
	if err := right.Close(); err != nil {
		t.Fatalf("close right: %v", err)
	}
	select {
	case <-src.closed:
	case <-afterTestTimeout():
		t.Fatal("upstream was not closed after both partition branches closed")
	}
}

func collectPartitionPair(ctx context.Context, left, right *Pipeline[int]) (leftItems []int, rightItems []int, err error) {
	type collected struct {
		items []int
		err   error
	}
	leftCh := make(chan collected, 1)
	rightCh := make(chan collected, 1)
	go func() {
		items, err := Collect(ctx, left)
		leftCh <- collected{items: items, err: err}
	}()
	go func() {
		items, err := Collect(ctx, right)
		rightCh <- collected{items: items, err: err}
	}()
	l := <-leftCh
	r := <-rightCh
	if l.err != nil {
		return l.items, r.items, l.err
	}
	return l.items, r.items, r.err
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

type countingIter struct {
	n int
}

func (it *countingIter) Next(ctx context.Context) (value int, ok bool, err error) {
	select {
	case <-ctx.Done():
		return 0, false, ctx.Err()
	default:
		it.n++
		return it.n, true, nil
	}
}

func (it *countingIter) Close() error { return nil }

type blockingCloseIter struct {
	started chan struct{}
	closed  chan struct{}
}

func (it *blockingCloseIter) Next(ctx context.Context) (value int, ok bool, err error) {
	select {
	case <-it.started:
	default:
		close(it.started)
	}
	<-ctx.Done()
	return 0, false, ctx.Err()
}

func (it *blockingCloseIter) Close() error {
	select {
	case <-it.closed:
	default:
		close(it.closed)
	}
	return nil
}

func afterTestTimeout() <-chan time.Time {
	return time.After(time.Second)
}
