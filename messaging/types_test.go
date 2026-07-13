package messaging

import (
	"encoding/json"
	"testing"
	"time"
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
	e, err := NewEvent("user.created", "my-service", payload{Name: "John"})
	if err != nil {
		t.Fatalf("NewEvent() error: %v", err)
	}
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
	e, err := NewEvent("order.placed", "shop", map[string]string{"item": "book"}, "order-123")
	if err != nil {
		t.Fatalf("NewEvent() error: %v", err)
	}
	if e.Subject != "order-123" {
		t.Errorf("Subject = %q, want order-123", e.Subject)
	}
}

func TestParseData(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}
	e, err := NewEvent("test", "src", payload{Name: "Alice"})
	if err != nil {
		t.Fatalf("NewEvent() error: %v", err)
	}
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

func TestMessage_Timestamp(t *testing.T) {
	now := time.Now()
	msg := Message{Timestamp: now}
	if msg.Timestamp != now {
		t.Errorf("Timestamp mismatch")
	}
}

func TestNewMessage(t *testing.T) {
	t.Parallel()

	msg := NewMessage("topic", "key", []byte("val"), nil)
	if msg.Topic != "topic" {
		t.Errorf("Topic = %q", msg.Topic)
	}
	if msg.Key != "key" {
		t.Errorf("Key = %q", msg.Key)
	}
	if string(msg.Value) != "val" {
		t.Errorf("Value = %q", string(msg.Value))
	}
	if msg.Headers == nil {
		t.Error("expected non-nil Headers")
	}
}

func TestNewMessagePreservesHeaders(t *testing.T) {
	t.Parallel()

	msg := NewMessage("topic", "key", []byte("val"), map[string]string{"h": "v"})
	if msg.Headers["h"] != "v" {
		t.Errorf("Headers[h] = %q, want v", msg.Headers["h"])
	}
}

func TestUnmarshalMessageJSON(t *testing.T) {
	t.Parallel()
	msg := Message{Value: []byte(`{"name":"Alice","age":30}`)}
	out, err := UnmarshalMessageJSON[struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}](msg)
	if err != nil {
		t.Fatalf("UnmarshalMessageJSON() error: %v", err)
	}
	if out.Name != "Alice" || out.Age != 30 {
		t.Fatalf("decoded = %+v", out)
	}

	if _, err := UnmarshalMessageJSON[map[string]string](Message{Value: []byte("not json")}); err == nil {
		t.Fatal("expected unmarshal error for invalid JSON")
	}
}

func TestMessage_RoutingKey(t *testing.T) {
	t.Parallel()
	if got := (Message{Key: "partition-1"}).RoutingKey(); got != "partition-1" {
		t.Fatalf("RoutingKey() = %q, want partition-1", got)
	}
}
