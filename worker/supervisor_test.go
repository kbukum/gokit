package worker_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kbukum/gokit/worker"
)

func TestSupervisorRecoversPanicAndContinues(t *testing.T) {
	t.Parallel()

	var callCount atomic.Int32

	h := worker.HandlerFunc[string, string](func(
		ctx context.Context, task string, emit func(worker.Event[string]),
	) error {
		n := callCount.Add(1)
		if n == 1 {
			panic("worker crash")
		}
		emit(worker.PartialEvent("recovered"))
		return nil
	})

	pool := worker.NewPool(h, worker.PoolConfig{
		Name:      "supervisor-test",
		Size:      1,
		QueueSize: 10,
		Supervisor: &worker.SupervisorConfig{
			RestartPolicy: worker.RestartOnFailure,
			MaxRestarts:   3,
			BackoffBase:   10 * time.Millisecond,
		},
	})
	defer pool.Stop(context.Background()) //nolint:errcheck // test cleanup

	// First task panics — should be caught, task completed with error
	handle1, err := pool.Submit(context.Background(), "crash-me")
	if err != nil {
		t.Fatalf("submit 1 failed: %v", err)
	}

	// Drain events — channel should close because panic is caught per-task
	for range handle1.Events() {
	}

	_, herr := handle1.Result()
	if herr == nil {
		t.Fatal("expected error from panicked task")
	}

	// Second task succeeds — same worker goroutine survived
	handle2, err := pool.Submit(context.Background(), "work-now")
	if err != nil {
		t.Fatalf("submit 2 failed: %v", err)
	}

	var foundPartial bool
	for e := range handle2.Events() {
		if e.Type == worker.EventPartial && e.Data == "recovered" {
			foundPartial = true
		}
	}

	_, herr = handle2.Result()
	if herr != nil {
		t.Fatalf("task 2 should have succeeded: %v", herr)
	}
	if !foundPartial {
		t.Fatal("expected partial event 'recovered' from surviving worker")
	}
}

func TestSupervisorTracksPanicCount(t *testing.T) {
	t.Parallel()

	var panicCount atomic.Int32

	h := worker.HandlerFunc[string, string](func(
		ctx context.Context, task string, emit func(worker.Event[string]),
	) error {
		if task == "crash" {
			panicCount.Add(1)
			panic("intentional crash")
		}
		return nil
	})

	pool := worker.NewPool(h, worker.PoolConfig{
		Name:      "panic-count-test",
		Size:      1,
		QueueSize: 10,
		Supervisor: &worker.SupervisorConfig{
			RestartPolicy: worker.RestartOnFailure,
			MaxRestarts:   5,
			BackoffBase:   10 * time.Millisecond,
		},
	})
	defer pool.Stop(context.Background()) //nolint:errcheck // test cleanup

	// Submit 3 crashing tasks
	for range 3 {
		h, submitErr := pool.Submit(context.Background(), "crash")
		if submitErr != nil {
			t.Fatalf("submit failed: %v", submitErr)
		}
		for range h.Events() {
		}
	}

	// Submit a normal task — worker should still be alive
	handle, err := pool.Submit(context.Background(), "ok")
	if err != nil {
		t.Fatalf("submit failed: %v", err)
	}
	for range handle.Events() {
	}
	_, herr := handle.Result()
	if herr != nil {
		t.Fatalf("expected success after panics: %v", herr)
	}

	if count := panicCount.Load(); count != 3 {
		t.Fatalf("expected 3 panics, got %d", count)
	}
}

