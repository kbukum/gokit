package messaging

import (
	"context"

	"github.com/kbukum/gokit/provider"
)

// ProducerProviderAdapter wraps a Producer as a provider.Provider and
// provider.Sink[Message]. This allows any messaging Producer to participate
// in the provider framework — composable with resilience wrappers,
// selectable via Manager, and pipelineable.
type ProducerProviderAdapter struct {
	name     string
	producer Producer
}

var _ provider.Provider = (*ProducerProviderAdapter)(nil)
var _ provider.Sink[Message] = (*ProducerProviderAdapter)(nil)

// NewProducerProviderAdapter wraps a Producer as a provider.Provider and Sink.
func NewProducerProviderAdapter(name string, p Producer) *ProducerProviderAdapter {
	return &ProducerProviderAdapter{name: name, producer: p}
}

// Name returns the provider's unique name.
func (a *ProducerProviderAdapter) Name() string { return a.name }

// IsAvailable checks if the producer is ready.
func (a *ProducerProviderAdapter) IsAvailable(_ context.Context) bool {
	return a.producer != nil
}

// Send writes a domain Message to the underlying producer.
func (a *ProducerProviderAdapter) Send(ctx context.Context, msg Message) error {
	return a.producer.PublishBinary(ctx, msg.Topic, msg.Key, msg.Value)
}

// Producer returns the underlying Producer for direct access.
func (a *ProducerProviderAdapter) Producer() Producer { return a.producer }
