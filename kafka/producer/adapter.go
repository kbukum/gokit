package producer

import (
	"context"
	"fmt"

	"github.com/kbukum/gokit/kafka"
	"github.com/kbukum/gokit/provider"
)

// compile-time assertions — Producer IS the provider
var _ provider.Sink[kafka.Message] = (*Producer)(nil)

// Name returns the producer name (implements provider.Provider).
func (p *Producer) Name() string {
	return p.cfg.Name
}

// IsAvailable checks if the producer is ready (implements provider.Provider).
func (p *Producer) IsAvailable(_ context.Context) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return !p.closed
}

// Send writes a single domain message to Kafka (implements provider.Sink[kafka.Message]).
func (p *Producer) Send(ctx context.Context, msg kafka.Message) error {
	if err := p.WriteMessages(ctx, msg.ToKafkaMessage()); err != nil {
		return fmt.Errorf("kafka producer send: %w", err)
	}
	return nil
}
