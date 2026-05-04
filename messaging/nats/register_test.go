package nats

import (
	"context"
	"errors"
	"testing"

	"github.com/kbukum/gokit/messaging"
)

func TestRegisterIsExplicitConfigFreeLazyAndConstructs(t *testing.T) {
	t.Parallel()

	reg := messaging.NewRegistry()
	if err := Register(reg); err != nil {
		t.Fatalf("register nats: %v", err)
	}
	if got := reg.ProducerAdapters(); len(got) != 1 || got[0] != "nats" {
		t.Fatalf("producer adapters = %v, want [nats]", got)
	}
	if got := reg.ConsumerAdapters(); len(got) != 1 || got[0] != "nats" {
		t.Fatalf("consumer adapters = %v, want [nats]", got)
	}

	cfg := &Config{URL: "nats://127.0.0.1:1", AllowInsecureDev: true}
	producer, err := reg.NewProducer(context.Background(), messaging.Config{Adapter: "nats", DeliveryGuarantee: messaging.DeliveryAtMostOnce, CommitStrategy: messaging.CommitAuto}, cfg, nil)
	if err != nil {
		t.Fatalf("new nats producer: %v", err)
	}
	natsProducer, ok := producer.(*Producer)
	if !ok {
		t.Fatalf("producer type = %T, want *Producer", producer)
	}
	if natsProducer.cfg.PublishTimeout != messaging.DefaultRequestTimeout {
		t.Fatalf("publish timeout = %q, want common request_timeout", natsProducer.cfg.PublishTimeout)
	}
	if closeErr := producer.(interface{ Close() error }).Close(); closeErr != nil {
		t.Fatalf("close nats producer: %v", closeErr)
	}
	consumer, err := reg.NewConsumer(context.Background(), messaging.Config{Adapter: "nats", DeliveryGuarantee: messaging.DeliveryAtMostOnce, CommitStrategy: messaging.CommitAuto, ConsumerGroup: "workers"}, cfg, nil, "events")
	if err != nil {
		t.Fatalf("new nats consumer: %v", err)
	}
	if consumer.Topic() != "events" {
		t.Fatalf("consumer topic = %q, want events", consumer.Topic())
	}
	natsConsumer, ok := consumer.(*Consumer)
	if !ok {
		t.Fatalf("consumer type = %T, want *Consumer", consumer)
	}
	if natsConsumer.cfg.QueueGroup != "workers" {
		t.Fatalf("queue group = %q, want common consumer_group", natsConsumer.cfg.QueueGroup)
	}
	if err := consumer.Close(); err != nil {
		t.Fatalf("close nats consumer: %v", err)
	}
}

func TestProducerReturnsClosedErrorAfterClose(t *testing.T) {
	t.Parallel()

	producer, err := NewProducer(Config{URL: "nats://127.0.0.1:1", AllowInsecureDev: true})
	if err != nil {
		t.Fatalf("new nats producer: %v", err)
	}
	if closeErr := producer.Close(); closeErr != nil {
		t.Fatalf("close nats producer: %v", closeErr)
	}

	err = producer.PublishBinary(context.Background(), "events", "", []byte("payload"))
	if !errors.Is(err, messaging.ErrClosed) {
		t.Fatalf("publish after close error = %v, want ErrClosed", err)
	}
}

func TestProducerRejectsInvalidTopicBeforeConnect(t *testing.T) {
	t.Parallel()

	producer, err := NewProducer(Config{URL: "nats://127.0.0.1:1", AllowInsecureDev: true})
	if err != nil {
		t.Fatalf("new nats producer: %v", err)
	}
	err = producer.PublishBinary(context.Background(), "bad topic", "", []byte("payload"))
	if err == nil {
		t.Fatal("expected invalid topic error")
	}
}

func TestConsumerReturnsClosedErrorAfterClose(t *testing.T) {
	t.Parallel()

	consumer, err := NewConsumer(Config{URL: "nats://127.0.0.1:1", AllowInsecureDev: true}, "events")
	if err != nil {
		t.Fatalf("new nats consumer: %v", err)
	}
	if closeErr := consumer.Close(); closeErr != nil {
		t.Fatalf("close nats consumer: %v", closeErr)
	}

	err = consumer.Consume(context.Background(), func(context.Context, messaging.Message) error {
		return nil
	})
	if !errors.Is(err, messaging.ErrClosed) {
		t.Fatalf("consume after close error = %v, want ErrClosed", err)
	}
}

func TestRegisterRejectsNilRegistry(t *testing.T) {
	t.Parallel()

	if err := Register(nil); err == nil {
		t.Fatal("expected nil registry error")
	}
}

func TestFactoryRejectsWrongConfigType(t *testing.T) {
	t.Parallel()

	reg := messaging.NewRegistry()
	if err := Register(reg); err != nil {
		t.Fatalf("register nats: %v", err)
	}
	_, err := reg.NewProducer(context.Background(), messaging.Config{Adapter: "nats", DeliveryGuarantee: messaging.DeliveryAtMostOnce, CommitStrategy: messaging.CommitAuto}, struct{}{}, nil)
	if err == nil {
		t.Fatal("expected config type error")
	}
}

func TestRegisterRejectsUnsupportedCommonConfig(t *testing.T) {
	t.Parallel()

	reg := messaging.NewRegistry()
	if err := Register(reg); err != nil {
		t.Fatalf("register nats: %v", err)
	}
	_, err := reg.NewProducer(context.Background(), messaging.Config{Adapter: "nats"}, &Config{URL: "nats://127.0.0.1:1", AllowInsecureDev: true}, nil)
	if err == nil {
		t.Fatal("expected unsupported delivery guarantee error")
	}
}

func TestRegisterRejectsAdapterManagedDLQ(t *testing.T) {
	t.Parallel()

	reg := messaging.NewRegistry()
	if err := Register(reg); err != nil {
		t.Fatalf("register nats: %v", err)
	}
	_, err := reg.NewProducer(context.Background(), messaging.Config{
		Adapter:           "nats",
		DeliveryGuarantee: messaging.DeliveryAtMostOnce,
		CommitStrategy:    messaging.CommitAuto,
		DLQ:               messaging.DLQPolicy{Enabled: true},
	}, &Config{URL: "nats://127.0.0.1:1", AllowInsecureDev: true}, nil)
	if err == nil {
		t.Fatal("expected adapter-managed DLQ error")
	}
}
