package messaging

import "github.com/kbukum/gokit/component"

// ProducerOption configures producer creation.
type ProducerOption func(*producerOptions)

type producerOptions struct{}

// ConsumerOption configures consumer creation.
type ConsumerOption func(*consumerOptions)

type consumerOptions struct{}

// BrokerComponent extends component.Component with producer/consumer factory methods.
// Implementations provide broker-specific creation logic while sharing the common lifecycle management from component.Component.
type BrokerComponent interface {
	component.Component

	// Producer creates a producer for the given topic.
	Producer(topic string, opts ...ProducerOption) (Producer, error)

	// Consumer registers a consumer for the given topics with the provided handler.
	Consumer(topics []string, handler MessageHandler, opts ...ConsumerOption) error
}
