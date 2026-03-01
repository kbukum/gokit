package kafka

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/logger"
)

// mockProducer implements ProducerCloser for testing
type mockProducer struct {
	closed atomic.Bool
}

func (m *mockProducer) Close() error {
	m.closed.Store(true)
	return nil
}

// mockConsumer implements ConsumerRunner for testing
type mockConsumer struct {
	topic      string
	consumed   atomic.Bool
	closeCalls atomic.Int32
}

func (m *mockConsumer) Consume(ctx context.Context) error {
	m.consumed.Store(true)
	<-ctx.Done()
	return ctx.Err()
}

func (m *mockConsumer) Close() error {
	m.closeCalls.Add(1)
	return nil
}

func (m *mockConsumer) Topic() string { return m.topic }

func TestComponent_NewComponent(t *testing.T) {
	log := logger.New(&logger.Config{Level: "error"}, "test")
	cfg := Config{Brokers: []string{"localhost:9092"}}
	comp := NewComponent(cfg, log)
	if comp == nil {
		t.Fatal("expected non-nil component")
	}
}

func TestComponent_Name(t *testing.T) {
	log := logger.New(&logger.Config{Level: "error"}, "test")
	comp := NewComponent(Config{}, log)
	if comp.Name() != "kafka" {
		t.Errorf("Name() = %q, want kafka", comp.Name())
	}
}

func TestComponent_SetProducer(t *testing.T) {
	log := logger.New(&logger.Config{Level: "error"}, "test")
	comp := NewComponent(Config{}, log)
	mp := &mockProducer{}
	comp.SetProducer(mp)
	if comp.Producer() != mp {
		t.Error("Producer() should return the set producer")
	}
}

func TestComponent_AddConsumer(t *testing.T) {
	log := logger.New(&logger.Config{Level: "error"}, "test")
	comp := NewComponent(Config{}, log)
	mc := &mockConsumer{topic: "events"}
	comp.AddConsumer(mc)

	// verify Describe includes the topic
	desc := comp.Describe()
	if desc.Name != "Kafka" {
		t.Errorf("Describe().Name = %q, want Kafka", desc.Name)
	}
}

func TestComponent_Describe(t *testing.T) {
	log := logger.New(&logger.Config{Level: "error"}, "test")
	cfg := Config{Brokers: []string{"b1:9092", "b2:9092"}}
	comp := NewComponent(cfg, log)
	mp := &mockProducer{}
	comp.SetProducer(mp)
	comp.AddConsumer(&mockConsumer{topic: "t1"})
	comp.AddConsumer(&mockConsumer{topic: "t2"})

	desc := comp.Describe()
	if desc.Type != "kafka" {
		t.Errorf("Type = %q, want kafka", desc.Type)
	}
	if desc.Details == "" {
		t.Error("expected non-empty Details")
	}
}

func TestComponent_StartStop(t *testing.T) {
	log := logger.New(&logger.Config{Level: "error"}, "test")
	comp := NewComponent(Config{}, log)
	mc := &mockConsumer{topic: "test"}
	comp.AddConsumer(mc)
	mp := &mockProducer{}
	comp.SetProducer(mp)

	ctx := context.Background()
	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	// double start should be no-op
	if err := comp.Start(ctx); err != nil {
		t.Fatalf("double Start() error: %v", err)
	}

	if err := comp.Stop(ctx); err != nil {
		t.Fatalf("Stop() error: %v", err)
	}

	if !mc.consumed.Load() {
		t.Error("consumer should have been consumed")
	}
	if mc.closeCalls.Load() != 1 {
		t.Errorf("consumer Close() called %d times, want 1", mc.closeCalls.Load())
	}
	if !mp.closed.Load() {
		t.Error("producer should have been closed")
	}
}

func TestComponent_StopNotRunning(t *testing.T) {
	log := logger.New(&logger.Config{Level: "error"}, "test")
	comp := NewComponent(Config{}, log)
	if err := comp.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() on not-running component should not error: %v", err)
	}
}

func TestComponent_AddConsumer_WhileRunning(t *testing.T) {
	log := logger.New(&logger.Config{Level: "error"}, "test")
	comp := NewComponent(Config{}, log)

	ctx := context.Background()
	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	mc := &mockConsumer{topic: "late-join"}
	comp.AddConsumer(mc)

	// Stop should wait for the late consumer too
	if err := comp.Stop(ctx); err != nil {
		t.Fatalf("Stop() error: %v", err)
	}
	if !mc.consumed.Load() {
		t.Error("late-joined consumer should have been consumed")
	}
}

func TestComponent_Health_NotRunning(t *testing.T) {
	log := logger.New(&logger.Config{Level: "error"}, "test")
	comp := NewComponent(Config{}, log)
	health := comp.Health(context.Background())
	if health.Status != component.StatusUnhealthy {
		t.Errorf("Health().Status = %q, want unhealthy", health.Status)
	}
}

func TestComponent_Interface(t *testing.T) {
	var _ component.Component = (*Component)(nil)
}
