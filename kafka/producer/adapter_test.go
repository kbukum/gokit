package producer

import (
	"context"
	"testing"

	"github.com/kbukum/gokit/kafka"
	"github.com/kbukum/gokit/logger"
)

func TestProducer_Name(t *testing.T) {
	p := &Producer{cfg: kafka.Config{Name: "my-producer"}}
	if got := p.Name(); got != "my-producer" {
		t.Errorf("Name() = %q, want my-producer", got)
	}
}

func TestProducer_Name_Empty(t *testing.T) {
	p := &Producer{cfg: kafka.Config{}}
	if got := p.Name(); got != "" {
		t.Errorf("Name() = %q, want empty", got)
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
	cfg := kafka.Config{
		Enabled: true,
		Brokers: []string{"localhost:9092"},
	}
	cfg.ApplyDefaults()
	p, err := NewLazyProducer(cfg, log)
	if err != nil {
		t.Fatalf("NewLazyProducer() error: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil producer")
	}
	// writer should NOT be initialized yet
	if p.writer != nil {
		t.Error("lazy producer should not have writer before first use")
	}
}

func TestNewLazyProducer_Disabled(t *testing.T) {
	log := logger.New(&logger.Config{Level: "error"}, "test")
	cfg := kafka.Config{Enabled: false}
	cfg.ApplyDefaults()
	_, err := NewLazyProducer(cfg, log)
	if err == nil {
		t.Error("expected error when kafka is disabled")
	}
}

func TestNewLazyProducer_InvalidConfig(t *testing.T) {
	log := logger.New(&logger.Config{Level: "error"}, "test")
	cfg := kafka.Config{
		Enabled:      true,
		Brokers:      []string{"localhost:9092"},
		BatchTimeout: "not-a-duration",
	}
	_, err := NewLazyProducer(cfg, log)
	if err == nil {
		t.Error("expected error for invalid config")
	}
}

func TestProducer_Close_NilWriter(t *testing.T) {
	log := logger.New(&logger.Config{Level: "error"}, "test")
	cfg := kafka.Config{Enabled: true}
	cfg.ApplyDefaults()
	p := &Producer{cfg: cfg, log: log.WithComponent("test")}
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
	cfg := kafka.Config{Enabled: true}
	cfg.ApplyDefaults()
	p, err := NewLazyProducer(cfg, log)
	if err != nil {
		t.Fatal(err)
	}
	_ = p.Close()
	err = p.WriteMessages(context.Background())
	if err == nil {
		t.Error("expected error writing to closed producer")
	}
}
