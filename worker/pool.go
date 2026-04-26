package worker

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// PoolConfig configures a worker pool.
type PoolConfig struct {
	Name        string            `yaml:"name"         mapstructure:"name"`
	Size        int               `yaml:"size"         mapstructure:"size"`         // fixed pool size (default: runtime.NumCPU)
	QueueSize   int               `yaml:"queue_size"   mapstructure:"queue_size"`   // bounded task queue (0 = unbuffered)
	EventBuffer int               `yaml:"event_buffer" mapstructure:"event_buffer"` // event channel buffer per task (default: 64)
	GracePeriod time.Duration     `yaml:"grace_period" mapstructure:"grace_period"` // shutdown grace (default: 5s)
	Dispatch    DispatchStrategy  `yaml:"dispatch"     mapstructure:"dispatch"`     // round_robin | least_loaded (default: round_robin)
	Supervisor  *SupervisorConfig `yaml:"supervisor,omitempty" mapstructure:"supervisor"`
}

func (c PoolConfig) withDefaults() PoolConfig {
	if c.Size <= 0 {
		c.Size = runtime.NumCPU()
	}
	if c.EventBuffer <= 0 {
		c.EventBuffer = 64
	}
	if c.GracePeriod <= 0 {
		c.GracePeriod = 5 * time.Second
	}
	return c
}

// PoolStats reports pool utilization.
type PoolStats struct {
	Active int `json:"active"` // workers currently executing tasks
	Idle   int `json:"idle"`   // workers waiting for tasks
	Queued int `json:"queued"` // tasks waiting in the queue
	Total  int `json:"total"`  // total tasks submitted
	Failed int `json:"failed"` // tasks that returned an error
}

// taskEnvelope wraps a task submission for internal dispatch.
type taskEnvelope[I, O any] struct {
	task   I
	handle *TaskHandle[O]
	ctx    context.Context
}

// Pool manages a fixed set of worker goroutines executing a Handler.
type Pool[I, O any] struct {
	handler  Handler[I, O]
	cfg      PoolConfig
	dispatch dispatcher

	// Per-worker input channels
	workers []chan taskEnvelope[I, O]
	stats   []workerStats

	// Aggregated event channel from all workers
	events chan Event[O]

	// Pool lifecycle
	cancel  context.CancelFunc
	poolCtx context.Context
	wg      sync.WaitGroup // tracks worker goroutines
	supWg   sync.WaitGroup // tracks supervisor goroutine

	stopped    atomic.Bool
	totalTasks atomic.Int64
	failCount  atomic.Int64

	// mu protects Stop's channel-close sequence (only used on shutdown path)
	mu         sync.Mutex
	supervisor *supervisor[I, O]
}

// NewPool creates a new worker pool with the given handler and configuration.
func NewPool[I, O any](handler Handler[I, O], cfg PoolConfig) *Pool[I, O] {
	cfg = cfg.withDefaults()
	poolCtx, cancel := context.WithCancel(context.Background()) //nolint:gosec // G118: cancel is retained on Pool and invoked in Stop()

	p := &Pool[I, O]{
		handler:  handler,
		cfg:      cfg,
		dispatch: newDispatcher(cfg.Dispatch),
		workers:  make([]chan taskEnvelope[I, O], cfg.Size),
		stats:    make([]workerStats, cfg.Size),
		events:   make(chan Event[O], cfg.EventBuffer*cfg.Size),
		cancel:   cancel,
		poolCtx:  poolCtx,
	}

	for i := range cfg.Size {
		p.workers[i] = make(chan taskEnvelope[I, O], cfg.QueueSize)
		p.wg.Add(1)
		go p.runWorker(i)
	}

	if cfg.Supervisor != nil {
		p.supervisor = newSupervisor(p, *cfg.Supervisor)
		p.supWg.Add(1)
		go func() {
			defer p.supWg.Done()
			p.supervisor.run(poolCtx)
		}()
	}

	return p
}

// Submit sends a task to the pool. Returns a handle to track the task.
func (p *Pool[I, O]) Submit(ctx context.Context, task I) (*TaskHandle[O], error) {
	if p.stopped.Load() {
		return nil, fmt.Errorf("worker: pool %q is stopped", p.cfg.Name)
	}

	// Task context is canceled if either the caller cancels or the pool shuts down.
	taskCtx, taskCancel := context.WithCancel(ctx)
	context.AfterFunc(p.poolCtx, taskCancel) //nolint:contextcheck // pool ctx is intentionally separate to allow shutdown to cancel in-flight tasks
	handle := newTaskHandle[O](taskCancel, p.cfg.EventBuffer)
	p.totalTasks.Add(1)

	// Dispatch to a worker — supervisor may reject unhealthy workers
	idx := p.selectWorker()
	if idx < 0 {
		taskCancel()
		return nil, fmt.Errorf("worker: pool %q has no healthy workers", p.cfg.Name)
	}
	env := taskEnvelope[I, O]{task: task, handle: handle, ctx: taskCtx}

	select {
	case p.workers[idx] <- env:
		return handle, nil
	case <-p.poolCtx.Done():
		taskCancel()
		return nil, fmt.Errorf("worker: pool %q shutting down", p.cfg.Name)
	}
}

// SubmitBatch sends multiple tasks. Returns handles in the same order.
func (p *Pool[I, O]) SubmitBatch(ctx context.Context, tasks []I) ([]*TaskHandle[O], error) {
	handles := make([]*TaskHandle[O], 0, len(tasks))
	for _, task := range tasks {
		h, err := p.Submit(ctx, task)
		if err != nil {
			// Cancel already-submitted tasks
			for _, prev := range handles {
				prev.Cancel()
			}
			return nil, err
		}
		handles = append(handles, h)
	}
	return handles, nil
}

