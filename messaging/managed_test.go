package messaging

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kbukum/gokit/logger"
)

// stubConsumer is a minimal Consumer implementation for testing.
type stubConsumer struct {
	topic     string
	consumeFn func(ctx context.Context, handler MessageHandler) error
	closeFn   func() error
	closed    atomic.Bool
}

func (s *stubConsumer) Consume(ctx context.Context, handler MessageHandler) error {
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

func newTestLogger() *logger.Logger {
	return logger.NewDefault("test")
}

func TestManagedConsumer_StartAndStop(t *testing.T) {
	sc := &stubConsumer{topic: "test-topic"}
	mc := NewManagedConsumer(ManagedConsumerConfig{
		Consumer: sc,
		Handler:  func(_ context.Context, _ Message) error { return nil },
		Log:      newTestLogger(),
	})

	if mc.IsRunning() {
		t.Fatal("expected not running before Start")
	}

	if err := mc.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	// Give goroutine time to start
	time.Sleep(50 * time.Millisecond)

	if !mc.IsRunning() {
		t.Fatal("expected running after Start")
	}

	if err := mc.Stop(context.Background()); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}

	if mc.IsRunning() {
		t.Fatal("expected not running after Stop")
	}

	if !sc.closed.Load() {
		t.Fatal("expected underlying consumer to be closed after Stop")
	}
}

func TestManagedConsumer_DoubleStart_Noop(t *testing.T) {
	sc := &stubConsumer{topic: "test-topic"}
	mc := NewManagedConsumer(ManagedConsumerConfig{
		Consumer: sc,
		Handler:  func(_ context.Context, _ Message) error { return nil },
		Log:      newTestLogger(),
	})

	ctx := context.Background()
	if err := mc.Start(ctx); err != nil {
		t.Fatalf("first Start returned error: %v", err)
	}
	defer mc.Stop(context.Background())

	time.Sleep(50 * time.Millisecond)

	// Second Start should be a no-op (return nil)
	if err := mc.Start(ctx); err != nil {
		t.Fatalf("second Start returned error: %v", err)
	}

	if !mc.IsRunning() {
		t.Fatal("expected still running after double Start")
	}
}

func TestManagedConsumer_StopWhenNotRunning_Noop(t *testing.T) {
	sc := &stubConsumer{topic: "test-topic"}
	mc := NewManagedConsumer(ManagedConsumerConfig{
		Consumer: sc,
		Handler:  func(_ context.Context, _ Message) error { return nil },
		Log:      newTestLogger(),
	})

	// Stop before ever starting should return nil
	if err := mc.Stop(context.Background()); err != nil {
		t.Fatalf("Stop when not running returned error: %v", err)
	}

	if mc.IsRunning() {
		t.Fatal("expected not running")
	}
}

func TestManagedConsumer_IsRunning_StateTransitions(t *testing.T) {
	started := make(chan struct{})
	sc := &stubConsumer{
		topic: "state-topic",
		consumeFn: func(ctx context.Context, _ MessageHandler) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		},
	}
	mc := NewManagedConsumer(ManagedConsumerConfig{
		Consumer: sc,
		Handler:  func(_ context.Context, _ Message) error { return nil },
		Log:      newTestLogger(),
	})

	// Phase 1: not running
	if mc.IsRunning() {
		t.Fatal("expected not running initially")
	}

	// Phase 2: start
	if err := mc.Start(context.Background()); err != nil {
		t.Fatalf("Start error: %v", err)
	}

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for consume to start")
	}

	if !mc.IsRunning() {
		t.Fatal("expected running after Start")
	}

	// Phase 3: stop
	if err := mc.Stop(context.Background()); err != nil {
		t.Fatalf("Stop error: %v", err)
	}

	if mc.IsRunning() {
		t.Fatal("expected not running after Stop")
	}
}

func TestManagedConsumer_Topic(t *testing.T) {
	sc := &stubConsumer{topic: "my-topic"}
	mc := NewManagedConsumer(ManagedConsumerConfig{
		Consumer: sc,
		Handler:  func(_ context.Context, _ Message) error { return nil },
		Log:      newTestLogger(),
	})

	if got := mc.Topic(); got != "my-topic" {
		t.Fatalf("Topic() = %q, want %q", got, "my-topic")
	}
}

func TestManagedConsumer_ConsumeError(t *testing.T) {
	consumeErr := errors.New("consume failed")
	sc := &stubConsumer{
		topic: "err-topic",
		consumeFn: func(_ context.Context, _ MessageHandler) error {
			return consumeErr
		},
	}
	mc := NewManagedConsumer(ManagedConsumerConfig{
		Consumer: sc,
		Handler:  func(_ context.Context, _ Message) error { return nil },
		Log:      newTestLogger(),
	})

	if err := mc.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	// Wait for the goroutine to finish due to consume error
	time.Sleep(200 * time.Millisecond)

	if mc.IsRunning() {
		t.Fatal("expected not running after consume error")
	}
}
