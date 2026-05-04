package rabbitmq

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
		t.Fatalf("register rabbitmq: %v", err)
	}
	if got := reg.ConsumerBackends(); len(got) != 1 || got[0] != "rabbitmq" {
		t.Fatalf("consumer backends = %v, want [rabbitmq]", got)
	}
	if got := reg.ProducerBackends(); len(got) != 1 || got[0] != "rabbitmq" {
		t.Fatalf("producer backends = %v, want [rabbitmq]", got)
	}

	cfg := &Config{URL: "amqp://127.0.0.1:1/", AllowInsecureDev: true}
	producer, err := reg.NewProducer(context.Background(), messaging.Config{Backend: "rabbitmq"}, cfg, nil)
	if err != nil {
		t.Fatalf("new rabbitmq producer: %v", err)
	}
	rabbitProducer, ok := producer.(*Producer)
	if !ok {
		t.Fatalf("producer type = %T, want *Producer", producer)
	}
	if rabbitProducer.cfg.PublishTimeout != messaging.DefaultRequestTimeout {
		t.Fatalf("publish timeout = %q, want common request_timeout", rabbitProducer.cfg.PublishTimeout)
	}
	if closeErr := producer.(interface{ Close() error }).Close(); closeErr != nil {
		t.Fatalf("close rabbitmq producer: %v", closeErr)
	}
	consumer, err := reg.NewConsumer(context.Background(), messaging.Config{Backend: "rabbitmq", ConsumerGroup: "workers"}, cfg, nil, "events")
	if err != nil {
		t.Fatalf("new rabbitmq consumer: %v", err)
	}
	if consumer.Topic() != "events" {
		t.Fatalf("consumer topic = %q, want events", consumer.Topic())
	}
	rabbitConsumer, ok := consumer.(*Consumer)
	if !ok {
		t.Fatalf("consumer type = %T, want *Consumer", consumer)
	}
	if rabbitConsumer.cfg.QueueName != "workers" {
		t.Fatalf("queue name = %q, want common consumer_group", rabbitConsumer.cfg.QueueName)
	}
	if err := consumer.Close(); err != nil {
		t.Fatalf("close rabbitmq consumer: %v", err)
	}
}

func TestProducerReturnsClosedErrorAfterClose(t *testing.T) {
	t.Parallel()

	producer, err := NewProducer(Config{URL: "amqp://127.0.0.1:1/", AllowInsecureDev: true})
	if err != nil {
		t.Fatalf("new rabbitmq producer: %v", err)
	}
	if closeErr := producer.Close(); closeErr != nil {
		t.Fatalf("close rabbitmq producer: %v", closeErr)
	}

	err = producer.PublishBinary(context.Background(), "events", "", []byte("payload"))
	if !errors.Is(err, messaging.ErrClosed) {
		t.Fatalf("publish after close error = %v, want ErrClosed", err)
	}
}

func TestProducerRejectsInvalidTopicBeforeConnect(t *testing.T) {
	t.Parallel()

	producer, err := NewProducer(Config{URL: "amqp://127.0.0.1:1/", AllowInsecureDev: true})
	if err != nil {
		t.Fatalf("new rabbitmq producer: %v", err)
	}
	err = producer.PublishBinary(context.Background(), "bad topic", "", []byte("payload"))
	if err == nil {
		t.Fatal("expected invalid topic error")
	}
}

func TestConsumerReturnsClosedErrorAfterClose(t *testing.T) {
	t.Parallel()

	consumer, err := NewConsumer(Config{URL: "amqp://127.0.0.1:1/", AllowInsecureDev: true}, "events")
	if err != nil {
		t.Fatalf("new rabbitmq consumer: %v", err)
	}
	if closeErr := consumer.Close(); closeErr != nil {
		t.Fatalf("close rabbitmq consumer: %v", closeErr)
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
		t.Fatalf("register rabbitmq: %v", err)
	}
	_, err := reg.NewProducer(context.Background(), messaging.Config{Backend: "rabbitmq"}, struct{}{}, nil)
	if err == nil {
		t.Fatal("expected config type error")
	}
}

func TestRegisterRejectsUnsupportedExactlyOnce(t *testing.T) {
	t.Parallel()

	reg := messaging.NewRegistry()
	if err := Register(reg); err != nil {
		t.Fatalf("register rabbitmq: %v", err)
	}
	_, err := reg.NewProducer(context.Background(), messaging.Config{Backend: "rabbitmq", DeliveryGuarantee: messaging.DeliveryExactlyOnce}, &Config{URL: "amqp://127.0.0.1:1/", AllowInsecureDev: true}, nil)
	if err == nil {
		t.Fatal("expected exactly-once unsupported error")
	}
}

func TestRegisterRejectsAdapterManagedDLQ(t *testing.T) {
	t.Parallel()

	reg := messaging.NewRegistry()
	if err := Register(reg); err != nil {
		t.Fatalf("register rabbitmq: %v", err)
	}
	_, err := reg.NewProducer(context.Background(), messaging.Config{
		Backend: "rabbitmq",
		DLQ:     messaging.DLQPolicy{Enabled: true},
	}, &Config{URL: "amqp://127.0.0.1:1/", AllowInsecureDev: true}, nil)
	if err == nil {
		t.Fatal("expected adapter-managed DLQ error")
	}
}
