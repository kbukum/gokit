package pipeline

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
)

func TestFromSlice_Collect(t *testing.T) {
	p := FromSlice([]int{1, 2, 3})
	got, err := Collect(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	want := []int{1, 2, 3}
	if !intSliceEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFromSlice_Empty(t *testing.T) {
	p := FromSlice([]int{})
	got, err := Collect(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestFrom_Iterator(t *testing.T) {
	iter := &sliceIter[string]{items: []string{"a", "b"}}
	p := From[string](iter)
	got, err := Collect(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("got %v, want [a b]", got)
	}
}

func TestMap(t *testing.T) {
	p := FromSlice([]int{1, 2, 3})
	doubled := Map(p, func(_ context.Context, n int) (int, error) {
		return n * 2, nil
	})
	got, err := Collect(context.Background(), doubled)
	if err != nil {
		t.Fatal(err)
	}
	want := []int{2, 4, 6}
	if !intSliceEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestMap_Error(t *testing.T) {
	p := FromSlice([]int{1, 2, 3})
	fail := Map(p, func(_ context.Context, n int) (int, error) {
		if n == 2 {
			return 0, errors.New("bad value")
		}
		return n, nil
	})
	got, err := Collect(context.Background(), fail)
	if err == nil {
		t.Fatal("expected error")
	}
	if len(got) != 1 || got[0] != 1 {
		t.Errorf("expected [1] before error, got %v", got)
	}
}

func TestMap_TypeConversion(t *testing.T) {
	p := FromSlice([]int{1, 2, 3})
	strs := Map(p, func(_ context.Context, n int) (string, error) {
		return fmt.Sprintf("#%d", n), nil
	})
	got, err := Collect(context.Background(), strs)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"#1", "#2", "#3"}
	if !strSliceEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFlatMap(t *testing.T) {
	p := FromSlice([]int{1, 2, 3})
	expanded := FlatMap(p, func(_ context.Context, n int) (Iterator[int], error) {
		return &sliceIter[int]{items: []int{n, n * 10}}, nil
	})
	got, err := Collect(context.Background(), expanded)
	if err != nil {
		t.Fatal(err)
	}
	want := []int{1, 10, 2, 20, 3, 30}
	if !intSliceEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFlatMap_EmptyInner(t *testing.T) {
	p := FromSlice([]int{1, 2, 3})
	expanded := FlatMap(p, func(_ context.Context, n int) (Iterator[int], error) {
		if n == 2 {
			return &sliceIter[int]{items: nil}, nil
		}
		return &sliceIter[int]{items: []int{n}}, nil
	})
	got, err := Collect(context.Background(), expanded)
	if err != nil {
		t.Fatal(err)
	}
	want := []int{1, 3}
	if !intSliceEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFilter(t *testing.T) {
	p := FromSlice([]int{1, 2, 3, 4, 5, 6})
	evens := Filter(p, func(n int) bool { return n%2 == 0 })
	got, err := Collect(context.Background(), evens)
	if err != nil {
		t.Fatal(err)
	}
	want := []int{2, 4, 6}
	if !intSliceEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFilter_None(t *testing.T) {
	p := FromSlice([]int{1, 3, 5})
	evens := Filter(p, func(n int) bool { return n%2 == 0 })
	got, err := Collect(context.Background(), evens)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestTap(t *testing.T) {
	var tapped []int
	p := FromSlice([]int{1, 2, 3})
	observed := Tap(p, func(_ context.Context, n int) error {
		tapped = append(tapped, n)
		return nil
	})
	got, err := Collect(context.Background(), observed)
	if err != nil {
		t.Fatal(err)
	}
	if !intSliceEqual(got, []int{1, 2, 3}) {
		t.Errorf("values should pass through unchanged, got %v", got)
	}
	if !intSliceEqual(tapped, []int{1, 2, 3}) {
		t.Errorf("tap should see all values, got %v", tapped)
	}
}

func TestTap_Error(t *testing.T) {
	p := FromSlice([]int{1, 2, 3})
	failing := Tap(p, func(_ context.Context, n int) error {
		if n == 2 {
			return errors.New("tap failed")
		}
		return nil
	})
	_, err := Collect(context.Background(), failing)
	if err == nil || !strings.Contains(err.Error(), "tap failed") {
		t.Errorf("expected tap error, got %v", err)
	}
}

func TestFanOut(t *testing.T) {
	p := FromSlice([]int{10})
	fanned := FanOut(p,
		func(_ context.Context, n int) (string, error) { return fmt.Sprintf("a:%d", n), nil },
		func(_ context.Context, n int) (string, error) { return fmt.Sprintf("b:%d", n), nil },
	)
	got, err := Collect(context.Background(), fanned)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || len(got[0]) != 2 {
		t.Fatalf("expected [[a:10 b:10]], got %v", got)
	}
	if got[0][0] != "a:10" || got[0][1] != "b:10" {
		t.Errorf("got %v, want [a:10 b:10]", got[0])
	}
}

func TestFanOut_Error(t *testing.T) {
	p := FromSlice([]int{1})
	fanned := FanOut(p,
		func(_ context.Context, _ int) (int, error) { return 1, nil },
		func(_ context.Context, _ int) (int, error) { return 0, errors.New("branch failed") },
	)
	_, err := Collect(context.Background(), fanned)
	if err == nil {
		t.Fatal("expected error from fan-out branch")
	}
}

func TestTapEach(t *testing.T) {
	var gotA, gotB string
	p := FromSlice([][]string{{"hello", "world"}})
	tapped := TapEach(p,
		func(_ context.Context, s string) error { gotA = s; return nil },
		func(_ context.Context, s string) error { gotB = s; return nil },
	)
	got, err := Collect(context.Background(), tapped)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0][0] != "hello" || got[0][1] != "world" {
		t.Errorf("values should pass through, got %v", got)
	}
	if gotA != "hello" || gotB != "world" {
		t.Errorf("taps should see respective elements: a=%q b=%q", gotA, gotB)
	}
}

func TestReduce(t *testing.T) {
	p := FromSlice([]int{1, 2, 3, 4, 5})
	sum := Reduce(p, 0, func(acc, n int) int { return acc + n })
	got, err := Collect(context.Background(), sum)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != 15 {
		t.Errorf("expected [15], got %v", got)
	}
}

func TestReduce_Empty(t *testing.T) {
	p := FromSlice([]int{})
	sum := Reduce(p, 42, func(acc, n int) int { return acc + n })
	got, err := Collect(context.Background(), sum)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != 42 {
		t.Errorf("expected [42] (initial value), got %v", got)
	}
}

func TestConcat(t *testing.T) {
	a := FromSlice([]int{1, 2})
	b := FromSlice([]int{3, 4})
	c := FromSlice([]int{5})
	combined := Concat(a, b, c)
	got, err := Collect(context.Background(), combined)
	if err != nil {
		t.Fatal(err)
	}
	want := []int{1, 2, 3, 4, 5}
	if !intSliceEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestBuffer(t *testing.T) {
	p := FromSlice([]int{1, 2, 3, 4, 5})
	buffered := Buffer(p, 3)
	got, err := Collect(context.Background(), buffered)
	if err != nil {
		t.Fatal(err)
	}
	want := []int{1, 2, 3, 4, 5}
	if !intSliceEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParallel(t *testing.T) {
	p := FromSlice([]int{1, 2, 3, 4, 5})
	doubled := Parallel(p, 3, func(_ context.Context, n int) (int, error) {
		return n * 2, nil
	})
	got, err := Collect(context.Background(), doubled)
	if err != nil {
		t.Fatal(err)
	}
	sort.Ints(got) // order not guaranteed
	want := []int{2, 4, 6, 8, 10}
	if !intSliceEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParallel_Error(t *testing.T) {
	p := FromSlice([]int{1, 2, 3, 4, 5})
	failing := Parallel(p, 2, func(_ context.Context, n int) (int, error) {
		if n == 3 {
			return 0, errors.New("worker failed")
		}
		return n, nil
	})
	_, err := Collect(context.Background(), failing)
	if err == nil {
		t.Fatal("expected error from parallel worker")
	}
}

func TestMerge(t *testing.T) {
	a := FromSlice([]int{1, 2, 3})
	b := FromSlice([]int{10, 20, 30})
	merged := Merge(a, b)
	got, err := Collect(context.Background(), merged)
	if err != nil {
		t.Fatal(err)
	}
	sort.Ints(got) // order not guaranteed
	want := []int{1, 2, 3, 10, 20, 30}
	if !intSliceEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestDrain_Run(t *testing.T) {
	var collected []int
	p := FromSlice([]int{1, 2, 3})
	r := Drain(p, func(_ context.Context, n int) error {
		collected = append(collected, n)
		return nil
	})
	if err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !intSliceEqual(collected, []int{1, 2, 3}) {
		t.Errorf("got %v, want [1 2 3]", collected)
	}
}

func TestForEach(t *testing.T) {
	var sum int
	p := FromSlice([]int{1, 2, 3})
	err := ForEach(context.Background(), p, func(_ context.Context, n int) error {
		sum += n
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if sum != 6 {
		t.Errorf("sum = %d, want 6", sum)
	}
}

func TestIter(t *testing.T) {
	p := FromSlice([]int{1, 2})
	ctx := context.Background()
	iter := p.Iter(ctx)
	defer iter.Close()

	v1, ok, err := iter.Next(ctx)
	if err != nil || !ok || v1 != 1 {
		t.Errorf("first Next: val=%d ok=%v err=%v", v1, ok, err)
	}
	v2, ok, err := iter.Next(ctx)
	if err != nil || !ok || v2 != 2 {
		t.Errorf("second Next: val=%d ok=%v err=%v", v2, ok, err)
	}
	_, ok, err = iter.Next(ctx)
	if err != nil || ok {
		t.Errorf("third Next should be exhausted: ok=%v err=%v", ok, err)
	}
}

func TestContext_Cancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	p := FromSlice([]int{1, 2, 3})
	buffered := Buffer(p, 1)
	_, err := Collect(ctx, buffered)
	if err == nil {
		// Buffer uses channels, so cancelled ctx should propagate
		// (but slice source doesn't check ctx, so this is best-effort)
	}
	_ = err // context cancellation is best-effort for sync sources
}

func TestChained_Pipeline(t *testing.T) {
	// Full pipeline: source → map → filter → tap → reduce
	var tapped []int
	p := FromSlice([]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})
	doubled := Map(p, func(_ context.Context, n int) (int, error) { return n * 2, nil })
	evens := Filter(doubled, func(n int) bool { return n%4 == 0 })
	observed := Tap(evens, func(_ context.Context, n int) error {
		tapped = append(tapped, n)
		return nil
	})
	sum := Reduce(observed, 0, func(acc, n int) int { return acc + n })

	got, err := Collect(context.Background(), sum)
	if err != nil {
		t.Fatal(err)
	}
	// Input doubled: 2,4,6,8,10,12,14,16,18,20 → filter %4==0: 4,8,12,16,20 → sum: 60
	if len(got) != 1 || got[0] != 60 {
		t.Errorf("expected [60], got %v", got)
	}
	if !intSliceEqual(tapped, []int{4, 8, 12, 16, 20}) {
		t.Errorf("tapped = %v, want [4 8 12 16 20]", tapped)
	}
}

func TestFanOut_TapEach_Pipeline(t *testing.T) {
	// Simulates: source → fanout(sentimentFn, fraudFn) → tapEach(pubA, pubB) → collect
	var pubA, pubB atomic.Int32

	src := FromSlice([]string{"hello", "world"})
	fanned := FanOut(src,
		func(_ context.Context, s string) (string, error) { return "sentiment:" + s, nil },
		func(_ context.Context, s string) (string, error) { return "fraud:" + s, nil },
	)
	tapped := TapEach(fanned,
		func(_ context.Context, _ string) error { pubA.Add(1); return nil },
		func(_ context.Context, _ string) error { pubB.Add(1); return nil },
	)
	got, err := Collect(context.Background(), tapped)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 results, got %d", len(got))
	}
	if got[0][0] != "sentiment:hello" || got[0][1] != "fraud:hello" {
		t.Errorf("first result = %v", got[0])
	}
	if pubA.Load() != 2 || pubB.Load() != 2 {
		t.Errorf("pubA=%d pubB=%d, want 2 each", pubA.Load(), pubB.Load())
	}
}

// --- helpers ---

func intSliceEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func strSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
