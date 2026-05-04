package memory

import (
	"context"
	"reflect"
	"testing"

	"github.com/kbukum/gokit/messaging"
)

func TestRegisterMemoryBackendConstructsFactories(t *testing.T) {
	t.Parallel()

	reg := messaging.NewRegistry()
	broker := NewBroker()
	if err := Register(reg); err != nil {
		t.Fatalf("register memory: %v", err)
	}

	cfg := messaging.Config{Backend: "memory"}
	providerCfg := &Config{Broker: broker}
	producer, err := reg.NewProducer(context.Background(), cfg, providerCfg, nil)
	if err != nil {
		t.Fatalf("new producer: %v", err)
	}
	consumer, err := reg.NewConsumer(context.Background(), cfg, providerCfg, nil, "events")
	if err != nil {
		t.Fatalf("new consumer: %v", err)
	}

	if err := producer.PublishBinary(context.Background(), "events", "k", []byte("v")); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if consumer.Topic() != "events" {
		t.Fatalf("consumer topic = %q, want events", consumer.Topic())
	}
	if broker.MessageCount("events") != 1 {
		t.Fatalf("message count = %d, want 1", broker.MessageCount("events"))
	}
}

func TestRegisterRejectsNilRegistry(t *testing.T) {
	t.Parallel()

	if err := Register(nil); err == nil {
		t.Fatal("expected nil registry error")
	}
}

func TestRegisterRejectsAdapterManagedDLQ(t *testing.T) {
	t.Parallel()

	reg := messaging.NewRegistry()
	if err := Register(reg); err != nil {
		t.Fatalf("register memory: %v", err)
	}
	_, err := reg.NewProducer(context.Background(), messaging.Config{
		Backend: "memory",
		DLQ:     messaging.DLQPolicy{Enabled: true},
	}, &Config{}, nil)
	if err == nil {
		t.Fatal("expected adapter-managed DLQ error")
	}
}

func TestConfigDoesNotExposeCoreNameEnabled(t *testing.T) {
	t.Parallel()

	typ := reflect.TypeOf(Config{})
	for _, name := range []string{"Name", "Enabled"} {
		if _, ok := typ.FieldByName(name); ok {
			t.Fatalf("memory.Config exposes core field %s", name)
		}
	}
}
