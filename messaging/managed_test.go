package messaging

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/util"
)

const waitFor = 2 * time.Second

// stubConsumer runs consumeFn per Consume call, or blocks until ctx ends.
type stubConsumer struct {
	topic     string
	consumeFn func(ctx context.Context, handler MessageHandler) error
	closeFn   func() error
	calls     atomic.Int32
	closed    atomic.Bool
	status    atomic.Pointer[error]
}

func (s *stubConsumer) Consume(ctx context.Context, handler MessageHandler) error {
	s.calls.Add(1)
	if s.consumeFn != nil {
		return s.consumeFn(ctx, handler)
	}
	<-ctx.Done()
	return ctx.Err()
}

func (s *stubConsumer) Topic() string { return s.topic }

func (s *stubConsumer) Close() error {
	s.closed.Store(true)
	if s.closeFn != nil {
		return s.closeFn()
	}
	return nil
}

// statusConsumer also reports its receive status.
type statusConsumer struct{ *stubConsumer }

func (s statusConsumer) ConsumeStatus() error {
	if p := s.status.Load(); p != nil {
		return *p
	}
	return nil
}

// timerClock is a fake clock that reports each timer it schedules.
type timerClock struct {
	*util.FakeClock
	timers chan time.Duration
}

func newTimerClock() *timerClock {
	return &timerClock{FakeClock: util.NewFakeClock(time.Time{}), timers: make(chan time.Duration, 16)}
}

func (c *timerClock) NewTimer(d time.Duration) util.Timer {
	t := c.FakeClock.NewTimer(d)
	c.timers <- d
	return t
}

func (c *timerClock) expectTimer(t *testing.T, want time.Duration) {
	t.Helper()
	select {
	case got := <-c.timers:
		if got != want {
			t.Fatalf("retry delay = %v, want %v", got, want)
		}
	case <-time.After(waitFor):
		t.Fatalf("no retry scheduled, want %v", want)
	}
}

type failure struct {
	err     error
	restart bool
}

type handled struct {
	msg Message
	age time.Duration
	err error
}

type recordingObserver struct {
	failed  chan failure
	handled chan handled
}

func newObserver() *recordingObserver {
	return &recordingObserver{failed: make(chan failure, 16), handled: make(chan handled, 16)}
}

func (o *recordingObserver) Failed(err error, restart bool) { o.failed <- failure{err, restart} }
func (o *recordingObserver) Handled(msg Message, age time.Duration, err error) {
	o.handled <- handled{msg, age, err}
}

func (o *recordingObserver) expectFailed(t *testing.T) failure {
	t.Helper()
	select {
	case f := <-o.failed:
		return f
	case <-time.After(waitFor):
		t.Fatal("no failure observed")
		return failure{}
	}
}

func newManaged(t *testing.T, cfg ManagedConsumerConfig) *ManagedConsumer {
	t.Helper()
	if cfg.Handler == nil {
		cfg.Handler = func(context.Context, Message) error { return nil }
	}
	mc, err := NewManagedConsumer(cfg)
	if err != nil {
		t.Fatalf("NewManagedConsumer: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), waitFor)
		defer cancel()
		_ = mc.Stop(ctx)
	})
	return mc
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(waitFor)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func healthIs(mc *ManagedConsumer, status component.HealthStatus) func() bool {
	return func() bool { return mc.Health(context.Background()).Status == status }
}

func TestNewManagedConsumerValidates(t *testing.T) {
	handler := func(context.Context, Message) error { return nil }
	sc := &stubConsumer{topic: "t"}
	cases := map[string]ManagedConsumerConfig{
		"no consumer":           {Handler: handler},
		"no handler":            {Consumer: sc},
		"negative timeout":      {Consumer: sc, Handler: handler, HandleTimeout: -time.Second},
		"negative retry":        {Consumer: sc, Handler: handler, RetryDelay: -time.Second},
		"max retry below retry": {Consumer: sc, Handler: handler, RetryDelay: 2 * time.Second, MaxRetryDelay: time.Second},
	}
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := NewManagedConsumer(cfg); err == nil {
				t.Fatal("want an error")
			}
		})
	}
}

