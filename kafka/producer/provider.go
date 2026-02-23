package producer

import (
	"context"
	"fmt"

	kafkago "github.com/segmentio/kafka-go"

	"github.com/kbukum/gokit/provider"
)

// SinkProvider wraps a Producer as a provider.Sink[kafkago.Message].
// This allows the Kafka producer to participate in the provider framework —
// composable with WithSinkResilience(), selectable via Manager, pipelineable.
//
// For batch writes, use Producer.WriteMessages directly.
// The Sink adapter sends one message at a time for composability.
type SinkProvider struct {
	name     string
	producer *Producer
}

// NewSinkProvider wraps an existing Producer as a Sink provider.
func NewSinkProvider(name string, producer *Producer) *SinkProvider {
	return &SinkProvider{name: name, producer: producer}
}

// Name returns the provider's unique name.
func (p *SinkProvider) Name() string { return p.name }

// IsAvailable checks if the producer is not closed and the writer is initialized.
func (p *SinkProvider) IsAvailable(_ context.Context) bool {
	p.producer.mu.RLock()
	defer p.producer.mu.RUnlock()
	return !p.producer.closed
}

// Send writes a single message to Kafka via WriteMessages.
func (p *SinkProvider) Send(ctx context.Context, msg kafkago.Message) error {
	if err := p.producer.WriteMessages(ctx, msg); err != nil {
		return fmt.Errorf("kafka sink send: %w", err)
	}
	return nil
}

// Producer returns the underlying *Producer for batch operations.
func (p *SinkProvider) Producer() *Producer { return p.producer }

// compile-time check
var _ provider.Sink[kafkago.Message] = (*SinkProvider)(nil)
