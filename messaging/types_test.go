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
		Payload: []byte("not json at all"),
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
		msg := Message{Payload: tt.value, Headers: map[string]string{}}
		if got := msg.IsJSON(); got != tt.want {
			t.Errorf("IsJSON(%q) = %v, want %v", string(tt.value), got, tt.want)
		}
	}
}

func TestMessage_UnmarshalPayloadJSON(t *testing.T) {
	msg := Message{Payload: []byte(`{"name":"Bob"}`)}
	var result map[string]string
	if err := msg.UnmarshalPayloadJSON(&result); err != nil {
		t.Fatalf("UnmarshalPayloadJSON() error: %v", err)
	}
	if result["name"] != "Bob" {
		t.Errorf("name = %q, want Bob", result["name"])
	}
}

func TestMessage_UnmarshalPayloadJSON_Invalid(t *testing.T) {
	msg := Message{Payload: []byte("not json")}
	var result map[string]string
	if err := msg.UnmarshalPayloadJSON(&result); err == nil {
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
	msg := Message{Payload: data}
	parsed, err := msg.ToEvent()
	if err != nil {
		t.Fatalf("ToEvent() error: %v", err)
	}
	if parsed.ID != "ev-1" || parsed.Type != "test.event" {
		t.Errorf("ToEvent() = %+v", parsed)
	}
}

func TestMessage_ToEvent_Invalid(t *testing.T) {
	msg := Message{Payload: []byte("not json")}
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
	if string(msg.Payload) != "val" {
		t.Errorf("Payload = %q", string(msg.Payload))
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
	msg := Message{Payload: []byte(`{"name":"Alice","age":30}`)}
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

	if _, err := UnmarshalMessageJSON[map[string]string](Message{Payload: []byte("not json")}); err == nil {
		t.Fatal("expected unmarshal error for invalid JSON")
	}
}

func TestMessage_RoutingKey(t *testing.T) {
	t.Parallel()
	if got := (Message{Key: "partition-1"}).RoutingKey(); got != "partition-1" {
		t.Fatalf("RoutingKey() = %q, want partition-1", got)
	}
}

func FuzzParseData(f *testing.F) {
	f.Add([]byte(`{"id":1}`))
	f.Add([]byte(`not-json`))

	type payload struct {
		ID int `json:"id"`
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = ParseData[payload](Event{Data: data})
	})
}

func TestMessage_JSONWireShape(t *testing.T) {
	t.Parallel()

	msg := Message{
		Key:       "k1",
		Payload:   []byte(`{"hello":"world"}`),
		Topic:     "orders",
		Partition: 2,
		Offset:    42,
		Timestamp: time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
		Headers:   map[string]string{"content-type": "application/json"},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}

	var wire map[string]json.RawMessage
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}

	wantKeys := map[string]bool{
		"key": true, "payload": true, "topic": true, "partition": true,
		"offset": true, "timestamp": true, "headers": true,
	}
	for k := range wire {
		if !wantKeys[k] {
			t.Errorf("unexpected wire key %q", k)
		}
	}
	for k := range wantKeys {
		if _, ok := wire[k]; !ok {
			t.Errorf("missing wire key %q", k)
		}
	}
	if _, ok := wire["value"]; ok {
		t.Error("wire shape must not contain legacy key \"value\"")
	}

	var payload string
	if err := json.Unmarshal(wire["payload"], &payload); err != nil {
		t.Fatalf("payload not base64 string: %v", err)
	}
	if payload == "" {
		t.Error("payload key must carry the message body")
	}

	var back Message
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("round-trip Unmarshal() error: %v", err)
	}
	if string(back.Payload) != `{"hello":"world"}` {
		t.Errorf("round-trip Payload = %q", string(back.Payload))
	}
	if back.Key != "k1" || back.Topic != "orders" || back.Partition != 2 || back.Offset != 42 {
		t.Errorf("round-trip envelope = %+v", back)
	}
}

func TestMessage_JSONHeadersOmitEmpty(t *testing.T) {
	t.Parallel()

	data, err := json.Marshal(Message{Key: "k", Payload: []byte("x"), Topic: "t"})
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}
	if _, ok := wire["headers"]; ok {
		t.Error("headers must be omitted when empty")
	}
}