func TestManagedConsumerNameAndTopic(t *testing.T) {
	mc := newManaged(t, ManagedConsumerConfig{Consumer: &stubConsumer{topic: "orders"}})
	if mc.Name() != "consumer:orders" || mc.Topic() != "orders" {
		t.Fatalf("name %q topic %q", mc.Name(), mc.Topic())
	}
	named := newManaged(t, ManagedConsumerConfig{Name: "usage", Consumer: &stubConsumer{topic: "orders"}})
	if named.Name() != "usage" {
		t.Fatalf("name %q", named.Name())
	}
	if named.DrainPhase() != component.DrainWorkers {
		t.Fatal("want the worker drain phase")
	}
}

func TestManagedConsumerLifecycle(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sc := &stubConsumer{topic: "t"}
		mc := newManaged(t, ManagedConsumerConfig{Consumer: sc})
		if h := mc.Health(context.Background()); h.Status != component.StatusUnhealthy || mc.IsRunning() {
			t.Fatalf("before start: %+v", h)
		}
		ctx, cancel := context.WithCancel(context.Background())
		if err := mc.Start(ctx); err != nil {
			t.Fatalf("Start: %v", err)
		}
		if err := mc.Start(ctx); err != nil {
			t.Fatalf("second Start: %v", err)
		}
		eventually(t, "consuming", func() bool { return sc.calls.Load() == 1 })
		cancel()
		// Consumption outlives the context Start was given.
		synctest.Wait()
		if !mc.IsRunning() || mc.Health(context.Background()).Status != component.StatusHealthy {
			t.Fatalf("after the start context ended: running %v health %+v", mc.IsRunning(), mc.Health(context.Background()))
		}
		if err := mc.Drain(context.Background()); err != nil {
			t.Fatalf("Drain: %v", err)
		}
		if mc.IsRunning() || !sc.closed.Load() {
			t.Fatal("want stopped and closed")
		}
		if h := mc.Health(context.Background()); h.Status != component.StatusUnhealthy || h.Message != "stopped" {
			t.Fatalf("after stop: %+v", h)
		}
		if err := mc.Start(context.Background()); !errors.Is(err, ErrClosed) {
			t.Fatalf("Start after Stop = %v, want ErrClosed", err)
		}
		if err := mc.Stop(context.Background()); err != nil {
			t.Fatalf("second Stop: %v", err)
		}
	})
}

