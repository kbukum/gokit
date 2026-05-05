package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Message represents a broker message with both binary and JSON support.
type Message struct {
	Key       string            `json:"key"`
	Value     []byte            `json:"value"`
	Topic     string            `json:"topic"`
	Partition int               `json:"partition"`
	Offset    int64             `json:"offset"`
	Timestamp time.Time         `json:"timestamp"`
	Headers   map[string]string `json:"headers,omitempty"`
}

// NewMessage creates a broker-neutral Message with topic, key, value, and headers.
func NewMessage(topic, key string, value []byte, headers map[string]string) Message {
	if headers == nil {
		headers = make(map[string]string)
	}
	return Message{
		Key:     key,
		Value:   value,
		Topic:   topic,
		Headers: headers,
	}
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
// Returns an error if data cannot be marshaled to JSON.
func NewEvent[D any](eventType, source string, data D, subject ...string) (Event, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return Event{}, fmt.Errorf("messaging: marshal event data: %w", err)
	}
	e := Event{
		ID:        uuid.New().String(),
		Type:      eventType,
		Source:    source,
		Version:   "1.0",
		Timestamp: time.Now().UTC(),
		Data:      raw,
	}
	if len(subject) > 0 {
		e.Subject = subject[0]
	}
	return e, nil
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
// v is intentionally opaque because encoding/json requires a caller-owned destination.
func (m Message) UnmarshalValueJSON(v any) error {
	return json.Unmarshal(m.Value, v)
}

// UnmarshalMessageJSON unmarshals the message value into a typed value.
func UnmarshalMessageJSON[T any](m Message) (T, error) {
	var out T
	err := json.Unmarshal(m.Value, &out)
	return out, err
}

// RoutingKey returns the broker-neutral partition/routing key for the message.
func (m Message) RoutingKey() string { return m.Key }

// ToEvent converts the message to an Event (assumes JSON content).
func (m Message) ToEvent() (Event, error) {
	var event Event
	err := json.Unmarshal(m.Value, &event)
	return event, err
}