func TestSupervisorNeverRestartStillCompletesTasks(t *testing.T) {
	t.Parallel()

	h := worker.HandlerFunc[string, string](func(
		ctx context.Context, task string, emit func(worker.Event[string]),
	) error {
		if task == "crash" {
			panic("crash")
		}
		emit(worker.PartialEvent("ok"))
		return nil
	})

	pool := worker.NewPool(h, worker.PoolConfig{
		Name:      "never-restart-test",
		Size:      1,
		QueueSize: 10,
		Supervisor: &worker.SupervisorConfig{
			RestartPolicy: worker.RestartNever,
		},
	})
	defer pool.Stop(context.Background()) //nolint:errcheck // test cleanup

	// Panicking task — should still complete with error (not hang)
	handle, err := pool.Submit(context.Background(), "crash")
	if err != nil {
		t.Fatalf("submit failed: %v", err)
	}
	for range handle.Events() {
	}
	_, herr := handle.Result()
	if herr == nil {
		t.Fatal("expected error from panicked task")
	}
}

func TestSupervisorHealthCheckRuns(t *testing.T) {
	t.Parallel()

	h := worker.HandlerFunc[string, string](func(
		ctx context.Context, task string, emit func(worker.Event[string]),
	) error {
		return nil
	})

	pool := worker.NewPool(h, worker.PoolConfig{
		Name: "health-test",
		Size: 2,
		Supervisor: &worker.SupervisorConfig{
			RestartPolicy:  worker.RestartOnFailure,
			HealthInterval: 50 * time.Millisecond,
		},
	})

	handle, err := pool.Submit(context.Background(), "ok")
	if err != nil {
		t.Fatalf("submit failed: %v", err)
	}
	for range handle.Events() {
	}
	_, herr := handle.Result()
	if herr != nil {
		t.Fatalf("unexpected error: %v", herr)
	}

	pool.Stop(context.Background()) //nolint:errcheck // test cleanup
}

func TestSupervisorMaxRestartsExceeded(t *testing.T) {
	t.Parallel()

	h := worker.HandlerFunc[string, string](func(
		ctx context.Context, task string, emit func(worker.Event[string]),
	) error {
		panic("always crash")
	})

	pool := worker.NewPool(h, worker.PoolConfig{
		Name:      "max-restart-test",
		Size:      1,
		QueueSize: 10,
		Supervisor: &worker.SupervisorConfig{
			RestartPolicy: worker.RestartOnFailure,
			MaxRestarts:   2,
			BackoffBase:   5 * time.Millisecond,
		},
	})
	defer pool.Stop(context.Background()) //nolint:errcheck // test cleanup

	// Crash twice to exceed MaxRestarts
	for range 2 {
		h, err := pool.Submit(context.Background(), "crash")
		if err != nil {
			t.Fatalf("submit failed: %v", err)
		}
		for range h.Events() {
		}
	}

	// Worker should be marked unhealthy — next submit should fail
	_, err := pool.Submit(context.Background(), "should-fail")
	if err == nil {
		t.Fatal("expected error when worker exceeded max restarts")
	}
}

func TestSupervisorHealthCheckWithDeadWorkers(t *testing.T) {
	t.Parallel()

	h := worker.HandlerFunc[string, string](func(
		ctx context.Context, task string, emit func(worker.Event[string]),
	) error {
		if task == "crash" {
			panic("crash")
		}
		return nil
	})

	pool := worker.NewPool(h, worker.PoolConfig{
		Name:      "health-dead-test",
		Size:      1,
		QueueSize: 10,
		Supervisor: &worker.SupervisorConfig{
			RestartPolicy:  worker.RestartOnFailure,
			MaxRestarts:    1,
			HealthInterval: 30 * time.Millisecond,
		},
	})
	defer pool.Stop(context.Background()) //nolint:errcheck // test cleanup

	// Crash to mark worker unhealthy
	handle, err := pool.Submit(context.Background(), "crash")
	if err != nil {
		t.Fatalf("submit failed: %v", err)
	}
	for range handle.Events() {
	}

	// Let health check run
	time.Sleep(80 * time.Millisecond)

	// Drain pool events to see health check log (just verify no panic)
	for {
		select {
		case _, ok := <-pool.Events():
			if !ok {
				return
			}
		default:
			return
		}
	}
}
