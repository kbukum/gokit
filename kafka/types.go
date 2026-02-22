package kafka

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
)

// Message represents a Kafka message with both binary and JSON support.
type Message struct {
	Key       string            `json:"key"`
	Value     []byte            `json:"value"`
	Topic     string            `json:"topic"`
	Partition int               `json:"partition"`
	Offset    int64             `json:"offset"`
	Timestamp time.Time         `json:"timestamp"`
	Headers   map[string]string `json:"headers,omitempty"`
}

// Event represents a structured event for domain messaging.
// Data is json.RawMessage so events can be forwarded without re-marshaling.
type Event struct {
	ID          string          `json:"id"`
	Type        string          `json:"type"`
	Source      string          `json:"source"`
	ContentType string          `json:"content_type,omitempty"`
	Version     string          `json:"version,omitempty"`
	Timestamp   time.Time       `json:"timestamp"`
	Subject     string          `json:"subject,omitempty"`
	Data        json.RawMessage `json:"data,omitempty"`
}

// ToJSON marshals the event to JSON.
func (e Event) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}

// NewEvent creates an Event with auto-generated ID and timestamp.
// Data is marshaled to json.RawMessage automatically.
func NewEvent[D any](eventType, source string, data D, subject ...string) Event {
	raw, _ := json.Marshal(data)
	e := Event{
		ID:        uuid.New().String(),
		Type:      eventType,
		Source:    source,
		Timestamp: time.Now().UTC(),
		Data:      raw,
	}
	if len(subject) > 0 {
		e.Subject = subject[0]
	}
	return e
}

// ParseData unmarshals the event's Data into a typed value.
func ParseData[D any](e Event) (D, error) {
	var data D
	err := json.Unmarshal(e.Data, &data)
	return data, err
}

// MessageHandler processes domain messages (supports both binary and JSON).
type MessageHandler func(ctx context.Context, msg Message) error

// EventHandler processes structured events.
type EventHandler func(ctx context.Context, event Event) error

// BinaryHandler processes raw binary messages.
type BinaryHandler func(ctx context.Context, key string, value []byte) error

// JSONHandler processes JSON messages with automatic unmarshalling.
type JSONHandler[T any] func(ctx context.Context, data T) error

// Publisher defines the unified Kafka publishing interface.
//
// Three methods for three use cases:
//   - Publish:       structured domain events (gokit Event with headers/metadata)
//   - PublishJSON:   arbitrary data as JSON (direct marshal, no envelope)
//   - PublishBinary: raw bytes (protobuf, avro, etc. — zero encoding overhead)
type Publisher interface {
	Publish(ctx context.Context, topic string, event Event, key ...string) error
	PublishJSON(ctx context.Context, topic string, key string, value interface{}) error
	PublishBinary(ctx context.Context, topic string, key string, data []byte) error
	Close() error
}

// FromKafkaMessage converts a kafka-go Message to the domain Message type.
func FromKafkaMessage(msg kafka.Message) Message {
	headers := make(map[string]string)
	for _, h := range msg.Headers {
		headers[h.Key] = string(h.Value)
	}
	return Message{
		Key:       string(msg.Key),
		Value:     msg.Value,
		Topic:     msg.Topic,
		Partition: msg.Partition,
		Offset:    msg.Offset,
		Timestamp: msg.Time,
		Headers:   headers,
	}
}

// ToKafkaMessage converts the domain Message back to a kafka-go Message.
func (m Message) ToKafkaMessage() kafka.Message {
	headers := make([]kafka.Header, 0, len(m.Headers))
	for k, v := range m.Headers {
		headers = append(headers, kafka.Header{Key: k, Value: []byte(v)})
	}
	return kafka.Message{
		Key:       []byte(m.Key),
		Value:     m.Value,
		Topic:     m.Topic,
		Partition: m.Partition,
		Offset:    m.Offset,
		Time:      m.Timestamp,
		Headers:   headers,
	}
}

// IsJSON checks if the message appears to be JSON.
func (m Message) IsJSON() bool {
	if ct, ok := m.Headers["content-type"]; ok && ct == "application/json" {
		return true
	}
	if len(m.Value) > 0 {
		return m.Value[0] == '{' || m.Value[0] == '['
	}
	return false
}

// UnmarshalValueJSON unmarshals the message value as JSON into v.
func (m Message) UnmarshalValueJSON(v interface{}) error {
	return json.Unmarshal(m.Value, v)
}

// ToEvent converts the message to an Event (assumes JSON content).
func (m Message) ToEvent() (Event, error) {
	var event Event
	err := json.Unmarshal(m.Value, &event)
	return event, err
}
