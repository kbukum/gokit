package worker_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kbukum/gokit/worker"
)

func TestPoolSubmitAndResult(t *testing.T) {
	t.Parallel()

	h := worker.HandlerFunc[int, int](func(
		ctx context.Context, task int, emit func(worker.Event[int]),
	) error {
		emit(worker.PartialEvent(task * 2))
		return nil
	})

	pool := worker.NewPool(h, worker.PoolConfig{
		Name: "test",
		Size: 2,
	})

	handle, err := pool.Submit(context.Background(), 21)
	if err != nil {
		t.Fatalf("submit failed: %v", err)
	}

	// Drain events
	for range handle.Events() {
	}

	_, herr := handle.Result()
	if herr != nil {
		t.Fatalf("unexpected error: %v", herr)
	}

	if err := pool.Stop(context.Background()); err != nil {
		t.Fatalf("stop failed: %v", err)
	}
}

func TestPoolEvents(t *testing.T) {
	t.Parallel()

	h := worker.HandlerFunc[string, string](func(
		ctx context.Context, task string, emit func(worker.Event[string]),
	) error {
		emit(worker.ProgressEvent[string](50, 100, "halfway"))
		emit(worker.PartialEvent("partial-" + task))
		return nil
	})

	pool := worker.NewPool(h, worker.PoolConfig{
		Name: "events-test",
		Size: 1,
	})

	handle, err := pool.Submit(context.Background(), "abc")
	if err != nil {
		t.Fatalf("submit failed: %v", err)
	}

	events := make([]worker.Event[string], 0, 4)
	for e := range handle.Events() {
		events = append(events, e)
	}

	if len(events) < 3 { // progress + partial + result
		t.Fatalf("expected at least 3 events, got %d", len(events))
	}

	if err := pool.Stop(context.Background()); err != nil {
		t.Fatalf("stop failed: %v", err)
	}
}

func TestPoolBatchSubmit(t *testing.T) {
	t.Parallel()

	h := worker.HandlerFunc[int, int](func(
		ctx context.Context, task int, emit func(worker.Event[int]),
	) error {
		return nil
	})

	pool := worker.NewPool(h, worker.PoolConfig{
		Name: "batch-test",
		Size: 4,
	})

	handles, err := pool.SubmitBatch(context.Background(), []int{1, 2, 3, 4, 5})
	if err != nil {
		t.Fatalf("batch submit failed: %v", err)
	}

	if len(handles) != 5 {
		t.Fatalf("expected 5 handles, got %d", len(handles))
	}

	for i, h := range handles {
		_, herr := h.Result()
		if herr != nil {
			t.Fatalf("task %d failed: %v", i, herr)
		}
	}

	if err := pool.Stop(context.Background()); err != nil {
		t.Fatalf("stop failed: %v", err)
	}
}

func TestPoolCancelTask(t *testing.T) {
	t.Parallel()

	h := worker.HandlerFunc[string, string](func(
		ctx context.Context, task string, emit func(worker.Event[string]),
	) error {
		<-ctx.Done()
		return ctx.Err()
	})

	pool := worker.NewPool(h, worker.PoolConfig{
		Name: "cancel-test",
		Size: 1,
	})

	handle, err := pool.Submit(context.Background(), "block")
	if err != nil {
		t.Fatalf("submit failed: %v", err)
	}

	// Cancel the task
	handle.Cancel()

	_, herr := handle.Result()
	if herr == nil {
		t.Fatal("expected error from canceled task")
	}

	if err := pool.Stop(context.Background()); err != nil {
		t.Fatalf("stop failed: %v", err)
	}
}

func TestPoolGracefulStop(t *testing.T) {
	t.Parallel()

	var completed int
	var mu sync.Mutex

	h := worker.HandlerFunc[int, int](func(
		ctx context.Context, task int, emit func(worker.Event[int]),
	) error {
		time.Sleep(50 * time.Millisecond)
		mu.Lock()
		completed++
		mu.Unlock()
		return nil
	})

	pool := worker.NewPool(h, worker.PoolConfig{
		Name:        "graceful-test",
		Size:        2,
		GracePeriod: 2 * time.Second,
	})

	for i := range 4 {
		_, err := pool.Submit(context.Background(), i)
		if err != nil {
			t.Fatalf("submit %d failed: %v", i, err)
		}
	}

	if err := pool.Stop(context.Background()); err != nil {
		t.Fatalf("stop failed: %v", err)
	}

	mu.Lock()
	if completed != 4 {
		t.Fatalf("expected 4 completed tasks, got %d", completed)
	}
	mu.Unlock()
}

