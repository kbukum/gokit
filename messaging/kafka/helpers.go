package kafka

import (
	kafkago "github.com/segmentio/kafka-go"

	"github.com/kbukum/gokit/messaging"
)

// FromKafkaMessage converts a kafka-go Message to the domain Message type.
func FromKafkaMessage(msg kafkago.Message) messaging.Message {
	headers := make(map[string]string)
	for _, h := range msg.Headers {
		headers[h.Key] = string(h.Value)
	}
	return messaging.Message{
		Key:       string(msg.Key),
		Value:     msg.Value,
		Topic:     msg.Topic,
		Partition: msg.Partition,
		Offset:    msg.Offset,
		Timestamp: msg.Time,
		Headers:   headers,
	}
}

// ToKafkaMessage converts the domain Message to a kafka-go Message.
func ToKafkaMessage(m messaging.Message) kafkago.Message {
	headers := make([]kafkago.Header, 0, len(m.Headers))
	for k, v := range m.Headers {
		headers = append(headers, kafkago.Header{Key: k, Value: []byte(v)})
	}
	return kafkago.Message{
		Key:       []byte(m.Key),
		Value:     m.Value,
		Topic:     m.Topic,
		Partition: m.Partition,
		Offset:    m.Offset,
		Time:      m.Timestamp,
		Headers:   headers,
	}
}

// NewMessage creates a basic domain Message with key, value, topic, and headers.
func NewMessage(topic, key string, value []byte, headers map[string]string) messaging.Message {
	if headers == nil {
		headers = make(map[string]string)
	}
	return messaging.Message{
		Key:     key,
		Value:   value,
		Topic:   topic,
		Headers: headers,
	}
}
