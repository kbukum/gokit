package provider_test

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/kbukum/gokit/provider"
)

// --- Test helpers ---

func newSliceIter[T any](items ...T) provider.Iterator[T] {
	return &sliceIterator[T]{items: items}
}

type streamTestHelper[I, O any] struct {
	name string
	fn   func(ctx context.Context, input I) (provider.Iterator[O], error)
}

func (s *streamTestHelper[I, O]) Name() string                       { return s.name }
func (s *streamTestHelper[I, O]) IsAvailable(_ context.Context) bool { return true }
func (s *streamTestHelper[I, O]) Execute(ctx context.Context, input I) (provider.Iterator[O], error) {
	return s.fn(ctx, input)
}

type rrTestHelper[I, O any] struct {
	name string
	fn   func(ctx context.Context, input I) (O, error)
}

func (t *rrTestHelper[I, O]) Name() string                       { return t.name }
func (t *rrTestHelper[I, O]) IsAvailable(_ context.Context) bool { return true }
func (t *rrTestHelper[I, O]) Execute(ctx context.Context, input I) (O, error) {
	return t.fn(ctx, input)
}

// --- FanOutStream tests ---

func TestFanOutStream_Basic(t *testing.T) {
	s1 := &streamTestHelper[string, int]{
		name: "s1",
		fn: func(_ context.Context, _ string) (provider.Iterator[int], error) {
			return newSliceIter(1, 2, 3), nil
		},
	}
	s2 := &streamTestHelper[string, int]{
		name: "s2",
		fn: func(_ context.Context, _ string) (provider.Iterator[int], error) {
			return newSliceIter(10, 20), nil
		},
	}

	fan := provider.FanOutStream("fan", s1, s2)
	if fan.Name() != "fan" {
		t.Errorf("Name = %q, want fan", fan.Name())
	}

	iter, err := fan.Execute(context.Background(), "input")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	defer iter.Close()

	var results []int
	for {
		val, ok, err := iter.Next(context.Background())
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		if !ok {
			break
		}
		results = append(results, val)
	}

	sort.Ints(results)
	expected := []int{1, 2, 3, 10, 20}
	if len(results) != len(expected) {
		t.Fatalf("got %d items, want %d", len(results), len(expected))
	}
	for i, v := range expected {
		if results[i] != v {
			t.Errorf("results[%d] = %d, want %d", i, results[i], v)
		}
	}
}

func TestFanOutStream_ExecuteError(t *testing.T) {
	s1 := &streamTestHelper[string, int]{
		name: "s1",
		fn: func(_ context.Context, _ string) (provider.Iterator[int], error) {
			return newSliceIter(1), nil
		},
	}
	s2 := &streamTestHelper[string, int]{
		name: "s2",
		fn: func(_ context.Context, _ string) (provider.Iterator[int], error) {
			return nil, errors.New("stream2 failed")
		},
	}

	fan := provider.FanOutStream("fan", s1, s2)
	_, err := fan.Execute(context.Background(), "input")
	if err == nil {
		t.Error("expected error when a stream fails to open")
	}
}

func TestFanOutStream_IsAvailable(t *testing.T) {
	s := &streamTestHelper[string, int]{name: "s1", fn: nil}
	fan := provider.FanOutStream("fan", s)
	if !fan.IsAvailable(context.Background()) {
		t.Error("should be available")
	}

	empty := provider.FanOutStream[string, int]("empty")
	if empty.IsAvailable(context.Background()) {
		t.Error("empty fan-out should not be available")
	}
}

func TestFanOutStream_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := &streamTestHelper[string, int]{
		name: "s1",
		fn: func(_ context.Context, _ string) (provider.Iterator[int], error) {
			return newSliceIter(1, 2, 3, 4, 5), nil
		},
	}

	fan := provider.FanOutStream("fan", s)
	iter, err := fan.Execute(ctx, "input")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	defer iter.Close()

	// Read one item then cancel.
	_, ok, err := iter.Next(ctx)
	if err != nil {
		t.Fatalf("first Next: %v", err)
	}
	if !ok {
		t.Fatal("expected first item")
	}

	cancel()
	// After cancel, Next may return error or exhausted — both are valid.
	_, _, _ = iter.Next(ctx)
}

// --- WindowedStream tests ---

