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
func (registryProducer) Send(context.Context, Message) error                         { return nil }
func (registryProducer) SendBatch(context.Context, []Message) error                  { return nil }
func (registryProducer) Flush(context.Context) error                                 { return nil }

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
	_, err := reg.NewProducer(context.Background(), Config{Adapter: "kafka"}, logging.NewDefault("registry-test"))
	if err == nil {
		t.Fatal("expected unregistered producer adapter error")
	}
}

func TestRegistryConstructsWithCapturedTypedConfig(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	producerCfg := struct{ Name string }{Name: "producer-a"}
	consumerCfg := struct{ Name string }{Name: "consumer-a"}
	if err := reg.RegisterProducer("custom", func(_ context.Context, cfg Config, log *logging.Logger) (Producer, error) {
		if cfg.Adapter != "custom" {
			t.Fatalf("cfg.Adapter = %q, want custom", cfg.Adapter)
		}
		if producerCfg.Name != "producer-a" {
			t.Fatalf("producer cfg not captured")
		}
		if log == nil {
			t.Fatal("log is nil")
		}
		return registryProducer{}, nil
	}); err != nil {
		t.Fatalf("register producer: %v", err)
	}
	if err := reg.RegisterConsumer("custom", func(_ context.Context, cfg Config, log *logging.Logger, topic string) (Consumer, error) {
		if cfg.Adapter != "custom" {
			t.Fatalf("cfg.Adapter = %q, want custom", cfg.Adapter)
		}
		if consumerCfg.Name != "consumer-a" {
			t.Fatalf("consumer cfg not captured")
		}
		if log == nil {
			t.Fatal("log is nil")
		}
		return registryConsumer{topic: topic}, nil
	}); err != nil {
		t.Fatalf("register consumer: %v", err)
	}
	if _, err := reg.NewProducer(context.Background(), Config{Adapter: "custom"}, nil); err != nil {
		t.Fatalf("new producer: %v", err)
	}
	consumer, err := reg.NewConsumer(context.Background(), Config{Adapter: "custom"}, nil, "events")
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
	factory := func(context.Context, Config, *logging.Logger) (Producer, error) { return registryProducer{}, nil }
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
	if err := reg.RegisterProducer("custom", func(context.Context, Config, *logging.Logger) (Producer, error) { return registryProducer{}, nil }); err != nil {
		t.Fatalf("register producer: %v", err)
	}
	producer, err := reg.NewProducer(context.Background(), Config{Adapter: "custom", Topics: []string{"events"}}, nil)
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

func TestRegistryTopicRestrictedProducerGatesEveryMethod(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	if err := reg.RegisterProducer("custom", func(context.Context, Config, *logging.Logger) (Producer, error) {
		return registryProducer{}, nil
	}); err != nil {
		t.Fatalf("register producer: %v", err)
	}
	producer, err := reg.NewProducer(context.Background(), Config{Adapter: "custom", Topics: []string{"events"}}, nil)
	if err != nil {
		t.Fatalf("new producer: %v", err)
	}
	ctx := context.Background()
	event := Event{ID: "1", Type: "t", Source: "s"}

	cases := []struct {
		name string
		call func(topic string) error
	}{
		{"Send", func(topic string) error { return producer.Send(ctx, Message{Topic: topic}) }},
		{"SendBatch", func(topic string) error { return producer.SendBatch(ctx, []Message{{Topic: topic}}) }},
		{"Publish", func(topic string) error { return producer.Publish(ctx, topic, event) }},
		{"PublishJSON", func(topic string) error { return producer.PublishJSON(ctx, topic, "k", map[string]int{"n": 1}) }},
		{"PublishBinary", func(topic string) error { return producer.PublishBinary(ctx, topic, "k", []byte("v")) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := tc.call("events"); err != nil {
				t.Fatalf("configured topic rejected: %v", err)
			}
			if err := tc.call("other"); err == nil {
				t.Fatalf("expected unconfigured topic error")
			}
		})
	}
}

func TestRegistryRejectsEmptyAdapterAndNilFactory(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	if err := reg.RegisterProducer("", func(context.Context, Config, *logging.Logger) (Producer, error) {
		return registryProducer{}, nil
	}); err == nil {
		t.Fatal("expected empty producer adapter error")
	}
	if err := reg.RegisterProducer("custom", nil); err == nil {
		t.Fatal("expected nil producer factory error")
	}
	if err := reg.RegisterConsumer("", func(context.Context, Config, *logging.Logger, string) (Consumer, error) {
		return registryConsumer{}, nil
	}); err == nil {
		t.Fatal("expected empty consumer adapter error")
	}
	if err := reg.RegisterConsumer("custom", nil); err == nil {
		t.Fatal("expected nil consumer factory error")
	}
}

func TestRegistryRejectsDuplicateConsumerAdapters(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	factory := func(context.Context, Config, *logging.Logger, string) (Consumer, error) {
		return registryConsumer{}, nil
	}
	if err := reg.RegisterConsumer("memory", factory); err != nil {
		t.Fatalf("register consumer: %v", err)
	}
	if err := reg.RegisterConsumer("memory", factory); err == nil {
		t.Fatal("expected duplicate consumer registration error")
	}
}

func TestRegistryRejectsInvalidAndDisabledConfig(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	if err := reg.RegisterProducer("custom", func(context.Context, Config, *logging.Logger) (Producer, error) {
		return registryProducer{}, nil
	}); err != nil {
		t.Fatalf("register producer: %v", err)
	}
	if err := reg.RegisterConsumer("custom", func(_ context.Context, _ Config, _ *logging.Logger, topic string) (Consumer, error) {
		return registryConsumer{topic: topic}, nil
	}); err != nil {
		t.Fatalf("register consumer: %v", err)
	}

	if _, err := reg.NewProducer(context.Background(), Config{Adapter: "bad adapter"}, nil); err == nil {
		t.Fatal("expected invalid producer config error")
	}
	if _, err := reg.NewConsumer(context.Background(), Config{Adapter: "bad adapter"}, nil, "events"); err == nil {
		t.Fatal("expected invalid consumer config error")
	}

	disabled := false
	if _, err := reg.NewProducer(context.Background(), Config{Adapter: "custom", Enabled: &disabled}, nil); err == nil {
		t.Fatal("expected disabled producer error")
	}
	if _, err := reg.NewConsumer(context.Background(), Config{Adapter: "custom", Enabled: &disabled}, nil, "events"); err == nil {
		t.Fatal("expected disabled consumer error")
	}

	if _, err := reg.NewConsumer(context.Background(), Config{Adapter: "custom"}, nil, "bad topic"); err == nil {
		t.Fatal("expected invalid topic error")
	}
	if _, err := reg.NewConsumer(context.Background(), Config{Adapter: "unregistered"}, nil, "events"); err == nil {
		t.Fatal("expected unregistered consumer adapter error")
	}
}

func TestRegistryHonorsConfiguredConsumerSubscriptions(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	if err := reg.RegisterConsumer("custom", func(_ context.Context, _ Config, _ *logging.Logger, topic string) (Consumer, error) {
		return registryConsumer{topic: topic}, nil
	}); err != nil {
		t.Fatalf("register consumer: %v", err)
	}
	if _, err := reg.NewConsumer(context.Background(), Config{Adapter: "custom", Subscriptions: []string{"events"}}, nil, "events"); err != nil {
		t.Fatalf("configured subscription rejected: %v", err)
	}
	if _, err := reg.NewConsumer(context.Background(), Config{Adapter: "custom", Subscriptions: []string{"events"}}, nil, "other"); err == nil {
		t.Fatal("expected unconfigured subscription error")
	}
}
