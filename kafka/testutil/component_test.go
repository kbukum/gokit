package testutil

import (
	"context"
	"testing"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/kafka"
	"github.com/kbukum/gokit/testutil"
)

func TestComponent_Interfaces(t *testing.T) {
	comp := NewComponent()
	var _ component.Component = comp
	var _ testutil.TestComponent = comp
	var _ kafka.ProducerCloser = comp.MockProducerClient()
}

func TestComponent_Lifecycle(t *testing.T) {
	comp := NewComponent()
	ctx := context.Background()

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	health := comp.Health(ctx)
	if health.Status != component.StatusHealthy {
		t.Errorf("Health = %q, want %q", health.Status, component.StatusHealthy)
	}

	if err := comp.Stop(ctx); err != nil {
		t.Fatalf("Stop() failed: %v", err)
	}
}

func TestMockProducer_WriteAndRead(t *testing.T) {
	comp := NewComponent()
	ctx := context.Background()
	comp.Start(ctx)
	defer comp.Stop(ctx)

	producer := comp.MockProducerClient()
	producer.WriteMessage("topic-1", []byte("key"), []byte("value"))
	producer.WriteMessage("topic-2", nil, []byte("data"))

	msgs := producer.Messages()
	if len(msgs) != 2 {
		t.Fatalf("Messages() = %d, want 2", len(msgs))
	}
	if msgs[0].Topic != "topic-1" || string(msgs[0].Value) != "value" {
		t.Errorf("msg[0] = %+v, unexpected", msgs[0])
	}

	// Reset clears messages
	comp.Reset(ctx)
	if len(producer.Messages()) != 0 {
		t.Error("Messages after Reset should be empty")
	}
}

func TestMockConsumer_FeedAndProcess(t *testing.T) {
	comp := NewComponent()
	mc := comp.AddConsumer("events")

	var received []Message
	mc.OnMessage(func(msg Message) {
		received = append(received, msg)
	})

	// Run consumer in background
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		mc.Consume(ctx)
		close(done)
	}()

	mc.Feed(Message{Topic: "events", Value: []byte("hello")})
	mc.Feed(Message{Topic: "events", Value: []byte("world")})

	// Give consumer time to process
	cancel()
	<-done

	if len(received) != 2 {
		t.Errorf("received %d messages, want 2", len(received))
	}
}

func TestMockConsumer_Topic(t *testing.T) {
	mc := NewMockConsumer("my-topic")
	if mc.Topic() != "my-topic" {
		t.Errorf("Topic() = %q, want %q", mc.Topic(), "my-topic")
	}
}
