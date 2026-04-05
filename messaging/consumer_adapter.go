package messaging

import (
	"context"

	"github.com/kbukum/gokit/provider"
)

// ConsumerProviderAdapter wraps a Consumer as a provider.Provider.
// This allows any messaging Consumer to participate in the provider framework.
type ConsumerProviderAdapter struct {
	name     string
	consumer Consumer
}

var _ provider.Provider = (*ConsumerProviderAdapter)(nil)

// NewConsumerProviderAdapter wraps a Consumer as a provider.Provider.
func NewConsumerProviderAdapter(name string, c Consumer) *ConsumerProviderAdapter {
	return &ConsumerProviderAdapter{name: name, consumer: c}
}

// Name returns the provider's unique name.
func (a *ConsumerProviderAdapter) Name() string { return a.name }

// IsAvailable checks if the consumer is ready.
func (a *ConsumerProviderAdapter) IsAvailable(_ context.Context) bool {
	return a.consumer != nil
}
