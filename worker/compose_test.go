package worker_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/kbukum/gokit/worker"
)

func TestFanOut(t *testing.T) {
	t.Parallel()

	h1 := worker.HandlerFunc[string, string](func(ctx context.Context, task string, emit func(worker.Event[string])) error {
		emit(worker.PartialEvent("h1-" + task))
		return nil
	})

	h2 := worker.HandlerFunc[string, string](func(ctx context.Context, task string, emit func(worker.Event[string])) error {
		emit(worker.PartialEvent("h2-" + task))
		return nil
	})

	fan := worker.FanOut("test-fan", h1, h2)

	var events []worker.Event[[]string]
	emit := func(e worker.Event[[]string]) { events = append(events, e) }

	err := fan.Handle(context.Background(), "input", emit)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// FanOut should emit a result event containing collected results
	var resultEvt *worker.Event[[]string]
	for i, e := range events {
		if e.Type == worker.EventResult {
			resultEvt = &events[i]
			break
		}
	}
	if resultEvt == nil {
		t.Fatal("expected a result event from FanOut")
	}
	if len(resultEvt.Data) != 2 {
		t.Fatalf("expected 2 results, got %d", len(resultEvt.Data))
	}
}

func TestMapReduce(t *testing.T) {
	t.Parallel()

	// Split int into parts, multiply each, sum results
	mr := worker.NewMapReduce(worker.MapReduceConfig[int, int, int]{
		Name:  "test-mr",
		Split: func(n int) []int { return []int{n, n * 2, n * 3} },
		Handler: worker.HandlerFunc[int, int](func(
			ctx context.Context, task int, emit func(worker.Event[int]),
		) error {
			return nil
		}),
		Combine: func(results []int) (int, error) {
			sum := 0
			for _, r := range results {
				sum += r
			}
			return sum, nil
		},
	})

	var events []worker.Event[int]
	emit := func(e worker.Event[int]) { events = append(events, e) }

	err := mr.Handle(context.Background(), 10, emit)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFanOutError(t *testing.T) {
	t.Parallel()

	good := worker.HandlerFunc[string, string](func(ctx context.Context, task string, emit func(worker.Event[string])) error {
		return nil
	})

	bad := worker.HandlerFunc[string, string](func(ctx context.Context, task string, emit func(worker.Event[string])) error {
		return fmt.Errorf("handler failed")
	})

	fan := worker.FanOut("error-fan", good, bad)
	err := fan.Handle(context.Background(), "input", func(worker.Event[[]string]) {})
	if err == nil {
		t.Fatal("expected error from FanOut when one handler fails")
	}
}

func TestPipeline(t *testing.T) {
	t.Parallel()

	// Stage 1: string → string (uppercase simulation via append)
	stage1 := worker.PipelineStage{
		Name: "prefix",
		Handler: worker.HandlerFunc[any, any](func(ctx context.Context, task any, emit func(worker.Event[any])) error {
			s := task.(string)
			emit(worker.PartialEvent[any]("processing: " + s))
			result := "PREFIXED-" + s
			emit(resultEvent(result))
			return nil
		}),
	}

	// Stage 2: string → string (suffix)
	stage2 := worker.PipelineStage{
		Name: "suffix",
		Handler: worker.HandlerFunc[any, any](func(ctx context.Context, task any, emit func(worker.Event[any])) error {
			s := task.(string)
			result := s + "-SUFFIXED"
			emit(resultEvent(result))
			return nil
		}),
	}

	pipe := worker.NewPipeline[string, string]("test-pipe", stage1, stage2)

	var events []worker.Event[string]
	emit := func(e worker.Event[string]) { events = append(events, e) }

	err := pipe.Handle(context.Background(), "hello", emit)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Find the final result event
	var found bool
	for _, e := range events {
		if e.Type == worker.EventResult && e.Data == "PREFIXED-hello-SUFFIXED" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected final result 'PREFIXED-hello-SUFFIXED', events: %+v", events)
	}
}

func TestPipelineError(t *testing.T) {
	t.Parallel()

	failing := worker.PipelineStage{
		Name: "fail",
		Handler: worker.HandlerFunc[any, any](func(ctx context.Context, task any, emit func(worker.Event[any])) error {
			return fmt.Errorf("stage failed")
		}),
	}

	pipe := worker.NewPipeline[string, string]("fail-pipe", failing)
	err := pipe.Handle(context.Background(), "input", func(worker.Event[string]) {})
	if err == nil {
		t.Fatal("expected error from pipeline with failing stage")
	}
}

// resultEvent is a test helper that creates a result event.
func resultEvent(data any) worker.Event[any] {
	return worker.Event[any]{
		Type: worker.EventResult,
		Data: data,
	}
}

func TestFanOutEmptyHandlers(t *testing.T) {
	t.Parallel()

	fanout := worker.FanOut[string, string]("empty-fanout")

	pool := worker.NewPool(fanout, worker.PoolConfig{Name: "fanout-empty", Size: 1})
	defer func() { _ = pool.Stop(context.Background()) }()

	handle, err := pool.Submit(context.Background(), "input")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	for range handle.Events() {
	}

	_, herr := handle.Result()
	if herr != nil {
		t.Fatalf("empty fanout should succeed: %v", herr)
	}
}
