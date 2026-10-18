package messaging

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/resilience"
	"github.com/kbukum/gokit/util"
)

// Managed consumer defaults.
const (
	// DefaultStopTimeout bounds ManagedConsumer.Stop when the caller's ctx has no deadline.
	DefaultStopTimeout = 10 * time.Second
	// DefaultRetryDelay is the first wait before a managed consumer restarts.
	DefaultRetryDelay = time.Second
	// DefaultMaxRetryDelay caps the wait before a managed consumer restarts.
	DefaultMaxRetryDelay = 30 * time.Second
)

// ErrConsumeEnded is reported when Consume returns without an error while the managed consumer is still running.
var ErrConsumeEnded = errors.New("messaging: consume ended")

// ConsumerObserver is told what a managed consumer does. Its methods must return promptly.
type ConsumerObserver interface {
	// Handled is called after each handler call with the message, its age when handled, zero when the message has no
	// timestamp, and the handler's result.
	Handled(msg Message, age time.Duration, err error)
	// Failed is called when Consume ends with err while the consumer runs; restart reports whether it starts again.
	Failed(err error, restart bool)
}

// LoggingObserver logs consumption failures to log; handled messages are not logged. A nil log observes nothing.
func LoggingObserver(log *logging.Logger) ConsumerObserver {
	return loggingObserver{log: log}
}

type loggingObserver struct{ log *logging.Logger }

func (loggingObserver) Handled(Message, time.Duration, error) {}

func (o loggingObserver) Failed(err error, restart bool) {
	if o.log != nil {
		o.log.Error("Managed consumer failed", map[string]any{"error": err.Error(), "restart": restart})
	}
}

type nopObserver struct{}

func (nopObserver) Handled(Message, time.Duration, error) {}
func (nopObserver) Failed(error, bool)                    {}

// ManagedConsumerConfig configures a [ManagedConsumer].
type ManagedConsumerConfig struct {
	// Name names the component; empty uses "consumer:" and the topic.
	Name     string
	Consumer Consumer
	Handler  MessageHandler
	// HandleTimeout bounds each handler call; zero leaves it to the transport.
	HandleTimeout time.Duration
	// Restart reports whether consumption starts again after Consume ends with err. Nil never restarts, and
	// [ErrClosed] never restarts.
	Restart func(err error) bool
	// RetryDelay is the first wait before a restart. It doubles per consecutive failure up to MaxRetryDelay and starts
	// over once a run handles a message. Zero values use [DefaultRetryDelay] and [DefaultMaxRetryDelay].
	RetryDelay    time.Duration
	MaxRetryDelay time.Duration
	// Observer is told about handled messages and failures; nil observes nothing.
	Observer ConsumerObserver
	// Clock schedules restarts and measures message age; nil uses the system clock.
	Clock util.TimerClock
}

// ManagedConsumer runs a [Consumer] in the background as a lifecycle component. Consumption does not follow the
// context Start is given; only Stop or Drain ends it. Stop closes the consumer, so a stopped managed consumer cannot
// start again.
type ManagedConsumer struct {
	name     string
	consumer Consumer
	handler  MessageHandler
	timeout  time.Duration
	restart  func(error) bool
	retry    time.Duration
	maxRetry time.Duration
	observer ConsumerObserver
	clock    util.TimerClock

	// age is the last measured message age in nanoseconds.
	age atomic.Int64
	// runHandled counts messages handled in the current run, to reset the restart backoff.
	runHandled atomic.Int64

	mu        sync.Mutex
	state     managedState
	cancel    context.CancelFunc
	done      chan struct{}
	closeOnce sync.Once
	// closed is closed once the consumer's Close returned; closeError is its result.
	closed     chan struct{}
	closeError error
}

type managedState int

const (
	managedIdle managedState = iota
	managedConsuming
	managedRestarting
	managedEnded
	managedStopped
)

// NewManagedConsumer validates cfg and returns a stopped managed consumer.
func NewManagedConsumer(cfg ManagedConsumerConfig) (*ManagedConsumer, error) {
	switch {
	case util.IsNil(cfg.Consumer):
		return nil, errors.New("messaging: managed consumer requires a consumer")
	case cfg.Handler == nil:
		return nil, errors.New("messaging: managed consumer requires a handler")
	case cfg.HandleTimeout < 0 || cfg.RetryDelay < 0 || cfg.MaxRetryDelay < 0:
		return nil, errors.New("messaging: managed consumer durations cannot be negative")
	}
	retry, maxRetry := cfg.RetryDelay, cfg.MaxRetryDelay
	if retry == 0 {
		retry = DefaultRetryDelay
	}
	if maxRetry == 0 {
		maxRetry = max(DefaultMaxRetryDelay, retry)
	}
	if maxRetry < retry {
		return nil, errors.New("messaging: managed consumer max retry delay is below its retry delay")
	}
	m := &ManagedConsumer{
		name: cfg.Name, consumer: cfg.Consumer, handler: cfg.Handler, timeout: cfg.HandleTimeout, restart: cfg.Restart,
		retry: retry, maxRetry: maxRetry, observer: cfg.Observer, clock: cfg.Clock, closed: make(chan struct{}),
	}
	if m.name == "" {
		m.name = "consumer:" + cfg.Consumer.Topic()
	}
	if util.IsNil(m.observer) {
		m.observer = nopObserver{}
	}
	if util.IsNil(m.clock) {
		m.clock = util.SystemClock{}
	}
	return m, nil
}

// Name implements [component.Component].
func (m *ManagedConsumer) Name() string { return m.name }

