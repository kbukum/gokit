package messaging

import (
	"context"
	"testing"

	"github.com/kbukum/gokit/logging"
)

type registryProducer struct{}

func (registryProducer) Publish(context.Context, string, Event, ...string) error     { return nil }
func (registryProducer) PublishJSON(context.Context, string, string, any) error      { return nil }
func (registryProducer) PublishBinary(context.Context, string, string, []byte) error { return nil }
func (registryProducer) Close() error                                                { return nil }

func (registryProducer) Send(context.Context, Message) error        { return nil }
func (registryProducer) SendBatch(context.Context, []Message) error { return nil }
func (registryProducer) Flush(context.Context) error                { return nil }

type registryConsumer struct{ topic string }

func (c registryConsumer) Consume(context.Context, MessageHandler) error { return nil }
func (c registryConsumer) Topic() string                                 { return c.topic }
func (c registryConsumer) Close() error                                  { return nil }

func TestRegistryExplicitRegistration(t *testing.T) {
	t.Parallel()

	reg := NewRegistry()
	if got := reg.ProducerAdapters(); len(got) != 0 {
		t.Fatalf("new registry has producer adapters: %v", got)
	}
	if got := reg.ConsumerAdapters(); len(got) != 0 {
		t.Fatalf("new registry has consumer adapters: %v", got)
	}

	_, err := reg.NewProducer(context.Background(), Config{Adapter: "kafka"}, nil, logging.NewDefault("registry-test"))
	if err == nil {
		t.Fatal("expected unregistered producer adapter error")
	}
}

func TestRegistryConstructsWithRuntimeConfig(t *testing.T) {
	t.Parallel()

	reg := NewRegistry()
	producerCfg := struct{ Name string }{Name: "producer-a"}
	consumerCfg := struct{ Name string }{Name: "consumer-a"}
	if err := reg.RegisterProducer("custom", func(_ context.Context, cfg Config, adapterCfg any, log *logging.Logger) (Producer, error) {
		if cfg.Adapter != "custom" {
			t.Fatalf("cfg.Adapter = %q, want custom", cfg.Adapter)
		}
		if adapterCfg != &producerCfg {
			t.Fatalf("adapterCfg = %p, want %p", adapterCfg, &producerCfg)
		}
		if log == nil {
			t.Fatal("log is nil")
		}
		return registryProducer{}, nil
	}); err != nil {
		t.Fatalf("register producer: %v", err)
	}
	if err := reg.RegisterConsumer("custom", func(_ context.Context, cfg Config, adapterCfg any, log *logging.Logger, topic string) (Consumer, error) {
		if cfg.Adapter != "custom" {
			t.Fatalf("cfg.Adapter = %q, want custom", cfg.Adapter)
		}
		if adapterCfg != &consumerCfg {
			t.Fatalf("adapterCfg = %p, want %p", adapterCfg, &consumerCfg)
		}
		if log == nil {
			t.Fatal("log is nil")
		}
		return registryConsumer{topic: topic}, nil
	}); err != nil {
		t.Fatalf("register consumer: %v", err)
	}

	if _, err := reg.NewProducer(context.Background(), Config{Adapter: "custom"}, &producerCfg, nil); err != nil {
		t.Fatalf("new producer: %v", err)
	}
	consumer, err := reg.NewConsumer(context.Background(), Config{Adapter: "custom"}, &consumerCfg, nil, "events")
	if err != nil {
		t.Fatalf("new consumer: %v", err)
	}
	if consumer.Topic() != "events" {
		t.Fatalf("consumer topic = %q, want events", consumer.Topic())
	}
}

func TestRegistryRejectsDuplicateAdapters(t *testing.T) {
	t.Parallel()

	reg := NewRegistry()
	factory := func(context.Context, Config, any, *logging.Logger) (Producer, error) {
		return registryProducer{}, nil
	}
	if err := reg.RegisterProducer("memory", factory); err != nil {
		t.Fatalf("register producer: %v", err)
	}
	if err := reg.RegisterProducer("memory", factory); err == nil {
		t.Fatal("expected duplicate producer registration error")
	}
}

func TestRegistryHonorsConfiguredProducerTopics(t *testing.T) {
	t.Parallel()

	reg := NewRegistry()
	if err := reg.RegisterProducer("custom", func(context.Context, Config, any, *logging.Logger) (Producer, error) {
		return registryProducer{}, nil
	}); err != nil {
		t.Fatalf("register producer: %v", err)
	}
	producer, err := reg.NewProducer(context.Background(), Config{Adapter: "custom", Topics: []string{"events"}}, nil, nil)
	if err != nil {
		t.Fatalf("new producer: %v", err)
	}
	if err := producer.PublishBinary(context.Background(), "events", "", nil); err != nil {
		t.Fatalf("configured topic rejected: %v", err)
	}
	if err := producer.PublishBinary(context.Background(), "other", "", nil); err == nil {
		t.Fatal("expected unconfigured topic error")
	}
}

func TestRegistryHonorsConfiguredConsumerSubscriptions(t *testing.T) {
	t.Parallel()

	reg := NewRegistry()
	if err := reg.RegisterConsumer("custom", func(_ context.Context, _ Config, _ any, _ *logging.Logger, topic string) (Consumer, error) {
		return registryConsumer{topic: topic}, nil
	}); err != nil {
		t.Fatalf("register consumer: %v", err)
	}
	if _, err := reg.NewConsumer(context.Background(), Config{Adapter: "custom", Subscriptions: []string{"events"}}, nil, nil, "events"); err != nil {
		t.Fatalf("configured subscription rejected: %v", err)
	}
	if _, err := reg.NewConsumer(context.Background(), Config{Adapter: "custom", Subscriptions: []string{"events"}}, nil, nil, "other"); err == nil {
		t.Fatal("expected unconfigured subscription error")
	}
}
