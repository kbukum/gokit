package worker_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/worker"
)

func TestSchedulerStartsAllJobs(t *testing.T) {
	t.Parallel()
	var a, b atomic.Int32
	s := worker.NewScheduler("test",
		worker.Job{Name: "a", Interval: 15 * time.Millisecond, Fn: func(_ context.Context) error { a.Add(1); return nil }},
		worker.Job{Name: "b", Interval: 15 * time.Millisecond, Fn: func(_ context.Context) error { b.Add(1); return nil }},
	)
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	if err := s.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if a.Load() < 1 {
		t.Errorf("job a ran %d times, want >= 1", a.Load())
	}
	if b.Load() < 1 {
		t.Errorf("job b ran %d times, want >= 1", b.Load())
	}
}

func TestSchedulerRunOnStart(t *testing.T) {
	t.Parallel()
	var count atomic.Int32
	s := worker.NewScheduler("ros-test",
		worker.Job{Name: "eager", Interval: time.Hour, RunOnStart: true, Fn: func(_ context.Context) error {
			count.Add(1)
			return nil
		}},
	)
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Give goroutine time to run the immediate execution.
	time.Sleep(30 * time.Millisecond)
	if err := s.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if count.Load() < 1 {
		t.Errorf("RunOnStart job ran %d times, want >= 1", count.Load())
	}
}

func TestSchedulerHealthAggregation(t *testing.T) {
	t.Parallel()
	s := worker.NewScheduler("health-test",
		worker.Job{Name: "ok", Interval: 10 * time.Millisecond, Fn: func(_ context.Context) error { return nil }},
		worker.Job{Name: "bad", Interval: 10 * time.Millisecond, Fn: func(_ context.Context) error { return errors.New("fail") }},
	)
	_ = s.Start(context.Background())
	time.Sleep(30 * time.Millisecond)

	h := s.Health(context.Background())
	if h.Status != component.StatusDegraded {
		t.Errorf("expected degraded, got %s", h.Status)
	}

	_ = s.Stop(context.Background())
}

func TestSchedulerDescribe(t *testing.T) {
	t.Parallel()
	s := worker.NewScheduler("my-scheduler",
		worker.Job{Name: "alpha", Interval: time.Hour, Fn: func(_ context.Context) error { return nil }},
		worker.Job{Name: "beta", Interval: time.Hour, Fn: func(_ context.Context) error { return nil }},
	)
	d := s.Describe()
	if d.Name != "my-scheduler" {
		t.Errorf("Name = %q, want my-scheduler", d.Name)
	}
	if d.Type != "scheduler" {
		t.Errorf("Type = %q, want scheduler", d.Type)
	}
	if d.Details != "2 jobs: alpha, beta" {
		t.Errorf("Details = %q", d.Details)
	}
}

func TestSchedulerStopIsIdempotent(t *testing.T) {
	t.Parallel()
	s := worker.NewScheduler("idem",
		worker.Job{Name: "j", Interval: time.Hour, Fn: func(_ context.Context) error { return nil }},
	)
	// Stop before start.
	if err := s.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	_ = s.Start(context.Background())
	_ = s.Stop(context.Background())
	// Double stop.
	if err := s.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestTickerWorkerWithRunOnStart(t *testing.T) {
	t.Parallel()
	var count atomic.Int32
	tw := worker.NewTickerWorker("ros", time.Hour, func(_ context.Context) error {
		count.Add(1)
		return nil
	}, worker.WithRunOnStart())

	_ = tw.Start(context.Background())
	time.Sleep(30 * time.Millisecond)
	_ = tw.Stop(context.Background())

	if count.Load() < 1 {
		t.Errorf("WithRunOnStart: ran %d times, want >= 1", count.Load())
	}
}

func TestTickerWorkerWithOnError(t *testing.T) {
	t.Parallel()
	var errCount atomic.Int32
	tw := worker.NewTickerWorker("onerr", 10*time.Millisecond, func(_ context.Context) error {
		return errors.New("oops")
	}, worker.WithOnError(func(_ error) {
		errCount.Add(1)
	}))

	_ = tw.Start(context.Background())
	time.Sleep(35 * time.Millisecond)
	_ = tw.Stop(context.Background())

	if errCount.Load() < 2 {
		t.Errorf("OnError called %d times, want >= 2", errCount.Load())
	}
}
