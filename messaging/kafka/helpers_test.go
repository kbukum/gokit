package kafka

import (
	"os"
	"strings"
	"testing"
	"time"

	kafkago "github.com/segmentio/kafka-go"

	"github.com/kbukum/gokit/messaging"
)

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

func TestToKafkaMessage(t *testing.T) {
	msg := messaging.NewMessage("t1", "k1", []byte("v1"), map[string]string{"h1": "val1"})
	msg.Partition = 1
	msg.Offset = 10
	km := ToKafkaMessage(msg)
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

func TestFromKafkaMessage_NoHeaders(t *testing.T) {
	km := kafkago.Message{Key: []byte("k"), Value: []byte("v")}
	msg := FromKafkaMessage(km)
	if len(msg.Headers) != 0 {
		t.Errorf("expected empty headers, got %v", msg.Headers)
	}
}

func TestKafkaPackageDoesNotDefineGenericMessageConstructor(t *testing.T) {
	t.Parallel()

	files := []string{"translator.go", "helpers.go"}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		if strings.Contains(string(data), "func NewMessage(") {
			t.Fatalf("%s defines generic NewMessage; use messaging.NewMessage in core", file)
		}
	}
}
