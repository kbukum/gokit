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
	cfg := &Config{URL: "amqp://127.0.0.1:1/", AllowInsecureDev: true}
	if err := Register(reg, *cfg); err != nil {
		t.Fatalf("register rabbitmq: %v", err)
	}
	if got := reg.ConsumerAdapters(); len(got) != 1 || got[0] != "rabbitmq" {
		t.Fatalf("consumer adapters = %v, want [rabbitmq]", got)
	}
	if got := reg.ProducerAdapters(); len(got) != 1 || got[0] != "rabbitmq" {
		t.Fatalf("producer adapters = %v, want [rabbitmq]", got)
	}

	producer, err := reg.NewProducer(context.Background(), messaging.Config{Adapter: "rabbitmq"}, nil)
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
	consumer, err := reg.NewConsumer(context.Background(), messaging.Config{Adapter: "rabbitmq", ConsumerGroup: "workers"}, nil, "events")
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

func TestRegisterRejectsUnsupportedCommonConfig(t *testing.T) {
	t.Parallel()
	reg := messaging.NewRegistry()
	if err := Register(reg, Config{URL: "amqp://127.0.0.1:1/", AllowInsecureDev: true}); err != nil {
		t.Fatalf("register rabbitmq: %v", err)
	}
	_, err := reg.NewProducer(context.Background(), messaging.Config{Adapter: "rabbitmq", DeliveryGuarantee: messaging.DeliveryExactlyOnce}, nil)
	if err == nil {
		t.Fatal("expected unsupported delivery guarantee error")
	}
}

func TestRegisterRejectsAdapterManagedDLQ(t *testing.T) {
	t.Parallel()
	reg := messaging.NewRegistry()
	if err := Register(reg, Config{URL: "amqp://127.0.0.1:1/", AllowInsecureDev: true}); err != nil {
		t.Fatalf("register rabbitmq: %v", err)
	}
	_, err := reg.NewProducer(context.Background(), messaging.Config{Adapter: "rabbitmq", DLQ: messaging.DLQPolicy{Enabled: true}}, nil)
	if err == nil {
		t.Fatal("expected adapter-managed DLQ error")
	}
}

func TestConsumerRejectsUnsupportedCommonConfig(t *testing.T) {
	t.Parallel()
	reg := messaging.NewRegistry()
	if err := Register(reg, Config{URL: "amqp://127.0.0.1:1/", AllowInsecureDev: true}); err != nil {
		t.Fatalf("register rabbitmq: %v", err)
	}
	base := messaging.Config{Adapter: "rabbitmq", DeliveryGuarantee: messaging.DeliveryAtMostOnce, CommitStrategy: messaging.CommitAuto, MaxInFlight: 1}

	cases := map[string]func(c messaging.Config) messaging.Config{
		"at-least-once-commit": func(c messaging.Config) messaging.Config {
			c.DeliveryGuarantee = messaging.DeliveryAtLeastOnce
			c.CommitStrategy = messaging.CommitAuto
			return c
		},
		"at-most-once-commit": func(c messaging.Config) messaging.Config {
			c.CommitStrategy = messaging.CommitAfterHandlerSuccess
			return c
		},
		"exactly-once": func(c messaging.Config) messaging.Config {
			c.DeliveryGuarantee = messaging.DeliveryExactlyOnce
			return c
		},
		"dlq": func(c messaging.Config) messaging.Config { c.DLQ = messaging.DLQPolicy{Enabled: true}; return c },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := reg.NewConsumer(context.Background(), mutate(base), nil, "events"); err == nil {
				t.Fatalf("expected consumer rejection for %s", name)
			}
		})
	}
}

func TestConsumerRejectsPrefetchMismatch(t *testing.T) {
	t.Parallel()
	reg := messaging.NewRegistry()
	if err := Register(reg, Config{URL: "amqp://127.0.0.1:1/", AllowInsecureDev: true, PrefetchCount: 5}); err != nil {
		t.Fatalf("register rabbitmq: %v", err)
	}
	_, err := reg.NewConsumer(context.Background(), messaging.Config{
		Adapter: "rabbitmq", DeliveryGuarantee: messaging.DeliveryAtMostOnce, CommitStrategy: messaging.CommitAuto,
		MaxInFlight: 1,
	}, nil, "events")
	if err == nil {
		t.Fatal("expected prefetch_count mismatch error")
	}
}

func TestConsumerRejectsQueueNameMismatch(t *testing.T) {
	t.Parallel()
	reg := messaging.NewRegistry()
	if err := Register(reg, Config{URL: "amqp://127.0.0.1:1/", AllowInsecureDev: true, QueueName: "preset"}); err != nil {
		t.Fatalf("register rabbitmq: %v", err)
	}
	_, err := reg.NewConsumer(context.Background(), messaging.Config{
		Adapter: "rabbitmq", DeliveryGuarantee: messaging.DeliveryAtMostOnce, CommitStrategy: messaging.CommitAuto,
		MaxInFlight: 1, ConsumerGroup: "workers",
	}, nil, "events")
	if err == nil {
		t.Fatal("expected queue_name mismatch error")
	}
}
