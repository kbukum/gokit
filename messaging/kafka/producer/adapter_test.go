package producer

import (
	"context"
	"errors"
	"testing"

	kafkago "github.com/segmentio/kafka-go"

	"github.com/kbukum/gokit/logger"
	"github.com/kbukum/gokit/messaging"
	"github.com/kbukum/gokit/messaging/kafka"
)

func TestProducer_Name(t *testing.T) {
	p := &Producer{name: "my-producer"}
	if got := p.Name(); got != "my-producer" {
		t.Errorf("Name() = %q, want my-producer", got)
	}
}

func TestProducer_Name_DefaultFromConstructor(t *testing.T) {
	log := logger.New(&logger.Config{Level: "error"}, "test")
	p, err := NewLazyProducer(messaging.Config{Adapter: "kafka"}, kafka.Config{Brokers: []string{"localhost:9092"}}, log)
	if err != nil {
		t.Fatalf("NewLazyProducer() error: %v", err)
	}
	if got := p.Name(); got != defaultProviderName {
		t.Errorf("Name() = %q, want %q", got, defaultProviderName)
	}
}

func TestProducer_IsAvailable_Open(t *testing.T) {
	p := &Producer{}
	if !p.IsAvailable(context.Background()) {
		t.Error("expected IsAvailable=true for open producer")
	}
}

func TestProducer_IsAvailable_Closed(t *testing.T) {
	p := &Producer{closed: true}
	if p.IsAvailable(context.Background()) {
		t.Error("expected IsAvailable=false for closed producer")
	}
}

func TestNewLazyProducer_Valid(t *testing.T) {
	log := logger.New(&logger.Config{Level: "error"}, "test")
	cfg := kafka.Config{Brokers: []string{"localhost:9092"}}
	p, err := NewLazyProducer(messaging.Config{Adapter: "kafka", Name: "events"}, cfg, log)
	if err != nil {
		t.Fatalf("NewLazyProducer() error: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil producer")
	}
	if p.Name() != "events" {
		t.Fatalf("Name() = %q, want events", p.Name())
	}
	if p.retryAttempts != messaging.DefaultRetryAttempts {
		t.Fatalf("retry attempts = %d, want common default", p.retryAttempts)
	}
	if p.writer != nil {
		t.Error("lazy producer should not have writer before first use")
	}
}

func TestNewLazyProducer_DisabledCommonConfig(t *testing.T) {
	log := logger.New(&logger.Config{Level: "error"}, "test")
	enabled := false
	_, err := NewLazyProducer(messaging.Config{Adapter: "kafka", Enabled: &enabled}, kafka.Config{}, log)
	if err == nil {
		t.Error("expected error when common messaging is disabled")
	}
}

func TestNewLazyProducer_InvalidConfig(t *testing.T) {
	log := logger.New(&logger.Config{Level: "error"}, "test")
	cfg := kafka.Config{
		Brokers:      []string{"localhost:9092"},
		BatchTimeout: "not-a-duration",
	}
	_, err := NewLazyProducer(messaging.Config{Adapter: "kafka"}, cfg, log)
	if err == nil {
		t.Error("expected error for invalid config")
	}
}

func TestProducer_Close_NilWriter(t *testing.T) {
	log := logger.New(&logger.Config{Level: "error"}, "test")
	p := &Producer{cfg: kafka.Config{}, log: log.WithComponent("test")}
	if err := p.Close(); err != nil {
		t.Fatalf("Close() error on nil writer: %v", err)
	}
	if !p.closed {
		t.Error("expected closed=true after Close()")
	}
}

func TestProducer_Close_Idempotent(t *testing.T) {
	log := logger.New(&logger.Config{Level: "error"}, "test")
	p := &Producer{cfg: kafka.Config{}, log: log.WithComponent("test")}
	_ = p.Close()
	if err := p.Close(); err != nil {
		t.Fatalf("second Close() should be no-op: %v", err)
	}
}

func TestProducer_Stats_NilWriter(t *testing.T) {
	p := &Producer{}
	stats := p.Stats()
	if stats.Writes != 0 {
		t.Errorf("Stats().Writes = %d, want 0", stats.Writes)
	}
}

func TestProducer_WriteMessages_Closed(t *testing.T) {
	log := logger.New(&logger.Config{Level: "error"}, "test")
	p, err := NewLazyProducer(messaging.Config{Adapter: "kafka"}, kafka.Config{}, log)
	if err != nil {
		t.Fatal(err)
	}
	_ = p.Close()
	err = p.WriteMessages(context.Background())
	if !errors.Is(err, messaging.ErrClosed) {
		t.Fatalf("expected ErrClosed writing to closed producer, got %v", err)
	}
}

func TestProducer_FlushNoOpRespectsContextAndClosedState(t *testing.T) {
	p := &Producer{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := p.Flush(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Flush(canceled) error = %v, want context.Canceled", err)
	}

	if err := p.Flush(context.Background()); err != nil {
		t.Fatalf("Flush() on open producer: %v", err)
	}
	if p.writer != nil {
		t.Fatal("Flush() should not initialize kafka writer")
	}

	p.closed = true
	if err := p.Flush(context.Background()); !errors.Is(err, messaging.ErrClosed) {
		t.Fatalf("Flush(closed) error = %v, want ErrClosed", err)
	}
}

func TestWriteMessagesRejectsInvalidTopicBeforeInit(t *testing.T) {
	p, err := NewLazyProducer(messaging.Config{}, kafka.Config{AllowInsecureDev: true}, nil)
	if err != nil {
		t.Fatalf("new lazy producer: %v", err)
	}
	err = p.WriteMessages(context.Background(), kafkago.Message{Topic: "bad topic", Value: []byte("payload")})
	if err == nil {
		t.Fatal("expected invalid topic error")
	}
}
