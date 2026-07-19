package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/kbukum/gokit/messaging"
)

// InMemoryProducer implements messaging.Producer using an InMemoryBroker.
type InMemoryProducer struct {
	broker *InMemoryBroker
	closed bool
	mu     sync.Mutex
}

var _ messaging.Producer = (*InMemoryProducer)(nil)

// Send writes a pre-built transport-agnostic message to the broker.
func (p *InMemoryProducer) Send(_ context.Context, msg messaging.Message) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return messaging.ErrClosed
	}
	p.mu.Unlock()
	return p.broker.publish(msg.Topic, msg)
}

// SendBatch writes pre-built messages to the broker in order.
func (p *InMemoryProducer) SendBatch(ctx context.Context, messages []messaging.Message) error {
	for _, msg := range messages {
		if err := p.Send(ctx, msg); err != nil {
			return err
		}
	}
	return nil
}

// Flush is a no-op for the in-memory broker because delivery is synchronous.
func (p *InMemoryProducer) Flush(_ context.Context) error { return nil }

// Publish sends a structured event to the broker.
func (p *InMemoryProducer) Publish(_ context.Context, topic string, event messaging.Event, key ...string) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return messaging.ErrClosed
	}
	p.mu.Unlock()

	data, err := event.ToJSON()
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	partitionKey := event.Subject
	if partitionKey == "" && len(key) > 0 {
		partitionKey = key[0]
	}
	if partitionKey == "" {
		partitionKey = event.ID
	}
	msg := messaging.Message{
		Key:       partitionKey,
		Value:     data,
		Topic:     topic,
		Timestamp: event.Timestamp,
		Headers: map[string]string{
			"event-id":     event.ID,
			"event-type":   event.Type,
			"event-source": event.Source,
			"content-type": "application/json",
		},
	}
	return p.broker.publish(topic, msg)
}

// PublishJSON marshals value as JSON and sends it to the broker.
func (p *InMemoryProducer) PublishJSON(_ context.Context, topic, key string, value any) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return messaging.ErrClosed
	}
	p.mu.Unlock()

	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}
	msg := messaging.Message{
		Key:       key,
		Value:     data,
		Topic:     topic,
		Timestamp: time.Now().UTC(),
		Headers:   map[string]string{"content-type": "application/json"},
	}
	return p.broker.publish(topic, msg)
}

// PublishBinary sends raw bytes to the broker.
func (p *InMemoryProducer) PublishBinary(_ context.Context, topic, key string, data []byte) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return messaging.ErrClosed
	}
	p.mu.Unlock()

	msg := messaging.Message{
		Key:       key,
		Value:     data,
		Topic:     topic,
		Timestamp: time.Now().UTC(),
		Headers:   map[string]string{"content-type": "application/octet-stream"},
	}
	return p.broker.publish(topic, msg)
}

// Close marks the producer as closed.
func (p *InMemoryProducer) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	return nil
}
