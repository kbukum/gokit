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
	var _ kafka.Publisher = comp.MockProducerClient()
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

func TestMockProducer_PublishJSON(t *testing.T) {
	p := &MockProducer{}
	ctx := context.Background()
	data := map[string]string{"name": "Alice"}
	if err := p.PublishJSON(ctx, "json-topic", "user-1", data); err != nil {
		t.Fatalf("PublishJSON() error: %v", err)
	}
	msgs := p.Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Topic != "json-topic" {
		t.Errorf("Topic = %q", msgs[0].Topic)
	}
	if string(msgs[0].Key) != "user-1" {
		t.Errorf("Key = %q", string(msgs[0].Key))
	}
	if msgs[0].Headers["content-type"] != "application/json" {
		t.Errorf("content-type = %q", msgs[0].Headers["content-type"])
	}
}

func TestMockProducer_PublishBinary(t *testing.T) {
	p := &MockProducer{}
	ctx := context.Background()
	if err := p.PublishBinary(ctx, "bin-topic", "key-1", []byte{0x01, 0x02}); err != nil {
		t.Fatalf("PublishBinary() error: %v", err)
	}
	msgs := p.Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Headers["content-type"] != "application/octet-stream" {
		t.Errorf("content-type = %q", msgs[0].Headers["content-type"])
	}
}

func TestMockProducer_Publish(t *testing.T) {
	p := &MockProducer{}
	ctx := context.Background()
	event := kafka.NewEvent("user.created", "test-service", map[string]string{"id": "123"}, "user-123")
	if err := p.Publish(ctx, "events", event); err != nil {
		t.Fatalf("Publish() error: %v", err)
	}
	msgs := p.Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Headers["event-type"] != "user.created" {
		t.Errorf("event-type = %q", msgs[0].Headers["event-type"])
	}
	if string(msgs[0].Key) != "user-123" {
		t.Errorf("Key = %q, want user-123", string(msgs[0].Key))
	}
}

func TestMockProducer_Publish_WithExplicitKey(t *testing.T) {
	p := &MockProducer{}
	event := kafka.NewEvent("test", "src", struct{}{}) // no subject
	if err := p.Publish(context.Background(), "t", event, "explicit-key"); err != nil {
		t.Fatal(err)
	}
	if string(p.Messages()[0].Key) != "explicit-key" {
		t.Errorf("Key = %q, want explicit-key", string(p.Messages()[0].Key))
	}
}

func TestMockProducer_Send(t *testing.T) {
	p := &MockProducer{}
	msg := kafka.Message{Topic: "send-topic", Key: "k1", Value: []byte("v1")}
	if err := p.Send(context.Background(), msg); err != nil {
		t.Fatalf("Send() error: %v", err)
	}
	msgs := p.Messages()
	if len(msgs) != 1 || msgs[0].Topic != "send-topic" {
		t.Errorf("unexpected messages: %+v", msgs)
	}
}

func TestMockProducer_ClosedErrors(t *testing.T) {
	p := &MockProducer{}
	p.Close()
	if !p.IsClosed() {
		t.Error("expected IsClosed=true")
	}
	if err := p.PublishJSON(context.Background(), "t", "k", "v"); err == nil {
		t.Error("expected error on closed producer")
	}
	if err := p.PublishBinary(context.Background(), "t", "k", nil); err == nil {
		t.Error("expected error on closed producer")
	}
	if err := p.Publish(context.Background(), "t", kafka.Event{}, "k"); err == nil {
		t.Error("expected error on closed producer")
	}
	if err := p.Send(context.Background(), kafka.Message{}); err == nil {
		t.Error("expected error on closed producer")
	}
}

func TestMockProducer_MessagesForTopic(t *testing.T) {
	p := &MockProducer{}
	p.WriteMessage("a", nil, []byte("1"))
	p.WriteMessage("b", nil, []byte("2"))
	p.WriteMessage("a", nil, []byte("3"))
	msgs := p.MessagesForTopic("a")
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages for topic a, got %d", len(msgs))
	}
}

func TestMockProducer_ResetReopens(t *testing.T) {
	p := &MockProducer{}
	p.Close()
	p.Reset()
	if p.IsClosed() {
		t.Error("Reset should reopen producer")
	}
	if len(p.Messages()) != 0 {
		t.Error("Reset should clear messages")
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
