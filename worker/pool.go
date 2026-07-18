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
	Overflow    OverflowPolicy    `yaml:"overflow"     mapstructure:"overflow"`     // block | reject | drop_oldest (default: block)
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
	if c.Overflow == "" {
		c.Overflow = OverflowBlock
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

	// Shared queue is the pool-wide bounded backlog.
	// Affinity channels are size-1 supervisor steering hints for healthy workers.
	queue      chan taskEnvelope[I, O]
	affinities []chan taskEnvelope[I, O]
	stats      []workerStats

	// Aggregated event channel from all workers
	events chan Event[O]

	// Pool lifecycle
	acceptCancel context.CancelFunc
	acceptCtx    context.Context
	cancel       context.CancelFunc
	poolCtx      context.Context
	wg           sync.WaitGroup // tracks worker goroutines
	supWg        sync.WaitGroup // tracks supervisor goroutine
	taskWg       sync.WaitGroup // tracks accepted tasks until completion or cancellation

	stopped    atomic.Bool
	totalTasks atomic.Int64
	failCount  atomic.Int64

	// mu serializes Stop with task acceptance so taskWg cannot race with shutdown waits.
	mu         sync.Mutex
	supervisor *supervisor[I, O]
}

// NewPool creates a new worker pool with the given handler and configuration.
func NewPool[I, O any](handler Handler[I, O], cfg PoolConfig) *Pool[I, O] {
	cfg = cfg.withDefaults()
	acceptCtx, acceptCancel := context.WithCancel(context.Background()) //nolint:gosec // G118: cancel is retained on Pool and invoked in Stop()
	poolCtx, cancel := context.WithCancel(context.Background())         //nolint:gosec // G118: cancel is retained on Pool and invoked in Stop()

	p := &Pool[I, O]{
		handler:      handler,
		cfg:          cfg,
		dispatch:     newDispatcher(cfg.Dispatch),
		queue:        make(chan taskEnvelope[I, O], cfg.QueueSize),
		affinities:   make([]chan taskEnvelope[I, O], cfg.Size),
		stats:        make([]workerStats, cfg.Size),
		events:       make(chan Event[O], cfg.EventBuffer*cfg.Size),
		acceptCancel: acceptCancel,
		acceptCtx:    acceptCtx,
		cancel:       cancel,
		poolCtx:      poolCtx,
	}

	for i := range cfg.Size {
		p.affinities[i] = make(chan taskEnvelope[I, O], 1)
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
		return nil, p.stoppedError()
	}

	// Task context is canceled if either the caller cancels or the pool shuts down.
	taskCtx, taskCancel := context.WithCancel(ctx)
	context.AfterFunc(p.poolCtx, taskCancel) //nolint:contextcheck // pool ctx is intentionally separate to allow shutdown to cancel in-flight tasks
	handle := newTaskHandle[O](taskCancel, p.cfg.EventBuffer)
	env := taskEnvelope[I, O]{task: task, handle: handle, ctx: taskCtx}

	p.mu.Lock()
	if p.stopped.Load() {
		p.mu.Unlock()
		taskCancel()
		return nil, p.stoppedError()
	}
	p.taskWg.Add(1)
	p.totalTasks.Add(1)
	p.mu.Unlock()

	var (
		submitted *TaskHandle[O]
		err       error
	)
	if p.supervisor == nil {
		submitted, err = p.enqueue(ctx, env)
	} else {
		idx := p.pickWorkerForRouting()
		if idx < 0 {
			taskCancel()
			p.taskWg.Done()
			return nil, fmt.Errorf("worker: pool %q has no healthy workers", p.cfg.Name)
		}
		submitted, err = p.enqueueAffinity(ctx, idx, env)
	}
	if err != nil {
		p.taskWg.Done()
	}
	return submitted, err
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

// Stop performs graceful shutdown: stops accepting tasks,
// waits for in-flight work to finish within GracePeriod, then force-cancels remaining.
func (p *Pool[I, O]) Stop(ctx context.Context) error {
	p.mu.Lock()
	if !p.stopped.CompareAndSwap(false, true) {
		p.mu.Unlock()
		return nil
	}
	p.mu.Unlock()

	p.acceptCancel()

	tasksDone := make(chan struct{})
	go func() {
		p.taskWg.Wait()
		close(tasksDone)
	}()

	graceCtx, graceCancel := context.WithTimeout(ctx, p.cfg.GracePeriod)
	defer graceCancel()

	select {
	case <-tasksDone:
	case <-graceCtx.Done():
		p.cancel()
		p.drainPending(p.poolCtx.Err())
		p.wg.Wait()
		<-tasksDone
	}

	// Input channels are intentionally never closed:
	// Submit callers may already be past the fast stopped check,
	// so shutdown is signaled only by poolCtx.
	p.cancel()
	p.wg.Wait()
	p.supWg.Wait()

	close(p.events)
	return nil
}

// Stats returns current pool statistics.
func (p *Pool[I, O]) Stats() PoolStats {
	var active int
	queued := len(p.queue)
	for i := range p.stats {
		active += int(p.stats[i].active.Load())
		queued += len(p.affinities[i])
	}

	return PoolStats{
		Active: active,
		Idle:   p.cfg.Size - active,
		Queued: queued,
		Total:  int(p.totalTasks.Load()),
		Failed: int(p.failCount.Load()),
	}
}

// pickWorkerForRouting picks a healthy worker for supervisor affinity routing.
// Without a supervisor, the shared queue provides fairness and dispatch is skipped.
func (p *Pool[I, O]) pickWorkerForRouting() int {
	if p.supervisor == nil {
		return -1
	}

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
	return -1
}

// runWorker is the goroutine loop for a single worker. If a supervisor is configured,
// panics are caught per-task, the task is failed,
// and the supervisor is notified to decide whether to keep the worker alive.
func (p *Pool[I, O]) runWorker(idx int) {
	defer p.wg.Done()

	workerID := fmt.Sprintf("%s-w%d", p.cfg.Name, idx)
	affinity := p.affinities[idx]
	queue := p.queue

	for {
		select {
		case <-p.poolCtx.Done():
			return
		case env := <-affinity:
			p.runEnvelope(workerID, idx, env)
			continue
		default:
		}

		select {
		case <-p.poolCtx.Done():
			return
		case env := <-affinity:
			p.runEnvelope(workerID, idx, env)
		case env := <-queue:
			p.runEnvelope(workerID, idx, env)
		}
	}
}

func (p *Pool[I, O]) runEnvelope(workerID string, idx int, env taskEnvelope[I, O]) {
	p.stats[idx].active.Add(1)
	defer p.stats[idx].active.Add(-1)
	defer p.taskWg.Done()
	p.executeTask(workerID, idx, env)
}

func (p *Pool[I, O]) drainPending(err error) {
	if err == nil {
		err = context.Canceled
	}
	for _, ch := range p.affinities {
		p.drainChannel(ch, err)
	}
	p.drainChannel(p.queue, err)
}

func (p *Pool[I, O]) drainChannel(ch chan taskEnvelope[I, O], err error) {
	for {
		select {
		case env := <-ch:
			env.handle.Cancel()
			var zero O
			env.handle.emit(errorEvent[O](err))
			env.handle.complete(zero, err)
			p.taskWg.Done()
		default:
			return
		}
	}
}

func (p *Pool[I, O]) stoppedError() error {
	return fmt.Errorf("worker: pool %q is stopped", p.cfg.Name)
}

// executeTask runs a single task within a worker goroutine.
func (p *Pool[I, O]) executeTask(workerID string, idx int, env taskEnvelope[I, O]) {
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
				return
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

	// Catch panics so the worker goroutine survives and the task handle is always completed —
	// callers waiting on handle.Result() or handle.Events() will never hang.
	// Report crash to supervisor BEFORE completing the handle
	// so supervisor state is consistent when callers observe task completion.
	defer func() {
		if r := recover(); r != nil {
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
			if p.supervisor != nil {
				p.supervisor.reportCrash(idx, r)
			}
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
}