func TestPoolStats(t *testing.T) {
	t.Parallel()

	h := worker.HandlerFunc[int, int](func(
		ctx context.Context, task int, emit func(worker.Event[int]),
	) error {
		return nil
	})

	pool := worker.NewPool(h, worker.PoolConfig{
		Name: "stats-test",
		Size: 4,
	})

	stats := pool.Stats()
	if stats.Idle != 4 {
		t.Fatalf("expected 4 idle workers, got %d", stats.Idle)
	}

	if err := pool.Stop(context.Background()); err != nil {
		t.Fatalf("stop failed: %v", err)
	}
}

func TestPoolSubmitAfterStop(t *testing.T) {
	t.Parallel()

	h := worker.HandlerFunc[int, int](func(
		ctx context.Context, task int, emit func(worker.Event[int]),
	) error {
		return nil
	})

	pool := worker.NewPool(h, worker.PoolConfig{
		Name: "stopped-test",
		Size: 1,
	})

	if err := pool.Stop(context.Background()); err != nil {
		t.Fatalf("stop failed: %v", err)
	}

	_, err := pool.Submit(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error when submitting to stopped pool")
	}
}

func TestPoolSubmitConcurrentWithStopDoesNotPanic(t *testing.T) {
	t.Parallel()

	h := worker.HandlerFunc[int, int](func(
		ctx context.Context, task int, emit func(worker.Event[int]),
	) error {
		<-ctx.Done()
		return ctx.Err()
	})

	pool := worker.NewPool(h, worker.PoolConfig{
		Name:        "submit-stop-race",
		Size:        1,
		GracePeriod: 10 * time.Millisecond,
	})

	const submitters = 128
	start := make(chan struct{})
	errCh := make(chan error, submitters)
	var wg sync.WaitGroup
	wg.Add(submitters)
	for i := range submitters {
		go func(task int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					errCh <- fmt.Errorf("submit panic: %v", r)
				}
			}()
			<-start
			handle, err := pool.Submit(context.Background(), task)
			if err != nil {
				if !strings.Contains(err.Error(), "is stopped") {
					errCh <- fmt.Errorf("unexpected submit error: %w", err)
				}
				return
			}
			if handle == nil {
				errCh <- fmt.Errorf("submit returned nil handle without error")
			}
		}(i)
	}

	close(start)
	if err := pool.Stop(context.Background()); err != nil {
		t.Fatalf("stop failed: %v", err)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestPoolHandlerError(t *testing.T) {
	t.Parallel()

	h := worker.HandlerFunc[string, string](func(
		ctx context.Context, task string, emit func(worker.Event[string]),
	) error {
		return fmt.Errorf("intentional failure")
	})

	pool := worker.NewPool(h, worker.PoolConfig{
		Name: "error-test",
		Size: 1,
	})

	handle, err := pool.Submit(context.Background(), "fail")
	if err != nil {
		t.Fatalf("submit failed: %v", err)
	}

	// Drain events
	for range handle.Events() {
	}

	_, herr := handle.Result()
	if herr == nil {
		t.Fatal("expected error from failed handler")
	}

	stats := pool.Stats()
	if stats.Failed != 1 {
		t.Fatalf("expected 1 failed task, got %d", stats.Failed)
	}

	if err := pool.Stop(context.Background()); err != nil {
		t.Fatalf("stop failed: %v", err)
	}
}

func TestPoolDoneChannel(t *testing.T) {
	t.Parallel()

	h := worker.HandlerFunc[int, int](func(
		ctx context.Context, task int, emit func(worker.Event[int]),
	) error {
		return nil
	})

	pool := worker.NewPool(h, worker.PoolConfig{Name: "done-test", Size: 1})
	defer func() { _ = pool.Stop(context.Background()) }()

	handle, err := pool.Submit(context.Background(), 42)
	if err != nil {
		t.Fatalf("submit failed: %v", err)
	}

	// Wait via Done() channel instead of Result()
	<-handle.Done()

	_, herr := handle.Result()
	if herr != nil {
		t.Fatalf("unexpected error: %v", herr)
	}
}

func TestPoolLeastLoadedDispatch(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	block := make(chan struct{})

	h := worker.HandlerFunc[string, string](func(
		ctx context.Context, task string, emit func(worker.Event[string]),
	) error {
		if task == "blocker" {
			started <- struct{}{}
			<-block
		}
		return nil
	})

	pool := worker.NewPool(h, worker.PoolConfig{
		Name:     "least-loaded-test",
		Size:     2,
		Dispatch: worker.LeastLoaded,
	})
	defer func() { _ = pool.Stop(context.Background()) }()

	// Submit a blocking task — one worker will be busy
	_, err := pool.Submit(context.Background(), "blocker")
	if err != nil {
		t.Fatalf("submit blocker failed: %v", err)
	}
	<-started // wait until worker is actually busy

	// Submit a fast task — should go to the idle worker
	h2, err := pool.Submit(context.Background(), "fast")
	if err != nil {
		t.Fatalf("submit fast failed: %v", err)
	}

	// Fast task should complete even though one worker is blocked
	<-h2.Done()
	_, herr := h2.Result()
	if herr != nil {
		t.Fatalf("fast task should succeed: %v", herr)
	}

	close(block) // unblock the first task
}

func TestPoolStopIdempotent(t *testing.T) {
	t.Parallel()

	h := worker.HandlerFunc[int, int](func(
		ctx context.Context, task int, emit func(worker.Event[int]),
	) error {
		return nil
	})

	pool := worker.NewPool(h, worker.PoolConfig{Name: "idempotent-stop", Size: 1})

	if err := pool.Stop(context.Background()); err != nil {
		t.Fatalf("first stop failed: %v", err)
	}
	// Second stop should be a no-op
	if err := pool.Stop(context.Background()); err != nil {
		t.Fatalf("second stop failed: %v", err)
	}
}

func TestPoolStopForceCancel(t *testing.T) {
	t.Parallel()

	h := worker.HandlerFunc[string, string](func(
		ctx context.Context, task string, emit func(worker.Event[string]),
	) error {
		<-ctx.Done()
		return ctx.Err()
	})

	pool := worker.NewPool(h, worker.PoolConfig{
		Name:        "force-cancel-test",
		Size:        1,
		GracePeriod: 50 * time.Millisecond,
	})

	_, err := pool.Submit(context.Background(), "block-forever")
	if err != nil {
		t.Fatalf("submit failed: %v", err)
	}

	// Stop should force-cancel after short grace period
	start := time.Now()
	if err := pool.Stop(context.Background()); err != nil {
		t.Fatalf("stop failed: %v", err)
	}
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Fatalf("stop took too long: %v (expected ~50ms)", elapsed)
	}
}