// Topic returns the consumer's topic.
func (m *ManagedConsumer) Topic() string { return m.consumer.Topic() }

// Start begins consuming in the background. It is a no-op while running and returns [ErrClosed] once stopped.
func (m *ManagedConsumer) Start(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	switch m.state {
	case managedStopped:
		return ErrClosed
	case managedIdle, managedEnded:
	default:
		return nil
	}
	run, cancel := context.WithCancel(context.WithoutCancel(ctx))
	m.cancel, m.done, m.state = cancel, make(chan struct{}), managedConsuming
	go m.run(run, m.done)
	return nil
}

// Stop ends consumption, waits within ctx for the handler in hand, then closes the consumer and waits within ctx for
// that too. Without a deadline on ctx it waits at most [DefaultStopTimeout]. The consumer is closed even when the
// wait times out; a close that outlasts ctx goes on in the background, and a later Stop waits for it again.
func (m *ManagedConsumer) Stop(ctx context.Context) error {
	m.mu.Lock()
	cancel, done := m.cancel, m.done
	m.state = managedStopped
	m.mu.Unlock()
	if _, ok := ctx.Deadline(); !ok {
		var stop context.CancelFunc
		ctx, stop = context.WithTimeout(ctx, DefaultStopTimeout)
		defer stop()
	}
	if done != nil {
		cancel()
		select {
		case <-done:
		case <-ctx.Done():
			m.startClose()
			return fmt.Errorf("messaging: managed consumer %s did not stop: %w", m.name, context.Cause(ctx))
		}
	}
	m.startClose()
	select {
	case <-m.closed:
		return m.closeError
	case <-ctx.Done():
		return fmt.Errorf("messaging: managed consumer %s did not close: %w", m.name, context.Cause(ctx))
	}
}

// startClose closes the consumer once, in the background; closed is closed when it returns.
func (m *ManagedConsumer) startClose() {
	m.closeOnce.Do(func() {
		go func() {
			defer close(m.closed)
			m.closeError = m.consumer.Close()
		}()
	})
}

// DrainPhase stops the consumer with the other workers, before the clients it uses.
func (*ManagedConsumer) DrainPhase() component.DrainPhase { return component.DrainWorkers }

// Drain stops the consumer.
func (m *ManagedConsumer) Drain(ctx context.Context) error { return m.Stop(ctx) }

// Health is healthy while consuming, degraded while waiting to restart or while a consumer that implements
// [ConsumerStatus] reports a receive failure, and unhealthy before Start, after consumption ended for good and after
// Stop. Causes go to the observer, not into health.
func (m *ManagedConsumer) Health(context.Context) component.Health {
	m.mu.Lock()
	state := m.state
	m.mu.Unlock()
	switch state {
	case managedIdle:
		return component.Unhealthy(m.name, "not started")
	case managedEnded:
		return component.Unhealthy(m.name, "consumption ended")
	case managedStopped:
		return component.Unhealthy(m.name, "stopped")
	case managedRestarting:
		return component.Degraded(m.name, "restarting")
	case managedConsuming:
		if status, ok := m.consumer.(ConsumerStatus); ok && status.ConsumeStatus() != nil {
			return component.Degraded(m.name, "not receiving")
		}
	}
	return component.Healthy(m.name)
}

// IsRunning reports whether consumption is running or waiting to restart.
func (m *ManagedConsumer) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.done != nil && !m.finished()
}

// finished reports whether the run goroutine has returned. The caller holds mu.
func (m *ManagedConsumer) finished() bool {
	select {
	case <-m.done:
		return true
	default:
		return false
	}
}

// MessageAge is the age of the last handled message that carried a timestamp, measured when it was handled.
func (m *ManagedConsumer) MessageAge() time.Duration { return time.Duration(m.age.Load()) }

func (m *ManagedConsumer) run(ctx context.Context, done chan struct{}) {
	defer close(done)
	failures := 0
	for {
		m.runHandled.Store(0)
		err := m.consumer.Consume(ctx, m.handle)
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			err = ErrConsumeEnded
		}
		restart := m.restart != nil && !errors.Is(err, ErrClosed) && m.restart(err)
		if !m.transition(restart) {
			return
		}
		m.observer.Failed(err, restart)
		if !restart {
			return
		}
		if m.runHandled.Load() > 0 {
			failures = 0
		}
		failures++
		if !m.wait(ctx, resilience.CalculateBackoff(failures, m.retry, m.maxRetry)) || !m.consuming() {
			return
		}
	}
}

// transition records that consumption ended and whether it restarts, unless the consumer is stopping.
func (m *ManagedConsumer) transition(restart bool) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state == managedStopped {
		return false
	}
	m.state = managedEnded
	if restart {
		m.state = managedRestarting
	}
	return true
}

func (m *ManagedConsumer) consuming() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state != managedRestarting {
		return false
	}
	m.state = managedConsuming
	return true
}

func (m *ManagedConsumer) wait(ctx context.Context, d time.Duration) bool {
	timer := m.clock.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C():
		return true
	}
}

func (m *ManagedConsumer) handle(ctx context.Context, msg Message) error {
	var age time.Duration
	if !msg.Timestamp.IsZero() {
		age = max(m.clock.Now().Sub(msg.Timestamp), 0)
		m.age.Store(int64(age))
	}
	if m.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, m.timeout)
		defer cancel()
	}
	err := m.handler(ctx, msg)
	m.runHandled.Add(1)
	m.observer.Handled(msg, age, err)
	return err
}