func TestWindowedStream_Basic(t *testing.T) {
	inner := &streamTestHelper[string, int]{
		name: "source",
		fn: func(_ context.Context, _ string) (provider.Iterator[int], error) {
			return newSliceIter(1, 2, 3, 4, 5), nil
		},
	}

	// Window of 2: sum each batch.
	summer := &rrTestHelper[[]int, int]{
		name: "sum",
		fn: func(_ context.Context, batch []int) (int, error) {
			sum := 0
			for _, v := range batch {
				sum += v
			}
			return sum, nil
		},
	}

	windowed := provider.WindowedStream[string, int, int]("windowed", inner, 2, summer)
	if windowed.Name() != "windowed" {
		t.Errorf("Name = %q, want windowed", windowed.Name())
	}

	iter, err := windowed.Execute(context.Background(), "input")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	defer iter.Close()

	var results []int
	for {
		val, ok, err := iter.Next(context.Background())
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		if !ok {
			break
		}
		results = append(results, val)
	}

	// [1,2]=3, [3,4]=7, [5]=5
	expected := []int{3, 7, 5}
	if len(results) != len(expected) {
		t.Fatalf("got %d windows, want %d: %v", len(results), len(expected), results)
	}
	for i, v := range expected {
		if results[i] != v {
			t.Errorf("window[%d] = %d, want %d", i, results[i], v)
		}
	}
}

func TestWindowedStream_EmptyStream(t *testing.T) {
	inner := &streamTestHelper[string, int]{
		name: "empty",
		fn: func(_ context.Context, _ string) (provider.Iterator[int], error) {
			return newSliceIter[int](), nil
		},
	}

	summer := &rrTestHelper[[]int, int]{
		name: "sum",
		fn: func(_ context.Context, batch []int) (int, error) {
			return 0, nil
		},
	}

	windowed := provider.WindowedStream[string, int, int]("w", inner, 3, summer)
	iter, err := windowed.Execute(context.Background(), "x")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	defer iter.Close()

	_, ok, err := iter.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if ok {
		t.Error("expected no items from empty stream")
	}
}

func TestWindowedStream_ProcessorError(t *testing.T) {
	inner := &streamTestHelper[string, int]{
		name: "source",
		fn: func(_ context.Context, _ string) (provider.Iterator[int], error) {
			return newSliceIter(1, 2), nil
		},
	}

	failing := &rrTestHelper[[]int, int]{
		name: "fail",
		fn: func(_ context.Context, _ []int) (int, error) {
			return 0, errors.New("process failed")
		},
	}

	windowed := provider.WindowedStream[string, int, int]("w", inner, 5, failing)
	iter, err := windowed.Execute(context.Background(), "x")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	defer iter.Close()

	_, _, err = iter.Next(context.Background())
	if err == nil {
		t.Error("expected error from failing processor")
	}
}

func TestWindowedStream_IsAvailable(t *testing.T) {
	inner := &streamTestHelper[string, int]{name: "s", fn: nil}
	proc := &rrTestHelper[[]int, int]{name: "p", fn: nil}

	w := provider.WindowedStream[string, int, int]("w", inner, 2, proc)
	if !w.IsAvailable(context.Background()) {
		t.Error("should be available when both inner and processor are available")
	}
}

// --- DrainIterator tests ---

func TestDrainIterator_Normal(t *testing.T) {
	inner := newSliceIter(1, 2, 3)
	drain := provider.DrainIterator(inner, 10)

	// Read all normally.
	var results []int
	for {
		val, ok, err := drain.Next(context.Background())
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		if !ok {
			break
		}
		results = append(results, val)
	}

	if len(results) != 3 {
		t.Errorf("got %d items, want 3", len(results))
	}

	if err := drain.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestDrainIterator_DrainOnClose(t *testing.T) {
	inner := newSliceIter(1, 2, 3, 4, 5)
	drain := provider.DrainIterator(inner, 10)

	// Read 2 items.
	for i := 0; i < 2; i++ {
		_, ok, err := drain.Next(context.Background())
		if err != nil || !ok {
			t.Fatalf("item %d: ok=%v err=%v", i, ok, err)
		}
	}

	// Close — should drain remaining 3 items.
	if err := drain.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	type drainGetter interface {
		Drained() []int
	}
	drained := drain.(drainGetter).Drained()
	if len(drained) != 3 {
		t.Errorf("drained %d items, want 3", len(drained))
	}
}

func TestDrainIterator_MaxDrainLimit(t *testing.T) {
	inner := newSliceIter(1, 2, 3, 4, 5, 6, 7, 8, 9, 10)
	drain := provider.DrainIterator(inner, 3)

	// Don't read any — close immediately, drains at most 3.
	if err := drain.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	type drainGetter interface {
		Drained() []int
	}
	drained := drain.(drainGetter).Drained()
	if len(drained) != 3 {
		t.Errorf("drained %d items, want 3 (max)", len(drained))
	}
}
