package worker_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kbukum/gokit/worker"
)

// ---------------------------------------------------------------------------
// 1. RoundRobin dispatch distributes across all workers
// ---------------------------------------------------------------------------

func TestRoundRobinDispatchDistribution(t *testing.T) {
	t.Parallel()

	const poolSize = 4
	var workerHits [poolSize]atomic.Int32

	h := worker.HandlerFunc[int, int](func(
		ctx context.Context, task int, emit func(worker.Event[int]),
	) error {
		// The WorkerID is set in events — emit a log and capture it
		emit(worker.LogEvent[int]("hit", nil))
		return nil
	})

	pool := worker.NewPool(h, worker.PoolConfig{
		Name:     "rr-dist",
		Size:     poolSize,
		Dispatch: worker.RoundRobin,
	})
	defer func() { _ = pool.Stop(context.Background()) }()

	// Submit exactly poolSize tasks so round-robin hits each worker once
	handles := make([]*worker.TaskHandle[int], poolSize)
	for i := range poolSize {
		var err error
		handles[i], err = pool.Submit(context.Background(), i)
		if err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
	}

	for _, h := range handles {
		for e := range h.Events() {
			if e.Type == worker.EventLog && e.WorkerID != "" {
				// Parse worker index from "rr-dist-w0" .. "rr-dist-w3"
				var idx int
				if _, err := fmt.Sscanf(e.WorkerID, "rr-dist-w%d", &idx); err == nil && idx < poolSize {
					workerHits[idx].Add(1)
				}
			}
		}
	}

	for i := range poolSize {
		if workerHits[i].Load() == 0 {
			t.Errorf("worker %d never received a task", i)
		}
	}
}

// ---------------------------------------------------------------------------
// 2. Pool with zero/negative size defaults to runtime.NumCPU
// ---------------------------------------------------------------------------

func TestPoolZeroSizeDefaults(t *testing.T) {
	t.Parallel()

	h := worker.HandlerFunc[int, int](func(
		ctx context.Context, task int, emit func(worker.Event[int]),
	) error {
		return nil
	})

	pool := worker.NewPool(h, worker.PoolConfig{
		Name: "zero-size",
		Size: 0, // should default
	})
	defer func() { _ = pool.Stop(context.Background()) }()

	stats := pool.Stats()
	if stats.Idle <= 0 {
		t.Fatalf("expected positive idle count from defaulted size, got %d", stats.Idle)
	}

	// Submit works
	handle, err := pool.Submit(context.Background(), 1)
	if err != nil {
		t.Fatalf("submit failed: %v", err)
	}
	for range handle.Events() {
	}
	if _, err := handle.Result(); err != nil {
		t.Fatalf("result failed: %v", err)
	}
}

func TestPoolNegativeSizeDefaults(t *testing.T) {
	t.Parallel()

	h := worker.HandlerFunc[int, int](func(
		ctx context.Context, task int, emit func(worker.Event[int]),
	) error {
		return nil
	})

	pool := worker.NewPool(h, worker.PoolConfig{
		Name: "neg-size",
		Size: -5, // should default
	})
	defer func() { _ = pool.Stop(context.Background()) }()

	stats := pool.Stats()
	if stats.Idle <= 0 {
		t.Fatalf("expected positive idle workers after negative config, got %d", stats.Idle)
	}
}

// ---------------------------------------------------------------------------
// 3. SubmitBatch with empty batch
// ---------------------------------------------------------------------------

func TestSubmitBatchEmpty(t *testing.T) {
	t.Parallel()

	h := worker.HandlerFunc[int, int](func(
		ctx context.Context, task int, emit func(worker.Event[int]),
	) error {
		return nil
	})

	pool := worker.NewPool(h, worker.PoolConfig{Name: "batch-empty", Size: 2})
	defer func() { _ = pool.Stop(context.Background()) }()

	handles, err := pool.SubmitBatch(context.Background(), []int{})
	if err != nil {
		t.Fatalf("empty batch should succeed: %v", err)
	}
	if len(handles) != 0 {
		t.Fatalf("expected 0 handles, got %d", len(handles))
	}
}

// ---------------------------------------------------------------------------
// 4. Task cancellation mid-execution emits error event
// ---------------------------------------------------------------------------

func TestTaskCancelMidExecution(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})

	h := worker.HandlerFunc[int, int](func(
		ctx context.Context, task int, emit func(worker.Event[int]),
	) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})

	pool := worker.NewPool(h, worker.PoolConfig{Name: "cancel-mid", Size: 1})
	defer func() { _ = pool.Stop(context.Background()) }()

	handle, err := pool.Submit(context.Background(), 1)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	<-started
	handle.Cancel()

	var gotError bool
	for e := range handle.Events() {
		if e.Type == worker.EventError {
			gotError = true
		}
	}

	_, herr := handle.Result()
	if herr == nil {
		t.Fatal("expected error from cancelled task")
	}
	if !gotError {
		t.Fatal("expected error event from cancelled task")
	}
}

