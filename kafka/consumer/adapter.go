package consumer

import (
	"context"

	"github.com/kbukum/gokit/provider"
)

// compile-time assertions
var _ provider.Provider = (*Consumer)(nil)

// Name returns the consumer name (implements provider.Provider).
func (c *Consumer) Name() string {
	return c.groupID + ":" + c.topic
}

// IsAvailable checks if the consumer is ready (implements provider.Provider).
func (c *Consumer) IsAvailable(_ context.Context) bool {
	return c.reader != nil
}
