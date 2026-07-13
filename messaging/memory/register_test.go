package memory

import (
	"context"
	"reflect"
	"testing"

	"github.com/kbukum/gokit/messaging"
)

func TestRegisterMemoryAdapterConstructsFactories(t *testing.T) {
	t.Parallel()

	reg := messaging.NewRegistry()
	broker := NewBroker()
	if err := Register(reg, Config{Broker: broker}); err != nil {
		t.Fatalf("register memory: %v", err)
	}

	cfg := messaging.Config{Adapter: "memory"}
	producer, err := reg.NewProducer(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("new producer: %v", err)
	}
	consumer, err := reg.NewConsumer(context.Background(), cfg, nil, "events")
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
		Adapter: "memory",
		DLQ:     messaging.DLQPolicy{Enabled: true},
	}, nil)
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

func TestConfigValidateRejectsNonPositiveBuffer(t *testing.T) {
	t.Parallel()
	if err := (Config{BufferSize: 0}).Validate(); err == nil {
		t.Fatal("expected buffer_size error for zero")
	}
	if err := (Config{BufferSize: -1}).Validate(); err == nil {
		t.Fatal("expected buffer_size error for negative")
	}
}

func TestBrokerFromConfigDefaultsAndCustomBuffer(t *testing.T) {
	t.Parallel()
	reg := messaging.NewRegistry()
	if err := Register(reg, Config{BufferSize: 4}); err != nil {
		t.Fatalf("register with custom buffer: %v", err)
	}
	producer, err := reg.NewProducer(context.Background(), messaging.Config{Adapter: "memory"}, nil)
	if err != nil {
		t.Fatalf("new producer: %v", err)
	}
	if err := producer.PublishBinary(context.Background(), "events", "k", []byte("v")); err != nil {
		t.Fatalf("publish: %v", err)
	}
}

func TestValidateCommonConsumerRejectsUnsupportedSettings(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		cfg  messaging.Config
	}{
		{"max-in-flight", messaging.Config{MaxInFlight: 2, DeliveryGuarantee: messaging.DeliveryAtMostOnce, CommitStrategy: messaging.CommitAuto}},
		{"at-least-once-wrong-commit", messaging.Config{MaxInFlight: 1, DeliveryGuarantee: messaging.DeliveryAtLeastOnce, CommitStrategy: messaging.CommitAuto}},
		{"at-most-once-wrong-commit", messaging.Config{MaxInFlight: 1, DeliveryGuarantee: messaging.DeliveryAtMostOnce, CommitStrategy: messaging.CommitAfterHandlerSuccess}},
		{"exactly-once", messaging.Config{MaxInFlight: 1, DeliveryGuarantee: messaging.DeliveryExactlyOnce, CommitStrategy: messaging.CommitAuto}},
		{"dlq", messaging.Config{MaxInFlight: 1, DeliveryGuarantee: messaging.DeliveryAtMostOnce, CommitStrategy: messaging.CommitAuto, DLQ: messaging.DLQPolicy{Enabled: true}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			reg := messaging.NewRegistry()
			if err := Register(reg); err != nil {
				t.Fatalf("register: %v", err)
			}
			cfg := tc.cfg
			cfg.Adapter = "memory"
			if _, err := reg.NewConsumer(context.Background(), cfg, nil, "events"); err == nil {
				t.Fatalf("expected consumer rejection for %s", tc.name)
			}
		})
	}
}

func TestValidateCommonConsumerAcceptsAtLeastOnce(t *testing.T) {
	t.Parallel()
	reg := messaging.NewRegistry()
	if err := Register(reg); err != nil {
		t.Fatalf("register: %v", err)
	}
	cfg := messaging.Config{
		Adapter:           "memory",
		MaxInFlight:       1,
		DeliveryGuarantee: messaging.DeliveryAtLeastOnce,
		CommitStrategy:    messaging.CommitAfterHandlerSuccess,
	}
	if _, err := reg.NewConsumer(context.Background(), cfg, nil, "events"); err != nil {
		t.Fatalf("at-least-once consumer rejected: %v", err)
	}
}