// Events returns an aggregated event channel from all workers.
func (p *Pool[I, O]) Events() <-chan Event[O] {
	return p.events
}

// Stop performs graceful shutdown: stops accepting tasks, waits for in-flight
// work to finish within GracePeriod, then force-cancels remaining.
func (p *Pool[I, O]) Stop(ctx context.Context) error {
	// Use mutex to serialize Stop calls; atomic swap prevents double-close
	p.mu.Lock()
	if !p.stopped.CompareAndSwap(false, true) {
		p.mu.Unlock()
		return nil
	}

	// Close worker input channels so they drain remaining tasks
	for _, ch := range p.workers {
		close(ch)
	}
	p.mu.Unlock()

	// Wait for all workers to finish or grace period to expire
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	graceCtx, graceCancel := context.WithTimeout(ctx, p.cfg.GracePeriod)
	defer graceCancel()

	select {
	case <-done:
		// All workers finished gracefully
	case <-graceCtx.Done():
		// Force cancel remaining work
		p.cancel()
		p.wg.Wait()
	}

	// Cancel pool context to stop the supervisor goroutine, then wait for it
	// to exit before closing the events channel.
	p.cancel()
	p.supWg.Wait()

	close(p.events)
	return nil
}

// Stats returns current pool statistics.
func (p *Pool[I, O]) Stats() PoolStats {
	var active, queued int
	for i := range p.stats {
		active += int(p.stats[i].active.Load())
		queued += len(p.workers[i])
	}

	return PoolStats{
		Active: active,
		Idle:   p.cfg.Size - active,
		Queued: queued,
		Total:  int(p.totalTasks.Load()),
		Failed: int(p.failCount.Load()),
	}
}

// selectWorker picks a healthy worker using the dispatch strategy.
// If a supervisor is configured, unhealthy workers are skipped.
// Returns -1 if no healthy worker is available.
func (p *Pool[I, O]) selectWorker() int {
	// Stats use atomics — no lock or copy needed
	if p.supervisor == nil {
		return p.dispatch.next(p.stats)
	}

	// Try the dispatch-selected worker first, then scan for a healthy one
	idx := p.dispatch.next(p.stats)
	if p.supervisor.shouldAcceptTask(idx) {
		return idx
	}

	for i := range p.cfg.Size {
		candidate := (idx + i + 1) % p.cfg.Size
		if p.supervisor.shouldAcceptTask(candidate) {
			return candidate
		}
	}
	return -1 // all workers unhealthy
}

// runWorker is the goroutine loop for a single worker.
// If a supervisor is configured, panics are caught per-task, the task is failed,
// and the supervisor is notified to decide whether to keep the worker alive.
func (p *Pool[I, O]) runWorker(idx int) {
	defer p.wg.Done()

	workerID := fmt.Sprintf("%s-w%d", p.cfg.Name, idx)

	for env := range p.workers[idx] {
		p.stats[idx].active.Add(1)

		panicked := p.executeTask(workerID, idx, env)

		p.stats[idx].active.Add(-1)

		// If the task panicked and we have a supervisor, report the crash.
		// The supervisor decides whether this worker should keep running.
		if panicked && p.supervisor != nil {
			p.supervisor.reportCrash(idx, "task panic")
			// Worker goroutine continues processing next task from its channel.
			// If the supervisor wants a full restart (e.g., corrupted state),
			// it can cancel the pool context. For per-task panics the worker
			// is still healthy — it was just one bad task.
		}
	}
}

// executeTask runs a single task within a worker goroutine.
// Returns true if the handler panicked.
func (p *Pool[I, O]) executeTask(workerID string, idx int, env taskEnvelope[I, O]) (panicked bool) {
	handle := env.handle

	// Apply supervisor backoff delay if this worker has recent failures
	if p.supervisor != nil {
		if d := p.supervisor.backoff(idx); d > 0 {
			t := time.NewTimer(d)
			select {
			case <-t.C:
			case <-env.ctx.Done():
				t.Stop()
				handle.complete(*new(O), env.ctx.Err())
				return false
			}
		}
	}

	// Build emit function that tags events with worker/task IDs and forwards
	emit := func(e Event[O]) {
		e.WorkerID = workerID
		e.TaskID = handle.ID()

		// Forward to task handle
		handle.emit(e)

		// Forward to pool-level aggregated channel (non-blocking)
		select {
		case p.events <- e:
		default:
			// Pool event channel full — drop to avoid blocking worker.
			// TaskHandle channel still receives the event.
		}
	}

	// Catch panics so the worker goroutine survives and the task handle is
	// always completed — callers waiting on handle.Result() or handle.Events()
	// will never hang.
	defer func() {
		if r := recover(); r != nil {
			panicked = true
			var result O
			var err error
			switch v := r.(type) {
			case error:
				err = fmt.Errorf("worker: panic: %w", v)
			default:
				err = fmt.Errorf("worker: panic: %v", v)
			}
			p.failCount.Add(1)
			emit(errorEvent[O](err))
			handle.complete(result, err)
		}
	}()

	var result O
	err := p.handler.Handle(env.ctx, env.task, emit)

	if err != nil {
		p.failCount.Add(1)
		emit(errorEvent[O](err))
	} else {
		emit(resultEvent(result))
	}

	handle.complete(result, err)
	return false
}