// ---------------------------------------------------------------------------
// 5. Event ordering: progress → partial → result
// ---------------------------------------------------------------------------

func TestEventOrdering(t *testing.T) {
	t.Parallel()

	h := worker.HandlerFunc[int, int](func(
		ctx context.Context, task int, emit func(worker.Event[int]),
	) error {
		emit(worker.ProgressEvent[int](1, 3, "step 1"))
		emit(worker.ProgressEvent[int](2, 3, "step 2"))
		emit(worker.PartialEvent(task * 2))
		return nil
	})

	pool := worker.NewPool(h, worker.PoolConfig{Name: "order-test", Size: 1})
	defer func() { _ = pool.Stop(context.Background()) }()

	handle, err := pool.Submit(context.Background(), 5)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	var types []worker.EventType
	for e := range handle.Events() {
		types = append(types, e.Type)
	}

	// Expected: Progress, Progress, Partial, Result
	expected := []worker.EventType{
		worker.EventProgress,
		worker.EventProgress,
		worker.EventPartial,
		worker.EventResult,
	}

	if len(types) != len(expected) {
		t.Fatalf("expected %d events, got %d: %v", len(expected), len(types), types)
	}
	for i := range expected {
		if types[i] != expected[i] {
			t.Errorf("event[%d]: expected %s, got %s", i, expected[i], types[i])
		}
	}
}

// ---------------------------------------------------------------------------
// 6. Stats accuracy during concurrent submissions
// ---------------------------------------------------------------------------

func TestStatsDuringConcurrency(t *testing.T) {
	t.Parallel()

	const total = 20
	barrier := make(chan struct{})

	h := worker.HandlerFunc[int, int](func(
		ctx context.Context, task int, emit func(worker.Event[int]),
	) error {
		<-barrier
		return nil
	})

	pool := worker.NewPool(h, worker.PoolConfig{
		Name: "stats-conc",
		Size: 4,
	})
	defer func() { _ = pool.Stop(context.Background()) }()

	var handles []*worker.TaskHandle[int]
	for i := range total {
		h, err := pool.Submit(context.Background(), i)
		if err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
		handles = append(handles, h)
	}

	// Give workers time to pick up tasks
	time.Sleep(50 * time.Millisecond)

	stats := pool.Stats()
	if stats.Total != total {
		t.Errorf("expected Total=%d, got %d", total, stats.Total)
	}
	if stats.Active < 1 {
		t.Error("expected at least 1 active worker")
	}

	close(barrier)

	for _, h := range handles {
		for range h.Events() {
		}
		if _, err := h.Result(); err != nil {
			t.Errorf("task error: %v", err)
		}
	}

	finalStats := pool.Stats()
	if finalStats.Failed != 0 {
		t.Errorf("expected 0 failures, got %d", finalStats.Failed)
	}
}

// ---------------------------------------------------------------------------
// 7. Middleware chain with panicking middleware → recovery
// ---------------------------------------------------------------------------

func TestMiddlewareChainPanicRecovery(t *testing.T) {
	t.Parallel()

	inner := worker.HandlerFunc[string, string](func(
		ctx context.Context, task string, emit func(worker.Event[string]),
	) error {
		if task == "panic" {
			panic("boom")
		}
		return nil
	})

	wrapped := worker.Chain(
		worker.WithRecovery[string, string](),
	)(inner)

	pool := worker.NewPool(wrapped, worker.PoolConfig{Name: "panic-mw", Size: 1})
	defer func() { _ = pool.Stop(context.Background()) }()

	handle, err := pool.Submit(context.Background(), "panic")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	for range handle.Events() {
	}

	_, herr := handle.Result()
	if herr == nil {
		t.Fatal("expected error from panic")
	}

	// WithRecovery wraps non-error panics as *PanicError
	if _, ok := herr.(*worker.PanicError); !ok {
		t.Fatalf("expected *PanicError, got %T: %v", herr, herr)
	}
}

// ---------------------------------------------------------------------------
// 8. Supervisor persistent failure → max restarts exceeded
// ---------------------------------------------------------------------------

