package bench

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// --- Timing Middleware ---

func TestTimingMiddlewareDelegation(t *testing.T) {
	t.Parallel()

	inner := EvaluatorFunc("inner", func(ctx context.Context, input []byte) (Prediction[string], error) {
		return Prediction[string]{Label: "ok", Score: 0.9}, nil
	})

	timed := WithTiming(inner)

	if timed.Name() != "inner" {
		t.Errorf("Name() = %q, want %q", timed.Name(), "inner")
	}
	if !timed.IsAvailable(context.Background()) {
		t.Error("IsAvailable() = false, want true")
	}
}

func TestTimingMiddlewareRecordsTiming(t *testing.T) {
	t.Parallel()

	inner := EvaluatorFunc("slow", func(ctx context.Context, input []byte) (Prediction[string], error) {
		time.Sleep(5 * time.Millisecond)
		return Prediction[string]{SampleID: string(input), Label: "ok"}, nil
	})

	timed := WithTiming(inner)

	ctx := context.Background()
	_, err := timed.Execute(ctx, []byte("s1"))
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	_, err = timed.Execute(ctx, []byte("s2"))
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	timings := timed.Timings()
	if len(timings) != 2 {
		t.Fatalf("len(Timings) = %d, want 2", len(timings))
	}

	for key, dur := range timings {
		if dur < 5*time.Millisecond {
			t.Errorf("Timings[%q] = %v, want >= 5ms", key, dur)
		}
	}
}

func TestTimingMiddlewareFallbackKey(t *testing.T) {
	t.Parallel()

	// When SampleID is empty, should use input hash as key.
	inner := EvaluatorFunc("test", func(ctx context.Context, input []byte) (Prediction[string], error) {
		return Prediction[string]{Label: "ok"}, nil
	})

	timed := WithTiming(inner)
	ctx := context.Background()

	_, _ = timed.Execute(ctx, []byte("input-a"))
	_, _ = timed.Execute(ctx, []byte("input-b"))

	timings := timed.Timings()
	if len(timings) != 2 {
		t.Errorf("len(Timings) = %d, want 2 (hash-keyed)", len(timings))
	}
}

func TestTimingMiddlewareReturnsCopy(t *testing.T) {
	t.Parallel()

	inner := EvaluatorFunc("test", func(ctx context.Context, input []byte) (Prediction[string], error) {
		return Prediction[string]{SampleID: "s1"}, nil
	})

	timed := WithTiming(inner)
	ctx := context.Background()
	_, _ = timed.Execute(ctx, []byte("x"))

	t1 := timed.Timings()
	t2 := timed.Timings()

	// Mutate one copy and verify the other is unaffected.
	t1["s1"] = 999 * time.Hour
	if t2["s1"] == 999*time.Hour {
		t.Error("Timings() should return independent copies")
	}
}

// --- Caching Middleware ---

func TestCachingMiddlewareDelegation(t *testing.T) {
	t.Parallel()

	inner := EvaluatorFunc("inner", func(ctx context.Context, input []byte) (Prediction[string], error) {
		return Prediction[string]{Label: "ok"}, nil
	})

	cached := WithCaching(inner)

	if cached.Name() != "inner" {
		t.Errorf("Name() = %q, want %q", cached.Name(), "inner")
	}
	if !cached.IsAvailable(context.Background()) {
		t.Error("IsAvailable() = false, want true")
	}
}

func TestCachingMiddlewareCachesResults(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	inner := EvaluatorFunc("counter", func(ctx context.Context, input []byte) (Prediction[string], error) {
		calls.Add(1)
		return Prediction[string]{Label: string(input), Score: 0.9}, nil
	})

	cached := WithCaching(inner)
	ctx := context.Background()

	// First call: cache miss.
	p1, err := cached.Execute(ctx, []byte("hello"))
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	// Second call with same input: cache hit.
	p2, err := cached.Execute(ctx, []byte("hello"))
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if calls.Load() != 1 {
		t.Errorf("inner called %d times, want 1 (cached)", calls.Load())
	}
	if p1.Label != p2.Label {
		t.Errorf("cached result differs: %q vs %q", p1.Label, p2.Label)
	}

	hits, misses := cached.Stats()
	if hits != 1 {
		t.Errorf("hits = %d, want 1", hits)
	}
	if misses != 1 {
		t.Errorf("misses = %d, want 1", misses)
	}
}

func TestCachingMiddlewareDifferentInputs(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	inner := EvaluatorFunc("counter", func(ctx context.Context, input []byte) (Prediction[string], error) {
		calls.Add(1)
		return Prediction[string]{Label: string(input)}, nil
	})

	cached := WithCaching(inner)
	ctx := context.Background()

	_, _ = cached.Execute(ctx, []byte("a"))
	_, _ = cached.Execute(ctx, []byte("b"))

	if calls.Load() != 2 {
		t.Errorf("inner called %d times, want 2 (different inputs)", calls.Load())
	}

	hits, misses := cached.Stats()
	if hits != 0 {
		t.Errorf("hits = %d, want 0", hits)
	}
	if misses != 2 {
		t.Errorf("misses = %d, want 2", misses)
	}
}

func TestCachingMiddlewareErrorNotCached(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	inner := EvaluatorFunc("fail", func(ctx context.Context, input []byte) (Prediction[string], error) {
		calls.Add(1)
		return Prediction[string]{}, context.DeadlineExceeded
	})

	cached := WithCaching(inner)
	ctx := context.Background()

	_, err1 := cached.Execute(ctx, []byte("x"))
	if err1 == nil {
		t.Fatal("expected error on first call")
	}

	// Same input again: should re-execute because errors are not cached.
	_, err2 := cached.Execute(ctx, []byte("x"))
	if err2 == nil {
		t.Fatal("expected error on second call")
	}

	if calls.Load() != 2 {
		t.Errorf("inner called %d times, want 2 (errors not cached)", calls.Load())
	}

	hits, misses := cached.Stats()
	if hits != 0 {
		t.Errorf("hits = %d, want 0 (no successful cache)", hits)
	}
	if misses != 2 {
		t.Errorf("misses = %d, want 2 (both failed)", misses)
	}
}

func TestCachingMiddlewareMultipleHits(t *testing.T) {
	t.Parallel()

	inner := EvaluatorFunc("test", func(ctx context.Context, input []byte) (Prediction[string], error) {
		return Prediction[string]{Label: "ok"}, nil
	})

	cached := WithCaching(inner)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		_, _ = cached.Execute(ctx, []byte("same"))
	}

	hits, misses := cached.Stats()
	if hits != 4 {
		t.Errorf("hits = %d, want 4", hits)
	}
	if misses != 1 {
		t.Errorf("misses = %d, want 1", misses)
	}
}
