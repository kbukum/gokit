package bench_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kbukum/gokit/bench"
	"github.com/kbukum/gokit/bench/metric"
	"github.com/kbukum/gokit/util"
)

func TestEvaluationPersistence(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := bench.NewResultStore(bench.NewDirStore(t.TempDir()))
	runner := bench.NewBenchRunner(
		bench.WithStore[string](store),
		bench.WithClock[string](util.NewFakeClock(time.Unix(0, 0))),
		bench.WithMetrics(metric.ExactMatch[string]()),
		bench.WithRepeats[string](2),
		bench.WithWarmup[string](1),
	)
	runner.Register("echo", bench.EvaluatorFunc("echo", func(_ context.Context, input []byte) (bench.Prediction[string], error) {
		return bench.Prediction[string]{Label: string(input)}, nil
	}))
	dataset := bench.NewSliceDataset(bench.DatasetDescriptor{Name: "fixture", Version: "1"}, []bench.Sample[string]{
		{ID: "a", Input: []byte("yes"), Label: "yes"},
		{ID: "b", Input: []byte("no"), Label: "yes"},
	})
	result, err := runner.Run(ctx, dataset)
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics[0].Value != .5 || result.Branches["echo"].Performance.Observations != 4 {
		t.Fatalf("unexpected result: %+v", result)
	}
	loaded, err := store.Load(ctx, result.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.EvaluationFingerprint != result.EvaluationFingerprint {
		t.Fatal("identity changed on reload")
	}
	iter, err := bench.Observations[string](ctx, store, result.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := iter.Close(); err != nil {
			t.Error(err)
		}
	}()
	count := 0
	for {
		_, ok, err := iter.Next(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			break
		}
		count++
	}
	if count != 4 {
		t.Fatalf("records = %d", count)
	}
}

func TestEvaluationCancellation(t *testing.T) {
	t.Parallel()
	for _, phase := range []string{"slot", "execution", "warmup", "repeats"} {
		t.Run(phase, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			objects := bench.NewDirStore(t.TempDir())
			store := bench.NewResultStore(objects)
			opts := []bench.RunOption[string]{bench.WithStore[string](store)}
			if phase == "warmup" {
				opts = append(opts, bench.WithWarmup[string](1))
			}
			if phase == "repeats" {
				opts = append(opts, bench.WithRepeats[string](3))
			}
			runner := bench.NewBenchRunner(opts...)
			started := make(chan struct{})
			var calls, active atomic.Int32
			runner.Register("cancel", bench.EvaluatorFunc("cancel", func(ctx context.Context, _ []byte) (bench.Prediction[string], error) {
				call := calls.Add(1)
				active.Add(1)
				defer active.Add(-1)
				if phase == "repeats" && call == 1 {
					return bench.Prediction[string]{}, nil
				}
				close(started)
				<-ctx.Done()
				return bench.Prediction[string]{}, ctx.Err()
			}))
			stopped := make(chan struct{})
			go func() {
				defer close(stopped)
				select {
				case <-started:
					cancel()
				case <-ctx.Done():
				}
			}()
			start := time.Now()
			result, err := runner.Run(ctx, bench.NewSliceDataset(bench.DatasetDescriptor{Name: "cancel", Version: "1"}, []bench.Sample[string]{{ID: "a"}, {ID: "b"}, {ID: "c"}}))
			<-stopped
			var runErr *bench.RunError
			if result != nil || !errors.As(err, &runErr) || runErr.Outcome != bench.OutcomeCancelled {
				t.Fatalf("result=%v err=%v", result, err)
			}
			keys, err := objects.List(context.Background(), "")
			if err != nil || len(keys) != 0 {
				t.Fatalf("cleanup: %v %v", keys, err)
			}
			if active.Load() != 0 {
				t.Fatal("admitted evaluator leaked after cancellation")
			}
			if time.Since(start) >= time.Second {
				t.Fatal("cancellation exceeded one-second teardown bound")
			}
			expected := int32(1)
			if phase == "repeats" {
				expected = 2
			}
			if calls.Load() != expected {
				t.Fatalf("admitted %d evaluations, want %d", calls.Load(), expected)
			}
		})
	}
}
