package kafka

import (
	"encoding/json"
	"testing"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

func TestEvent_ToJSON(t *testing.T) {
	e := Event{
		ID:     "test-id",
		Type:   "user.created",
		Source: "test-service",
	}
	data, err := e.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON() error: %v", err)
	}
	var parsed Event
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.ID != "test-id" || parsed.Type != "user.created" {
		t.Errorf("round-trip failed: %+v", parsed)
	}
}

func TestNewEvent(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}
	e := NewEvent("user.created", "my-service", payload{Name: "John"})
	if e.ID == "" {
		t.Error("expected auto-generated ID")
	}
	if e.Type != "user.created" {
		t.Errorf("Type = %q, want user.created", e.Type)
	}
	if e.Source != "my-service" {
		t.Errorf("Source = %q, want my-service", e.Source)
	}
	if e.Timestamp.IsZero() {
		t.Error("expected non-zero Timestamp")
	}
	if e.Subject != "" {
		t.Error("expected empty Subject when not provided")
	}
}

func TestNewEvent_WithSubject(t *testing.T) {
	e := NewEvent("order.placed", "shop", map[string]string{"item": "book"}, "order-123")
	if e.Subject != "order-123" {
		t.Errorf("Subject = %q, want order-123", e.Subject)
	}
}

func TestParseData(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}
	e := NewEvent("test", "src", payload{Name: "Alice"})
	parsed, err := ParseData[payload](e)
	if err != nil {
		t.Fatalf("ParseData() error: %v", err)
	}
	if parsed.Name != "Alice" {
		t.Errorf("Name = %q, want Alice", parsed.Name)
	}
}

func TestParseData_Invalid(t *testing.T) {
	e := Event{Data: []byte("not-json")}
	_, err := ParseData[map[string]string](e)
	if err == nil {
		t.Error("expected error for invalid JSON data")
	}
}

func TestFromKafkaMessage(t *testing.T) {
	now := time.Now()
	km := kafkago.Message{
		Key:       []byte("key1"),
		Value:     []byte(`{"hello":"world"}`),
		Topic:     "test-topic",
		Partition: 2,
		Offset:    42,
		Time:      now,
		Headers: []kafkago.Header{
			{Key: "content-type", Value: []byte("application/json")},
		},
	}
	msg := FromKafkaMessage(km)
	if msg.Key != "key1" {
		t.Errorf("Key = %q, want key1", msg.Key)
	}
	if string(msg.Value) != `{"hello":"world"}` {
		t.Errorf("Value = %q", string(msg.Value))
	}
	if msg.Topic != "test-topic" {
		t.Errorf("Topic = %q", msg.Topic)
	}
	if msg.Partition != 2 {
		t.Errorf("Partition = %d, want 2", msg.Partition)
	}
	if msg.Offset != 42 {
		t.Errorf("Offset = %d, want 42", msg.Offset)
	}
	if msg.Timestamp != now {
		t.Errorf("Timestamp mismatch")
	}
	if msg.Headers["content-type"] != "application/json" {
		t.Errorf("Header content-type = %q", msg.Headers["content-type"])
	}
}

func TestMessage_ToKafkaMessage(t *testing.T) {
	msg := Message{
		Key:       "k1",
		Value:     []byte("v1"),
		Topic:     "t1",
		Partition: 1,
		Offset:    10,
		Headers:   map[string]string{"h1": "val1"},
	}
	km := msg.ToKafkaMessage()
	if string(km.Key) != "k1" {
		t.Errorf("Key = %q", string(km.Key))
	}
	if string(km.Value) != "v1" {
		t.Errorf("Value = %q", string(km.Value))
	}
	if km.Topic != "t1" {
		t.Errorf("Topic = %q", km.Topic)
	}
	if len(km.Headers) != 1 || km.Headers[0].Key != "h1" {
		t.Errorf("Headers = %v", km.Headers)
	}
}

func TestMessage_IsJSON_ByHeader(t *testing.T) {
	msg := Message{
		Headers: map[string]string{"content-type": "application/json"},
		Value:   []byte("not json at all"),
	}
	if !msg.IsJSON() {
		t.Error("expected IsJSON=true when content-type header is application/json")
	}
}

func TestMessage_IsJSON_ByContent(t *testing.T) {
	tests := []struct {
		value []byte
		want  bool
	}{
		{[]byte(`{"key":"val"}`), true},
		{[]byte(`[1,2,3]`), true},
		{[]byte(`plain text`), false},
		{[]byte{}, false},
		{nil, false},
	}
	for _, tt := range tests {
		msg := Message{Value: tt.value, Headers: map[string]string{}}
		if got := msg.IsJSON(); got != tt.want {
			t.Errorf("IsJSON(%q) = %v, want %v", string(tt.value), got, tt.want)
		}
	}
}

func TestMessage_UnmarshalValueJSON(t *testing.T) {
	msg := Message{Value: []byte(`{"name":"Bob"}`)}
	var result map[string]string
	if err := msg.UnmarshalValueJSON(&result); err != nil {
		t.Fatalf("UnmarshalValueJSON() error: %v", err)
	}
	if result["name"] != "Bob" {
		t.Errorf("name = %q, want Bob", result["name"])
	}
}

func TestMessage_UnmarshalValueJSON_Invalid(t *testing.T) {
	msg := Message{Value: []byte("not json")}
	var result map[string]string
	if err := msg.UnmarshalValueJSON(&result); err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestMessage_ToEvent(t *testing.T) {
	e := Event{
		ID:     "ev-1",
		Type:   "test.event",
		Source: "unit-test",
	}
	data, _ := json.Marshal(e)
	msg := Message{Value: data}
	parsed, err := msg.ToEvent()
	if err != nil {
		t.Fatalf("ToEvent() error: %v", err)
	}
	if parsed.ID != "ev-1" || parsed.Type != "test.event" {
		t.Errorf("ToEvent() = %+v", parsed)
	}
}

func TestMessage_ToEvent_Invalid(t *testing.T) {
	msg := Message{Value: []byte("not json")}
	_, err := msg.ToEvent()
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestFromKafkaMessage_NoHeaders(t *testing.T) {
	km := kafkago.Message{Key: []byte("k"), Value: []byte("v")}
	msg := FromKafkaMessage(km)
	if len(msg.Headers) != 0 {
		t.Errorf("expected empty headers, got %v", msg.Headers)
	}
}