func TestSupervisorMaxRestartsWithFastBackoff(t *testing.T) {
	t.Parallel()

	h := worker.HandlerFunc[string, string](func(
		ctx context.Context, task string, emit func(worker.Event[string]),
	) error {
		panic("always crash")
	})

	pool := worker.NewPool(h, worker.PoolConfig{
		Name:      "sup-max",
		Size:      1,
		QueueSize: 10,
		Supervisor: &worker.SupervisorConfig{
			RestartPolicy: worker.RestartOnFailure,
			MaxRestarts:   2,
			BackoffBase:   time.Millisecond, // fast for tests
		},
	})
	defer func() { _ = pool.Stop(context.Background()) }()

	// Submit tasks that panic — after MaxRestarts the worker becomes unhealthy
	for i := range 3 {
		handle, err := pool.Submit(context.Background(), fmt.Sprintf("crash-%d", i))
		if err != nil {
			// After enough panics, submit may fail with no healthy workers
			break
		}
		for range handle.Events() {
		}
		handle.Result() //nolint:errcheck
	}

	// Next submit should fail: no healthy workers
	_, err := pool.Submit(context.Background(), "should-fail")
	if err == nil {
		t.Fatal("expected error after max restarts exceeded")
	}
}

// ---------------------------------------------------------------------------
// 9. FanOut with empty handler list
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// 10. Pool-level Events() channel aggregates from all workers
// ---------------------------------------------------------------------------

func TestPoolEventsAggregation(t *testing.T) {
	t.Parallel()

	h := worker.HandlerFunc[int, int](func(
		ctx context.Context, task int, emit func(worker.Event[int]),
	) error {
		emit(worker.LogEvent[int](fmt.Sprintf("task-%d", task), nil))
		return nil
	})

	pool := worker.NewPool(h, worker.PoolConfig{
		Name: "agg-events",
		Size: 2,
	})

	// Drain pool events in background
	var poolEvents []worker.Event[int]
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for e := range pool.Events() {
			mu.Lock()
			poolEvents = append(poolEvents, e)
			mu.Unlock()
		}
	}()

	const numTasks = 4
	handles := make([]*worker.TaskHandle[int], numTasks)
	for i := range numTasks {
		var err error
		handles[i], err = pool.Submit(context.Background(), i)
		if err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
	}

	for _, h := range handles {
		for range h.Events() {
		}
		h.Result() //nolint:errcheck
	}

	if err := pool.Stop(context.Background()); err != nil {
		t.Fatalf("stop: %v", err)
	}

	wg.Wait()

	mu.Lock()
	// Each task emits: log + result = 2 events → 4 tasks = at least 8
	if len(poolEvents) < numTasks {
		t.Errorf("expected at least %d pool-level events, got %d", numTasks, len(poolEvents))
	}
	mu.Unlock()
}

// ---------------------------------------------------------------------------
// 11. Submit after Stop returns error (verified with batch)
// ---------------------------------------------------------------------------

func TestSubmitBatchAfterStop(t *testing.T) {
	t.Parallel()

	h := worker.HandlerFunc[int, int](func(
		ctx context.Context, task int, emit func(worker.Event[int]),
	) error {
		return nil
	})

	pool := worker.NewPool(h, worker.PoolConfig{Name: "batch-after-stop", Size: 1})
	_ = pool.Stop(context.Background())

	_, err := pool.SubmitBatch(context.Background(), []int{1, 2, 3})
	if err == nil {
		t.Fatal("expected error from batch submit to stopped pool")
	}
}

// ---------------------------------------------------------------------------
// 12. Supervisor RestartAlways: worker stays healthy despite panics
// ---------------------------------------------------------------------------

func TestSupervisorRestartAlways(t *testing.T) {
	t.Parallel()

	var successCount atomic.Int32

	h := worker.HandlerFunc[int, int](func(
		ctx context.Context, task int, emit func(worker.Event[int]),
	) error {
		if task < 0 {
			panic("negative")
		}
		successCount.Add(1)
		return nil
	})

	pool := worker.NewPool(h, worker.PoolConfig{
		Name:      "restart-always",
		Size:      1,
		QueueSize: 10,
		Supervisor: &worker.SupervisorConfig{
			RestartPolicy: worker.RestartAlways,
			MaxRestarts:   0,
			BackoffBase:   time.Millisecond,
		},
	})
	defer func() { _ = pool.Stop(context.Background()) }()

	// Panic task
	handle, _ := pool.Submit(context.Background(), -1)
	for range handle.Events() {
	}
	handle.Result() //nolint:errcheck

	// Successful task should still work since policy is RestartAlways
	handle2, err := pool.Submit(context.Background(), 42)
	if err != nil {
		t.Fatalf("submit after panic with RestartAlways should work: %v", err)
	}
	for range handle2.Events() {
	}
	if _, err := handle2.Result(); err != nil {
		t.Fatalf("task should succeed: %v", err)
	}
	if successCount.Load() != 1 {
		t.Fatalf("expected 1 success, got %d", successCount.Load())
	}
}
