package producer

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	kafkago "github.com/segmentio/kafka-go"

	"github.com/kbukum/gokit/kafka"
	"github.com/kbukum/gokit/logger"
)

// Publisher provides a high-level API for publishing events and JSON data to Kafka.
type Publisher interface {
	Publish(ctx context.Context, topic string, event kafka.Event, key ...string) error
	PublishJSON(ctx context.Context, topic string, key string, data interface{}) error
	Close() error
}

// KafkaPublisher implements Publisher by wrapping a Producer.
type KafkaPublisher struct {
	producer *Producer
	log      *logger.Logger
}

var _ Publisher = (*KafkaPublisher)(nil)

// NewPublisher creates a Publisher that wraps the given Producer.
func NewPublisher(producer *Producer, log *logger.Logger) *KafkaPublisher {
	return &KafkaPublisher{
		producer: producer,
		log:      log.WithComponent("kafka.publisher"),
	}
}

// Publish sends a structured Event to Kafka. An optional partition key can be
// provided; otherwise the event Subject or ID is used.
func (p *KafkaPublisher) Publish(ctx context.Context, topic string, event kafka.Event, key ...string) error {
	data, err := event.ToJSON()
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	partitionKey := determineKey(event, key)

	msg := kafkago.Message{
		Topic: topic,
		Key:   []byte(partitionKey),
		Value: data,
		Headers: []kafkago.Header{
			{Key: "event-id", Value: []byte(event.ID)},
			{Key: "event-type", Value: []byte(event.Type)},
			{Key: "event-source", Value: []byte(event.Source)},
			{Key: "content-type", Value: []byte("application/json")},
		},
		Time: event.Timestamp,
	}

	if err := p.producer.WriteMessages(ctx, msg); err != nil {
		return fmt.Errorf("publish event: %w", err)
	}
	return nil
}

// PublishJSON marshals data as JSON, wraps it in an Event, and publishes it.
func (p *KafkaPublisher) PublishJSON(ctx context.Context, topic string, key string, data interface{}) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}

	var eventData map[string]interface{}
	if err := json.Unmarshal(raw, &eventData); err != nil {
		eventData = map[string]interface{}{"payload": data}
	}

	event := kafka.Event{
		ID:          key,
		Type:        "kafka.message",
		Source:      "gokit",
		ContentType: "application/json",
		Version:     "1.0",
		Timestamp:   time.Now(),
		Data:        eventData,
		Subject:     key,
	}

	return p.Publish(ctx, topic, event, key)
}

// Close shuts down the underlying producer.
func (p *KafkaPublisher) Close() error {
	return p.producer.Close()
}

func determineKey(event kafka.Event, keys []string) string {
	if event.Subject != "" {
		return event.Subject
	}
	if len(keys) > 0 && keys[0] != "" {
		return keys[0]
	}
	return event.ID
}
