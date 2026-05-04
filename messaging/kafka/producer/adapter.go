package producer

import (
	"context"
	"fmt"

	"github.com/kbukum/gokit/messaging"
	"github.com/kbukum/gokit/messaging/kafka"
	"github.com/kbukum/gokit/provider"
)

// compile-time assertions — Producer IS the provider
var _ provider.Sink[messaging.Message] = (*Producer)(nil)

// Name returns the producer name (implements provider.Provider).
func (p *Producer) Name() string {
	return p.name
}

// IsAvailable checks if the producer is ready (implements provider.Provider).
func (p *Producer) IsAvailable(_ context.Context) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return !p.closed
}

// Send writes a single domain message to Kafka (implements provider.Sink[messaging.Message]).
func (p *Producer) Send(ctx context.Context, msg messaging.Message) error {
	if err := p.WriteMessages(ctx, kafka.ToKafkaMessage(msg)); err != nil {
		return fmt.Errorf("kafka producer send: %w", err)
	}
	return nil
}
