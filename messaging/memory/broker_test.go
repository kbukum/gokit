package memory

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/kbukum/gokit/messaging"
)

func TestBroker_ProduceConsume(t *testing.T) {
	broker := NewBroker()
	defer broker.Close()

	producer := broker.Producer()
	consumer := broker.Consumer("test-topic")

	ctx, cancel := context.WithCancel(context.Background())
	var received []messaging.Message
	var mu sync.Mutex
	done := make(chan struct{})

	go func() {
		defer close(done)
		_ = consumer.Consume(ctx, func(_ context.Context, msg messaging.Message) error {
			mu.Lock()
			received = append(received, msg)
			mu.Unlock()
			if len(received) >= 2 {
				cancel()
			}
			return nil
		})
	}()

	if err := producer.PublishBinary(ctx, "test-topic", "k1", []byte("hello")); err != nil {
		t.Fatalf("PublishBinary() error: %v", err)
	}
	if err := producer.PublishBinary(ctx, "test-topic", "k2", []byte("world")); err != nil {
		t.Fatalf("PublishBinary() error: %v", err)
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("timed out waiting for consumer")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(received))
	}
	if string(received[0].Value) != "hello" {
		t.Errorf("msg[0].Value = %q, want hello", string(received[0].Value))
	}
	if received[0].Key != "k1" {
		t.Errorf("msg[0].Key = %q, want k1", received[0].Key)
	}
}

func TestBroker_PublishJSON(t *testing.T) {
	broker := NewBroker()
	defer broker.Close()

	producer := broker.Producer()
	consumer := broker.Consumer("json-topic")

	ctx, cancel := context.WithCancel(context.Background())
	var received messaging.Message
	done := make(chan struct{})

	go func() {
		defer close(done)
		_ = consumer.Consume(ctx, func(_ context.Context, msg messaging.Message) error {
			received = msg
			cancel()
			return nil
		})
	}()

	data := map[string]string{"name": "Alice"}
	if err := producer.PublishJSON(ctx, "json-topic", "user-1", data); err != nil {
		t.Fatalf("PublishJSON() error: %v", err)
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("timed out")
	}

	if received.Key != "user-1" {
		t.Errorf("Key = %q, want user-1", received.Key)
	}
	if received.Headers["content-type"] != "application/json" {
		t.Errorf("content-type = %q", received.Headers["content-type"])
	}
	var parsed map[string]string
	if err := json.Unmarshal(received.Value, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed["name"] != "Alice" {
		t.Errorf("name = %q, want Alice", parsed["name"])
	}
}

func TestBroker_PublishEvent(t *testing.T) {
	broker := NewBroker()
	defer broker.Close()

	producer := broker.Producer()
	consumer := broker.Consumer("events")

	ctx, cancel := context.WithCancel(context.Background())
	var received messaging.Message
	done := make(chan struct{})

	go func() {
		defer close(done)
		_ = consumer.Consume(ctx, func(_ context.Context, msg messaging.Message) error {
			received = msg
			cancel()
			return nil
		})
	}()

	event, err := messaging.NewEvent("user.created", "test-svc", map[string]string{"id": "42"}, "user-42")
	if err != nil {
		t.Fatalf("NewEvent() error: %v", err)
	}

	if err := producer.Publish(ctx, "events", event); err != nil {
		t.Fatalf("Publish() error: %v", err)
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("timed out")
	}

	if received.Key != "user-42" {
		t.Errorf("Key = %q, want user-42", received.Key)
	}
	if received.Headers["event-type"] != "user.created" {
		t.Errorf("event-type = %q", received.Headers["event-type"])
	}
}

func TestBroker_ClosedProducer(t *testing.T) {
	broker := NewBroker()
	defer broker.Close()

	producer := broker.Producer()
	_ = producer.Close()

	if err := producer.PublishBinary(context.Background(), "t", "k", []byte("data")); err == nil {
		t.Error("expected error publishing to closed producer")
	}
}

func TestBroker_ConsumerTopic(t *testing.T) {
	broker := NewBroker()
	defer broker.Close()
	consumer := broker.Consumer("my-topic")
	if consumer.Topic() != "my-topic" {
		t.Errorf("Topic() = %q, want my-topic", consumer.Topic())
	}
}

func TestBroker_ConsumerClose(t *testing.T) {
	broker := NewBroker()
	defer broker.Close()
	consumer := broker.Consumer("t")
	if err := consumer.Close(); err != nil {
		t.Errorf("Close() error: %v", err)
	}
}

func TestBroker_MultipleConsumers(t *testing.T) {
	broker := NewBroker()
	defer broker.Close()

	producer := broker.Producer()
	c1 := broker.Consumer("shared")
	c2 := broker.Consumer("shared")

	ctx, cancel := context.WithCancel(context.Background())

	var mu sync.Mutex
	c1Messages := 0
	c2Messages := 0

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		_ = c1.Consume(ctx, func(_ context.Context, _ messaging.Message) error {
			mu.Lock()
			c1Messages++
			mu.Unlock()
			return nil
		})
	}()

	go func() {
		defer wg.Done()
		_ = c2.Consume(ctx, func(_ context.Context, _ messaging.Message) error {
			mu.Lock()
			c2Messages++
			mu.Unlock()
			return nil
		})
	}()

	if err := producer.PublishBinary(ctx, "shared", "k", []byte("data")); err != nil {
		t.Fatalf("Publish error: %v", err)
	}

	// Give consumers time to process
	time.Sleep(50 * time.Millisecond)
	cancel()
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if c1Messages != 1 || c2Messages != 1 {
		t.Errorf("c1=%d, c2=%d — both should be 1 (fan-out)", c1Messages, c2Messages)
	}
}

func TestBrokerWithBuffer(t *testing.T) {
	broker := NewBrokerWithBuffer(1)
	defer broker.Close()
	producer := broker.Producer()
	_ = broker.Consumer("t") // subscribe so publish has a target

	// First publish should succeed (buffer=1)
	if err := producer.PublishBinary(context.Background(), "t", "k", []byte("1")); err != nil {
		t.Fatalf("first publish: %v", err)
	}
	// Second publish should fail (buffer full)
	if err := producer.PublishBinary(context.Background(), "t", "k", []byte("2")); err == nil {
		t.Error("expected error for full buffer")
	}
}
