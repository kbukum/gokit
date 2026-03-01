package testutil

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/kafka"
	"github.com/kbukum/gokit/testutil"
)

// Message represents a produced or consumed message.
type Message struct {
	Topic   string
	Key     []byte
	Value   []byte
	Headers map[string]string
}

// MockProducer is an in-memory producer that records all written messages.
// Implements kafka.ProducerCloser and kafka.Publisher.
type MockProducer struct {
	messages []Message
	mu       sync.Mutex
	closed   bool
}

var _ kafka.ProducerCloser = (*MockProducer)(nil)
var _ kafka.Publisher = (*MockProducer)(nil)

// WriteMessage records a message (legacy helper).
func (p *MockProducer) WriteMessage(topic string, key, value []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.messages = append(p.messages, Message{Topic: topic, Key: key, Value: value})
}

// Publish sends a structured gokit Event to the in-memory store.
func (p *MockProducer) Publish(_ context.Context, topic string, event kafka.Event, key ...string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return fmt.Errorf("producer is closed")
	}
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
	p.messages = append(p.messages, Message{
		Topic: topic,
		Key:   []byte(partitionKey),
		Value: data,
		Headers: map[string]string{
			"event-id":     event.ID,
			"event-type":   event.Type,
			"event-source": event.Source,
			"content-type": "application/json",
		},
	})
	return nil
}

// PublishJSON marshals value as JSON and records it.
func (p *MockProducer) PublishJSON(_ context.Context, topic string, key string, value interface{}) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return fmt.Errorf("producer is closed")
	}
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}
	p.messages = append(p.messages, Message{
		Topic:   topic,
		Key:     []byte(key),
		Value:   data,
		Headers: map[string]string{"content-type": "application/json"},
	})
	return nil
}

// PublishBinary records raw bytes.
func (p *MockProducer) PublishBinary(_ context.Context, topic string, key string, data []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return fmt.Errorf("producer is closed")
	}
	p.messages = append(p.messages, Message{
		Topic:   topic,
		Key:     []byte(key),
		Value:   data,
		Headers: map[string]string{"content-type": "application/octet-stream"},
	})
	return nil
}

// Send writes a domain Message (implements provider.Sink[kafka.Message]).
func (p *MockProducer) Send(_ context.Context, msg kafka.Message) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return fmt.Errorf("producer is closed")
	}
	p.messages = append(p.messages, Message{
		Topic:   msg.Topic,
		Key:     []byte(msg.Key),
		Value:   msg.Value,
		Headers: msg.Headers,
	})
	return nil
}

// Messages returns all recorded messages.
func (p *MockProducer) Messages() []Message {
	p.mu.Lock()
	defer p.mu.Unlock()
	cp := make([]Message, len(p.messages))
	copy(cp, p.messages)
	return cp
}

// MessagesForTopic returns messages filtered by topic.
func (p *MockProducer) MessagesForTopic(topic string) []Message {
	p.mu.Lock()
	defer p.mu.Unlock()
	var result []Message
	for _, m := range p.messages {
		if m.Topic == topic {
			result = append(result, m)
		}
	}
	return result
}

// Reset clears all recorded messages and reopens the producer.
func (p *MockProducer) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.messages = nil
	p.closed = false
}

// Close marks the producer as closed.
func (p *MockProducer) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	return nil
}

// IsClosed returns whether the producer has been closed.
func (p *MockProducer) IsClosed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closed
}

// MockConsumer is an in-memory consumer that blocks until fed messages or canceled.
type MockConsumer struct {
	topic    string
	messages chan Message
	handler  func(Message)
}

var _ kafka.ConsumerRunner = (*MockConsumer)(nil)

// NewMockConsumer creates a mock consumer for the given topic.
func NewMockConsumer(topic string) *MockConsumer {
	return &MockConsumer{
		topic:    topic,
		messages: make(chan Message, 100),
	}
}

// OnMessage sets a handler called for each consumed message.
func (c *MockConsumer) OnMessage(fn func(Message)) {
	c.handler = fn
}

// Feed sends a message to the consumer's input channel.
func (c *MockConsumer) Feed(msg Message) {
	c.messages <- msg
}

// Consume blocks until context is canceled, processing fed messages.
// After cancellation, any remaining buffered messages are still delivered.
func (c *MockConsumer) Consume(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			// Drain buffered messages before returning
			for {
				select {
				case msg := <-c.messages:
					if c.handler != nil {
						c.handler(msg)
					}
				default:
					return ctx.Err()
				}
			}
		case msg := <-c.messages:
			if c.handler != nil {
				c.handler(msg)
			}
		}
	}
}

// Close is a no-op for the mock.
func (c *MockConsumer) Close() error { return nil }

// Topic returns the consumer's topic.
func (c *MockConsumer) Topic() string { return c.topic }

// Component is a test Kafka component with mock producer and consumers.
type Component struct {
	producer  *MockProducer
	consumers map[string]*MockConsumer
	started   bool
	mu        sync.RWMutex
}

var _ component.Component = (*Component)(nil)
var _ testutil.TestComponent = (*Component)(nil)

// NewComponent creates a new mock Kafka test component.
func NewComponent() *Component {
	return &Component{
		producer:  &MockProducer{},
		consumers: make(map[string]*MockConsumer),
	}
}

// AddConsumer adds a mock consumer for the given topic.
func (c *Component) AddConsumer(topic string) *MockConsumer {
	c.mu.Lock()
	defer c.mu.Unlock()
	mc := NewMockConsumer(topic)
	c.consumers[topic] = mc
	return mc
}

// MockProducerClient returns the mock producer for assertions.
func (c *Component) MockProducerClient() *MockProducer {
	return c.producer
}

// MockConsumerClient returns the mock consumer for a specific topic.
func (c *Component) MockConsumerClient(topic string) *MockConsumer {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.consumers[topic]
}

// --- component.Component ---

func (c *Component) Name() string { return "kafka-test" }

func (c *Component) Start(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.started {
		return fmt.Errorf("component already started")
	}
	c.started = true
	return nil
}

func (c *Component) Stop(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.started = false
	return nil
}

func (c *Component) Health(_ context.Context) component.Health {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.started {
		return component.Health{Name: c.Name(), Status: component.StatusUnhealthy, Message: "not started"}
	}
	return component.Health{Name: c.Name(), Status: component.StatusHealthy}
}

// --- testutil.TestComponent ---

func (c *Component) Reset(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.started {
		return fmt.Errorf("component not started")
	}
	c.producer.Reset()
	return nil
}

func (c *Component) Snapshot(_ context.Context) (interface{}, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.producer.Messages(), nil
}

func (c *Component) Restore(_ context.Context, _ interface{}) error {
	// Kafka is a log — restoring state doesn't make semantic sense.
	// Reset is the appropriate operation for test isolation.
	return nil
}
