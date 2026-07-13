package consumer

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
		t.Fatalf("register kafka consumer: %v", err)
	}
	if got := reg.ConsumerAdapters(); len(got) != 1 || got[0] != "kafka" {
		t.Fatalf("consumer adapters = %v, want [kafka]", got)
	}
	if got := reg.ProducerAdapters(); len(got) != 0 {
		t.Fatalf("producer adapters = %v, want []", got)
	}

	consumer, err := reg.NewConsumer(context.Background(), messaging.Config{Adapter: "kafka", ConsumerGroup: "test-group"}, nil, "events")
	if err != nil {
		t.Fatalf("new kafka consumer: %v", err)
	}
	if consumer.Topic() != "events" {
		t.Fatalf("consumer topic = %q, want events", consumer.Topic())
	}
	kafkaConsumer, ok := consumer.(*Consumer)
	if !ok {
		t.Fatalf("consumer type = %T, want *Consumer", consumer)
	}
	if kafkaConsumer.GroupID() != "test-group" {
		t.Fatalf("consumer group = %q, want common consumer_group", kafkaConsumer.GroupID())
	}
	if kafkaConsumer.commitStrategy != messaging.CommitAfterHandlerSuccess {
		t.Fatalf("commit strategy = %q, want common default", kafkaConsumer.commitStrategy)
	}
	if err := consumer.Close(); err != nil {
		t.Fatalf("close kafka consumer: %v", err)
	}
}

func TestRegisterRejectsNilRegistry(t *testing.T) {
	t.Parallel()

	if err := Register(nil, kafka.Config{}); err == nil {
		t.Fatal("expected nil registry error")
	}
}

func TestRegisterRejectsUnsupportedMaxInFlight(t *testing.T) {
	t.Parallel()

	reg := messaging.NewRegistry()
	if err := Register(reg, kafka.Config{Brokers: []string{"127.0.0.1:1"}}); err != nil {
		t.Fatalf("register kafka consumer: %v", err)
	}
	_, err := reg.NewConsumer(context.Background(), messaging.Config{Adapter: "kafka", ConsumerGroup: "test-group", MaxInFlight: 2}, nil, "events")
	if err == nil {
		t.Fatal("expected max_in_flight unsupported error")
	}
}