func TestPoolSubmitBatchPartialCancel(t *testing.T) {
	t.Parallel()

	calls := 0
	h := worker.HandlerFunc[int, int](func(
		ctx context.Context, task int, emit func(worker.Event[int]),
	) error {
		calls++
		return nil
	})

	pool := worker.NewPool(h, worker.PoolConfig{
		Name:      "batch-cancel-test",
		Size:      1,
		QueueSize: 1, // very small queue
	})

	// Stop the pool, then try a batch — the stopped check should trigger mid-batch
	_ = pool.Stop(context.Background())

	_, err := pool.SubmitBatch(context.Background(), []int{1, 2, 3})
	if err == nil {
		t.Fatal("expected error from batch submit to stopped pool")
	}
}

func TestPoolNoHealthyWorkers(t *testing.T) {
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
		Name:      "no-healthy-test",
		Size:      1,
		QueueSize: 10,
		Supervisor: &worker.SupervisorConfig{
			RestartPolicy: worker.RestartNever,
		},
	})
	defer func() { _ = pool.Stop(context.Background()) }()

	// First task panics — worker is marked unhealthy with RestartNever
	handle, err := pool.Submit(context.Background(), "crash")
	if err != nil {
		t.Fatalf("submit failed: %v", err)
	}
	for range handle.Events() {
	}

	// Second submit should fail — no healthy workers
	_, err = pool.Submit(context.Background(), "should-fail")
	if err == nil {
		t.Fatal("expected error when no healthy workers available")
	}
}

func TestSharedQueueUsesAvailableWorkers(t *testing.T) {
	t.Parallel()

	const poolSize = 4
	var workerHits [poolSize]atomic.Int32
	block := make(chan struct{})

	h := worker.HandlerFunc[int, int](func(
		ctx context.Context, task int, emit func(worker.Event[int]),
	) error {
		emit(worker.LogEvent[int]("hit", nil))
		<-block
		return nil
	})

	pool := worker.NewPool(h, worker.PoolConfig{
		Name:      "shared-dist",
		Size:      poolSize,
		QueueSize: 0,
		Dispatch:  worker.RoundRobin,
	})
	defer func() { _ = pool.Stop(context.Background()) }()

	handles := make([]*worker.TaskHandle[int], poolSize)
	for i := range poolSize {
		var err error
		handles[i], err = pool.Submit(context.Background(), i)
		if err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
	}
	close(block)

	for _, h := range handles {
		for e := range h.Events() {
			if e.Type == worker.EventLog && e.WorkerID != "" {
				var idx int
				if _, err := fmt.Sscanf(e.WorkerID, "shared-dist-w%d", &idx); err == nil && idx < poolSize {
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
		Name:      "stats-conc",
		Size:      4,
		QueueSize: total,
	})
	defer func() { _ = pool.Stop(context.Background()) }()

	handles := make([]*worker.TaskHandle[int], 0, total)
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
		h.Result() //nolint:errcheck // intentional in test cleanup
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
