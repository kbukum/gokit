package worker

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kbukum/gokit/component"
)

// TickerFunc is the callback invoked on every tick.
type TickerFunc func(ctx context.Context) error

// TickerOption configures optional TickerWorker behavior.
type TickerOption func(*TickerWorker)

// WithRunOnStart causes the worker to execute fn once immediately when the background goroutine starts, before entering the periodic loop. The initial run does NOT block app startup — it runs inside the goroutine launched by Start.
func WithRunOnStart() TickerOption {
	return func(w *TickerWorker) { w.runOnStart = true }
}

// WithOnError registers a callback that is invoked after every tick that returns a non-nil error. Useful for logging or alerting.
func WithOnError(fn func(error)) TickerOption {
	return func(w *TickerWorker) { w.onError = fn }
}

// TickerWorker is a Component that runs a function on a fixed interval.
//
// Start launches a background goroutine; Stop signals it and waits for a clean exit. Health reports the last-run time and any recent errors.
//
// Example:
//
//	tw := worker.NewTickerWorker("cache-cleanup", 30*time.Second, func(ctx context.Context) error {
//	    return cache.Cleanup(ctx)
//	}, worker.WithRunOnStart())
//	registry.Register(tw)
type TickerWorker struct {
	name       string
	interval   time.Duration
	fn         TickerFunc
	runOnStart bool
	onError    func(error)

	cancel  context.CancelFunc
	done    chan struct{}
	running atomic.Bool

	mu        sync.RWMutex
	lastRun   time.Time
	lastErr   error
	runCount  uint64
	failCount uint64
}

// NewTickerWorker creates a TickerWorker with the given name, interval, and handler.
func NewTickerWorker(name string, interval time.Duration, fn TickerFunc, opts ...TickerOption) *TickerWorker {
	w := &TickerWorker{
		name:     name,
		interval: interval,
		fn:       fn,
	}
	for _, o := range opts {
		o(w)
	}
	return w
}

// Name returns the component name.
func (w *TickerWorker) Name() string { return w.name }

// Start launches the ticker loop in a background goroutine.
func (w *TickerWorker) Start(_ context.Context) error {
	if w.running.Load() {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	w.done = make(chan struct{})
	w.running.Store(true)

	go w.loop(ctx) //nolint:contextcheck // ticker loop is long-lived and detached from the lifecycle Start ctx
	return nil
}

// Stop signals the ticker loop to exit and waits for it to finish.
func (w *TickerWorker) Stop(_ context.Context) error {
	if !w.running.Load() {
		return nil
	}
	w.cancel()
	<-w.done
	w.running.Store(false)
	return nil
}

// Health returns the current health status.
func (w *TickerWorker) Health(_ context.Context) component.Health {
	if !w.running.Load() {
		return component.Health{
			Name:    w.name,
			Status:  component.StatusUnhealthy,
			Message: "not running",
		}
	}
	w.mu.RLock()
	lastErr := w.lastErr
	runCount := w.runCount
	failCount := w.failCount
	w.mu.RUnlock()

	if lastErr != nil {
		return component.Health{
			Name:    w.name,
			Status:  component.StatusDegraded,
			Message: lastErr.Error(),
		}
	}
	msg := ""
	if runCount > 0 {
		msg = "ok"
	}
	_ = failCount // available for extended health
	return component.Health{
		Name:    w.name,
		Status:  component.StatusHealthy,
		Message: msg,
	}
}

// RunCount returns the total number of completed ticks.
func (w *TickerWorker) RunCount() uint64 {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.runCount
}

// FailCount returns the total number of failed ticks.
func (w *TickerWorker) FailCount() uint64 {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.failCount
}

func (w *TickerWorker) loop(ctx context.Context) {
	defer close(w.done)

	if w.runOnStart {
		w.tick(ctx)
	}

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.tick(ctx)
		}
	}
}

func (w *TickerWorker) tick(ctx context.Context) {
	err := w.fn(ctx)
	w.mu.Lock()
	w.lastRun = time.Now()
	w.runCount++
	if err != nil {
		w.lastErr = err
		w.failCount++
	} else {
		w.lastErr = nil
	}
	w.mu.Unlock()
	if err != nil && w.onError != nil {
		w.onError(err)
	}
}
