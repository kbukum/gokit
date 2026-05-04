package messaging_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/kbukum/gokit/messaging"
)

// mockProducer captures published events for assertion.
type mockProducer struct {
	mu     sync.Mutex
	events []publishedEvent
}

type publishedEvent struct {
	topic string
	event messaging.Event
	key   string
}

func (m *mockProducer) Publish(_ context.Context, topic string, event messaging.Event, key ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := ""
	if len(key) > 0 {
		k = key[0]
	}
	m.events = append(m.events, publishedEvent{topic: topic, event: event, key: k})
	return nil
}

func (m *mockProducer) PublishJSON(context.Context, string, string, any) error      { return nil }
func (m *mockProducer) PublishBinary(context.Context, string, string, []byte) error { return nil }
func (m *mockProducer) Send(context.Context, messaging.Message) error               { return nil }
func (m *mockProducer) SendBatch(context.Context, []messaging.Message) error        { return nil }
func (m *mockProducer) Flush(context.Context) error                                 { return nil }
func (m *mockProducer) Close() error                                                { return nil }

func TestPublishCreatesEnvelope(t *testing.T) {
	mock := &mockProducer{}
	pub := messaging.NewEventPublisher(mock, "test-service")

	type Payload struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	err := pub.Publish(context.Background(), "my.topic", "payload.created", Payload{Name: "hello", Value: 42})
	if err != nil {
		t.Fatal(err)
	}

	if len(mock.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(mock.events))
	}
	ev := mock.events[0]
	if ev.topic != "my.topic" {
		t.Errorf("topic = %q, want %q", ev.topic, "my.topic")
	}
	if ev.event.Type != "payload.created" {
		t.Errorf("type = %q, want %q", ev.event.Type, "payload.created")
	}
	if ev.event.Source != "test-service" {
		t.Errorf("source = %q, want %q", ev.event.Source, "test-service")
	}
	if ev.event.ID == "" {
		t.Error("expected non-empty event ID")
	}

	var parsed Payload
	if err := json.Unmarshal(ev.event.Data, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Name != "hello" || parsed.Value != 42 {
		t.Errorf("data = %+v, want {hello 42}", parsed)
	}
}

func TestPublishKeyedSetsSubjectAndKey(t *testing.T) {
	mock := &mockProducer{}
	pub := messaging.NewEventPublisher(mock, "order-svc")

	err := pub.PublishKeyed(context.Background(), "orders", "order.placed", map[string]string{"id": "123"}, "order-123")
	if err != nil {
		t.Fatal(err)
	}

	ev := mock.events[0]
	if ev.event.Subject != "order-123" {
		t.Errorf("subject = %q, want %q", ev.event.Subject, "order-123")
	}
	if ev.key != "order-123" {
		t.Errorf("key = %q, want %q", ev.key, "order-123")
	}
	if ev.event.Type != "order.placed" {
		t.Errorf("type = %q, want %q", ev.event.Type, "order.placed")
	}
}

func TestSourceAccessor(t *testing.T) {
	mock := &mockProducer{}
	pub := messaging.NewEventPublisher(mock, "my-service")
	if pub.Source() != "my-service" {
		t.Errorf("Source() = %q, want %q", pub.Source(), "my-service")
	}
}