func TestManagedConsumerStartRefusesEndedContext(t *testing.T) {
	mc := newManaged(t, ManagedConsumerConfig{Consumer: &stubConsumer{topic: "t"}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := mc.Start(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Start = %v", err)
	}
}

func TestManagedConsumerStopBeforeStartCloses(t *testing.T) {
	sc := &stubConsumer{topic: "t"}
	mc := newManaged(t, ManagedConsumerConfig{Consumer: sc})
	if err := mc.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if !sc.closed.Load() {
		t.Fatal("want closed")
	}
	if err := mc.Start(context.Background()); !errors.Is(err, ErrClosed) {
		t.Fatalf("Start = %v", err)
	}
}

func TestManagedConsumerEndsWithoutRestartPolicy(t *testing.T) {
	boom := errors.New("boom")
	sc := &stubConsumer{topic: "t", consumeFn: func(context.Context, MessageHandler) error { return boom }}
	obs := newObserver()
	mc := newManaged(t, ManagedConsumerConfig{Consumer: sc, Observer: obs})
	if err := mc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f := obs.expectFailed(t); !errors.Is(f.err, boom) || f.restart {
		t.Fatalf("failure %+v", f)
	}
	eventually(t, "ended", func() bool { return !mc.IsRunning() })
	if h := mc.Health(context.Background()); h.Status != component.StatusUnhealthy || h.Message != "consumption ended" {
		t.Fatalf("health %+v", h)
	}
	if sc.closed.Load() {
		t.Fatal("only Stop closes the consumer")
	}
}

func TestManagedConsumerRestartsWithBackoff(t *testing.T) {
	boom := errors.New("boom")
	var mu sync.Mutex
	failures := 2
	deliver := make(chan struct{})
	sc := &stubConsumer{topic: "t"}
	sc.consumeFn = func(ctx context.Context, h MessageHandler) error {
		mu.Lock()
		fail := failures > 0
		failures--
		mu.Unlock()
		if fail {
			return boom
		}
		select {
		case <-deliver:
			_ = h(ctx, Message{})
			return boom
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	clock, obs := newTimerClock(), newObserver()
	mc := newManaged(t, ManagedConsumerConfig{
		Consumer: sc, Observer: obs, Clock: clock, RetryDelay: time.Second, MaxRetryDelay: 3 * time.Second,
		Restart: func(err error) bool { return errors.Is(err, boom) },
	})
	if err := mc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f := obs.expectFailed(t); !f.restart {
		t.Fatalf("failure %+v", f)
	}
	clock.expectTimer(t, time.Second)
	if h := mc.Health(context.Background()); h.Status != component.StatusDegraded || h.Message != "restarting" {
		t.Fatalf("health while waiting %+v", h)
	}
	clock.Advance(time.Second)
	obs.expectFailed(t)
	clock.expectTimer(t, 2*time.Second)
	clock.Advance(2 * time.Second)
	eventually(t, "healthy", healthIs(mc, component.StatusHealthy))
	// A run that handled a message resets the backoff.
	close(deliver)
	obs.expectFailed(t)
	clock.expectTimer(t, time.Second)
	if got := sc.calls.Load(); got != 3 {
		t.Fatalf("Consume calls = %d, want 3", got)
	}
}

func TestManagedConsumerBackoffIsCapped(t *testing.T) {
	boom := errors.New("boom")
	sc := &stubConsumer{topic: "t", consumeFn: func(context.Context, MessageHandler) error { return boom }}
	clock, obs := newTimerClock(), newObserver()
	mc := newManaged(t, ManagedConsumerConfig{
		Consumer: sc, Observer: obs, Clock: clock, RetryDelay: time.Second, MaxRetryDelay: 3 * time.Second,
		Restart: func(error) bool { return true },
	})
	if err := mc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, want := range []time.Duration{time.Second, 2 * time.Second, 3 * time.Second, 3 * time.Second} {
		obs.expectFailed(t)
		clock.expectTimer(t, want)
		clock.Advance(want)
	}
}

func TestManagedConsumerNeverRestartsClosedConsumer(t *testing.T) {
	sc := &stubConsumer{topic: "t", consumeFn: func(context.Context, MessageHandler) error { return ErrClosed }}
	obs := newObserver()
	mc := newManaged(t, ManagedConsumerConfig{Consumer: sc, Observer: obs, Restart: func(error) bool { return true }})
	if err := mc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f := obs.expectFailed(t); f.restart || !errors.Is(f.err, ErrClosed) {
		t.Fatalf("failure %+v", f)
	}
	eventually(t, "ended", func() bool { return !mc.IsRunning() })
}

func TestManagedConsumerReportsEndWithoutError(t *testing.T) {
	sc := &stubConsumer{topic: "t", consumeFn: func(context.Context, MessageHandler) error { return nil }}
	obs := newObserver()
	mc := newManaged(t, ManagedConsumerConfig{Consumer: sc, Observer: obs})
	if err := mc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f := obs.expectFailed(t); !errors.Is(f.err, ErrConsumeEnded) {
		t.Fatalf("failure %+v", f)
	}
}

func TestManagedConsumerHealthFollowsConsumerStatus(t *testing.T) {
	sc := statusConsumer{&stubConsumer{topic: "t"}}
	mc := newManaged(t, ManagedConsumerConfig{Consumer: sc})
	if err := mc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	eventually(t, "healthy", healthIs(mc, component.StatusHealthy))
	down := errors.New("broker unreachable")
	sc.status.Store(&down)
	if h := mc.Health(context.Background()); h.Status != component.StatusDegraded || strings.Contains(h.Message, "unreachable") {
		t.Fatalf("health %+v", h)
	}
	sc.status.Store(nil)
	eventually(t, "healthy again", healthIs(mc, component.StatusHealthy))
}

func TestManagedConsumerHandlerTimeoutAndMessageAge(t *testing.T) {
	clock := newTimerClock()
	sent := clock.Now().Add(-5 * time.Second)
	results := make(chan error, 2)
	sc := &stubConsumer{topic: "t", consumeFn: func(ctx context.Context, h MessageHandler) error {
		results <- h(ctx, Message{Key: "a", Timestamp: sent})
		results <- h(ctx, Message{Key: "b"})
		<-ctx.Done()
		return ctx.Err()
	}}
	handlerErr := errors.New("handler")
	obs := newObserver()
	mc := newManaged(t, ManagedConsumerConfig{
		Consumer: sc, Observer: obs, Clock: clock, HandleTimeout: time.Minute,
		Handler: func(ctx context.Context, m Message) error {
			deadline, ok := ctx.Deadline()
			if !ok || deadline.After(time.Now().Add(time.Minute)) {
				t.Errorf("handler deadline %v %v", deadline, ok)
			}
			if m.Key == "b" {
				return handlerErr
			}
			return nil
		},
	})
	if mc.MessageAge() != 0 {
		t.Fatal("want no age before a message")
	}
	if err := mc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, want := range []handled{{age: 5 * time.Second}, {err: handlerErr}} {
		select {
		case got := <-obs.handled:
			if got.age != want.age || !errors.Is(got.err, want.err) {
				t.Fatalf("handled %+v, want age %v err %v", got, want.age, want.err)
			}
		case <-time.After(waitFor):
			t.Fatal("not handled")
		}
	}
	if err := <-results; err != nil {
		t.Fatalf("first result %v", err)
	}
	if err := <-results; !errors.Is(err, handlerErr) {
		t.Fatalf("second result %v", err)
	}
	// A message without a timestamp keeps the last known age.
	if got := mc.MessageAge(); got != 5*time.Second {
		t.Fatalf("MessageAge = %v", got)
	}
}

func TestManagedConsumerStopIsBounded(t *testing.T) {
	release := make(chan struct{})
	sc := &stubConsumer{topic: "t", consumeFn: func(context.Context, MessageHandler) error {
		<-release
		return nil
	}}
	mc := newManaged(t, ManagedConsumerConfig{Consumer: sc})
	if err := mc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	eventually(t, "consuming", func() bool { return sc.calls.Load() == 1 })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := mc.Stop(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stop = %v", err)
	}
	eventually(t, "a timed-out Stop still closes the consumer", sc.closed.Load)
	if err := mc.Start(context.Background()); !errors.Is(err, ErrClosed) {
		t.Fatalf("Start after a timed-out Stop = %v", err)
	}
	close(release)
	eventually(t, "loop ended", func() bool { return !mc.IsRunning() })
}

// Stop's budget also bounds closing the consumer; a later Stop waits for the same close.
func TestManagedConsumerStopBoundsClose(t *testing.T) {
	release := make(chan struct{})
	closeErr := errors.New("close")
	sc := &stubConsumer{topic: "t", closeFn: func() error { <-release; return closeErr }}
	mc := newManaged(t, ManagedConsumerConfig{Consumer: sc})
	if err := mc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := mc.Stop(ctx); !errors.Is(err, context.DeadlineExceeded) || errors.Is(err, closeErr) {
		t.Fatalf("Stop with a stuck close = %v", err)
	}
	close(release)
	if err := mc.Stop(context.Background()); !errors.Is(err, closeErr) {
		t.Fatalf("Stop after the close finished = %v", err)
	}
	if sc.calls.Load() != 1 {
		t.Fatalf("Consume calls = %d", sc.calls.Load())
	}
}

func TestLoggingObserver(t *testing.T) {
	obs := LoggingObserver(nil)
	obs.Failed(errors.New("boom"), true)
	obs.Handled(Message{}, 0, nil)
}

func TestManagedConsumerStopReportsCloseError(t *testing.T) {
	closeErr := errors.New("close")
	sc := &stubConsumer{topic: "t", closeFn: func() error { return closeErr }}
	mc := newManaged(t, ManagedConsumerConfig{Consumer: sc})
	if err := mc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := mc.Stop(context.Background()); !errors.Is(err, closeErr) {
		t.Fatalf("Stop = %v", err)
	}
	if err := mc.Stop(context.Background()); !errors.Is(err, closeErr) {
		t.Fatalf("second Stop = %v", err)
	}
}
