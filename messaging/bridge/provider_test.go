package bridge_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/kbukum/gokit/messaging"
	"github.com/kbukum/gokit/messaging/bridge"
	"github.com/kbukum/gokit/messaging/memory"
)

func TestProducerAsSink(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		topic string
		msg   messaging.Message
	}{
		{
			name:  "simple message",
			topic: "orders",
			msg: messaging.Message{
				Key:   "k1",
				Value: []byte("hello"),
			},
		},
		{
			name:  "empty value",
			topic: "events",
			msg: messaging.Message{
				Key:   "k2",
				Value: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			broker := memory.NewBroker()
			defer broker.Close()
			producer := broker.Producer()

			sink := bridge.ProducerAsSink("test-sink", producer, tt.topic)

			if sink.Name() != "test-sink" {
				t.Errorf("Name() = %q, want %q", sink.Name(), "test-sink")
			}
			if !sink.IsAvailable(context.Background()) {
				t.Error("IsAvailable() = false, want true")
			}

			if err := sink.Send(context.Background(), tt.msg); err != nil {
				t.Fatalf("Send() error: %v", err)
			}

			msgs := broker.Messages(tt.topic)
			if len(msgs) != 1 {
				t.Fatalf("expected 1 message on topic %q, got %d", tt.topic, len(msgs))
			}
			if !bytes.Equal(msgs[0].Value, tt.msg.Value) {
				t.Errorf("value = %q, want %q", msgs[0].Value, tt.msg.Value)
			}
		})
	}
}

func TestEventProducerAsSink(t *testing.T) {
	t.Parallel()

	broker := memory.NewBroker()
	defer broker.Close()
	producer := broker.Producer()

	sink := bridge.EventProducerAsSink("event-sink", producer, "domain-events")

	if sink.Name() != "event-sink" {
		t.Errorf("Name() = %q, want %q", sink.Name(), "event-sink")
	}

	event, err := messaging.NewEvent("user.created", "test", map[string]string{"id": "42"})
	if err != nil {
		t.Fatalf("NewEvent() error: %v", err)
	}

	if err := sink.Send(context.Background(), event); err != nil {
		t.Fatalf("Send() error: %v", err)
	}

	msgs := broker.Messages("domain-events")
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Headers["event-type"] != "user.created" {
		t.Errorf("event-type header = %q, want %q", msgs[0].Headers["event-type"], "user.created")
	}
}

func TestConsumerAsStream(t *testing.T) {
	t.Parallel()

	broker := memory.NewBroker()
	defer broker.Close()

	producer := broker.Producer()
	consumer := broker.Consumer("stream-topic")

	stream := bridge.ConsumerAsStream("test-stream", consumer)

	if stream.Name() != "test-stream" {
		t.Errorf("Name() = %q, want %q", stream.Name(), "test-stream")
	}
	if !stream.IsAvailable(context.Background()) {
		t.Error("IsAvailable() = false, want true")
	}

	// Publish messages before starting the iterator.
	for i := range 3 {
		_ = producer.PublishBinary(context.Background(), "stream-topic", "k", []byte{byte(i)})
	}

	iter, err := stream.Execute(context.Background(), struct{}{})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	defer func() { _ = iter.Close() }()

	for i := range 3 {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		msg, ok, err := iter.Next(ctx)
		cancel()
		if err != nil {
			t.Fatalf("Next() error: %v", err)
		}
		if !ok {
			t.Fatal("Next() returned ok=false, want true")
		}
		if len(msg.Value) != 1 || msg.Value[0] != byte(i) {
			t.Errorf("message %d value = %v, want [%d]", i, msg.Value, i)
		}
	}
}

func TestConsumerAsStream_Close(t *testing.T) {
	t.Parallel()

	broker := memory.NewBroker()
	defer broker.Close()

	consumer := broker.Consumer("close-topic")
	stream := bridge.ConsumerAsStream("close-test", consumer)

	iter, err := stream.Execute(context.Background(), struct{}{})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	// Close should not hang.
	if err := iter.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}
}
