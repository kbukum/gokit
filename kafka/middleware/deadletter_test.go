package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kbukum/gokit/kafka"
)

// mockPublisher records calls so tests can inspect what was published.
type mockPublisher struct {
	lastTopic string
	lastKey   string
	lastValue interface{}
	err       error
}

func (m *mockPublisher) Publish(_ context.Context, _ string, _ kafka.Event, _ ...string) error {
	return m.err
}
func (m *mockPublisher) PublishJSON(_ context.Context, topic, key string, value interface{}) error {
	m.lastTopic = topic
	m.lastKey = key
	m.lastValue = value
	return m.err
}
func (m *mockPublisher) PublishBinary(_ context.Context, _, _ string, _ []byte) error {
	return m.err
}
func (m *mockPublisher) Close() error { return nil }

func TestNewDeadLetterProducer_DefaultSuffix(t *testing.T) {
	d := NewDeadLetterProducer(&mockPublisher{})
	if d.suffix != ".dlq" {
		t.Errorf("default suffix = %q, want .dlq", d.suffix)
	}
}

func TestNewDeadLetterProducer_CustomSuffix(t *testing.T) {
	d := NewDeadLetterProducer(&mockPublisher{}, WithSuffix(".dead"))
	if d.suffix != ".dead" {
		t.Errorf("suffix = %q, want .dead", d.suffix)
	}
}

func TestDeadLetterProducer_Send(t *testing.T) {
	pub := &mockPublisher{}
	d := NewDeadLetterProducer(pub)

	msg := kafka.Message{
		Topic:   "orders",
		Key:     "order-123",
		Value:   []byte(`{"id":"order-123"}`),
		Headers: map[string]string{"x-retry-count": "3", "content-type": "application/json"},
	}

	err := d.Send(context.Background(), msg, errors.New("processing failed"))
	if err != nil {
		t.Fatalf("Send() error: %v", err)
	}

	if pub.lastTopic != "orders.dlq" {
		t.Errorf("topic = %q, want orders.dlq", pub.lastTopic)
	}
	if pub.lastKey != "order-123" {
		t.Errorf("key = %q, want order-123", pub.lastKey)
	}

	// Verify envelope contents.
	data, _ := json.Marshal(pub.lastValue)
	var env DeadLetterEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if env.OriginalTopic != "orders" {
		t.Errorf("OriginalTopic = %q, want orders", env.OriginalTopic)
	}
	if env.Error != "processing failed" {
		t.Errorf("Error = %q, want processing failed", env.Error)
	}
	if env.RetryCount != 3 {
		t.Errorf("RetryCount = %d, want 3", env.RetryCount)
	}
	if env.Timestamp.IsZero() {
		t.Error("expected non-zero Timestamp")
	}
	if env.Headers["content-type"] != "application/json" {
		t.Errorf("Headers[content-type] = %q", env.Headers["content-type"])
	}
	if string(env.Payload) != `{"id":"order-123"}` {
		t.Errorf("Payload = %q", string(env.Payload))
	}
}

func TestDeadLetterProducer_Send_EmptyKey(t *testing.T) {
	pub := &mockPublisher{}
	d := NewDeadLetterProducer(pub)

	msg := kafka.Message{
		Topic:   "events",
		Key:     "",
		Value:   []byte("data"),
		Headers: map[string]string{},
	}

	_ = d.Send(context.Background(), msg, errors.New("err"))
	if pub.lastKey != "dlq" {
		t.Errorf("key = %q, want dlq (fallback)", pub.lastKey)
	}
}

func TestDeadLetterProducer_Send_NoRetryCountHeader(t *testing.T) {
	pub := &mockPublisher{}
	d := NewDeadLetterProducer(pub)

	msg := kafka.Message{
		Topic:   "events",
		Value:   []byte("data"),
		Headers: map[string]string{},
	}

	_ = d.Send(context.Background(), msg, errors.New("err"))

	data, _ := json.Marshal(pub.lastValue)
	var env DeadLetterEnvelope
	_ = json.Unmarshal(data, &env)

	if env.RetryCount != 0 {
		t.Errorf("RetryCount = %d, want 0 when header absent", env.RetryCount)
	}
}

func TestDeadLetterProducer_Send_PublisherError(t *testing.T) {
	pubErr := errors.New("publish failed")
	pub := &mockPublisher{err: pubErr}
	d := NewDeadLetterProducer(pub)

	msg := kafka.Message{Topic: "t", Headers: map[string]string{}}
	err := d.Send(context.Background(), msg, errors.New("original"))
	if !errors.Is(err, pubErr) {
		t.Errorf("expected publisher error, got %v", err)
	}
}

func TestDeadLetterProducer_Send_CustomSuffix(t *testing.T) {
	pub := &mockPublisher{}
	d := NewDeadLetterProducer(pub, WithSuffix("-dead-letter"))

	msg := kafka.Message{Topic: "payments", Headers: map[string]string{}}
	_ = d.Send(context.Background(), msg, errors.New("err"))

	if pub.lastTopic != "payments-dead-letter" {
		t.Errorf("topic = %q, want payments-dead-letter", pub.lastTopic)
	}
}
