package producer

import (
	"context"
	"testing"

	"github.com/kbukum/gokit/messaging"
	"github.com/kbukum/gokit/messaging/kafka"
)

func TestRegisterIsExplicitConfigFreeAndConstructs(t *testing.T) {
	t.Parallel()

	reg := messaging.NewRegistry()
	cfg := &kafka.Config{Brokers: []string{"127.0.0.1:1"}}
	if err := Register(reg, *cfg); err != nil {
		t.Fatalf("register kafka producer: %v", err)
	}
	if got := reg.ProducerAdapters(); len(got) != 1 || got[0] != "kafka" {
		t.Fatalf("producer adapters = %v, want [kafka]", got)
	}
	if got := reg.ConsumerAdapters(); len(got) != 0 {
		t.Fatalf("consumer adapters = %v, want []", got)
	}

	producer, err := reg.NewProducer(context.Background(), messaging.Config{Adapter: "kafka", Name: "events", RetryAttempts: 7}, nil)
	if err != nil {
		t.Fatalf("new kafka producer: %v", err)
	}
	kafkaProducer, ok := producer.(*Producer)
	if !ok {
		t.Fatalf("producer type = %T, want *Producer", producer)
	}
	if kafkaProducer.Name() != "events" {
		t.Fatalf("producer name = %q, want common name", kafkaProducer.Name())
	}
	if kafkaProducer.retryAttempts != 7 {
		t.Fatalf("retry attempts = %d, want common retry_attempts", kafkaProducer.retryAttempts)
	}
	if kafkaProducer.requestTimeout.String() != messaging.DefaultRequestTimeout {
		t.Fatalf("request timeout = %s, want common default", kafkaProducer.requestTimeout)
	}
	if err := producer.(interface{ Close() error }).Close(); err != nil {
		t.Fatalf("close kafka producer: %v", err)
	}
}

func TestRegisterRejectsNilRegistry(t *testing.T) {
	t.Parallel()

	if err := Register(nil, kafka.Config{}); err == nil {
		t.Fatal("expected nil registry error")
	}
}
