package messaging

import "context"

// EventPublisher is a convenience facade that wraps a Producer with a pre-configured source name.
// Every call to Publish
// or PublishKeyed automatically constructs an Event envelope (UUID, timestamp, source)
// so callers only provide the topic, event type, and payload.
type EventPublisher struct {
	producer Producer
	source   string
}

// NewEventPublisher creates an EventPublisher.
//
//   - producer: any Producer implementation (Kafka, in-memory, …).
//   - source:   the originating service name embedded in every event.
func NewEventPublisher(producer Producer, source string) *EventPublisher {
	return &EventPublisher{producer: producer, source: source}
}

// Publish sends a typed payload as a domain event.
//
// An Event envelope is built with a fresh UUID, UTC timestamp, the configured source,
// and data marshaled from the generic payload.
func (p *EventPublisher) Publish(ctx context.Context, topic, eventType string, data any) error {
	event, err := NewEvent[any](eventType, p.source, data)
	if err != nil {
		return err
	}
	return p.producer.Publish(ctx, topic, event)
}

// PublishKeyed sends a typed payload with an explicit partition key.
//
// The key is set both as the Event.Subject and the Kafka partition key.
// This is a deliberate five-parameter convenience variant of the idiomatic four-parameter Publish:
// the explicit key reads better as a positional argument than folded behind an opaque payload struct.
func (p *EventPublisher) PublishKeyed(ctx context.Context, topic, eventType string, data any, key string) error {
	event, err := NewEvent[any](eventType, p.source, data, key)
	if err != nil {
		return err
	}
	return p.producer.Publish(ctx, topic, event, key)
}

// Source returns the configured source name.
func (p *EventPublisher) Source() string {
	return p.source
}

// Producer returns the underlying producer.
func (p *EventPublisher) Producer() Producer {
	return p.producer
}
