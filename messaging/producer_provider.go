package messaging

import (
	"context"

	"github.com/kbukum/gokit/provider"
)

// ProducerSinkProvider wraps a Producer as a provider.Sink[Message].
// Unlike ProducerProviderAdapter which wraps with a fixed name, this
// allows a custom name independent of the underlying producer.
//
// For batch writes, use the Producer directly.
// The Sink adapter sends one message at a time for composability.
type ProducerSinkProvider struct {
	name     string
	producer Producer
}

var _ provider.Sink[Message] = (*ProducerSinkProvider)(nil)

// NewProducerSinkProvider wraps a Producer as a named Sink provider.
func NewProducerSinkProvider(name string, p Producer) *ProducerSinkProvider {
	return &ProducerSinkProvider{name: name, producer: p}
}

// Name returns the provider's unique name.
func (p *ProducerSinkProvider) Name() string { return p.name }

// IsAvailable checks if the producer is ready.
func (p *ProducerSinkProvider) IsAvailable(_ context.Context) bool {
	return p.producer != nil
}

// Send writes a domain Message via the underlying producer.
func (p *ProducerSinkProvider) Send(ctx context.Context, msg Message) error {
	return p.producer.PublishBinary(ctx, msg.Topic, msg.Key, msg.Value)
}

// Producer returns the underlying Producer for direct access.
func (p *ProducerSinkProvider) Producer() Producer { return p.producer }
