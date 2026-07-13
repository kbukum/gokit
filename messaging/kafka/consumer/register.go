package consumer

import (
	"context"
	"fmt"

	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/messaging"
	"github.com/kbukum/gokit/messaging/kafka"
)

const adapterName = "kafka"

// Register adds a typed Kafka consumer factory to registry.
func Register(registry *messaging.Registry, cfg kafka.Config) error {
	if registry == nil {
		return fmt.Errorf("kafka consumer: messaging registry is nil")
	}
	return registry.RegisterConsumer(adapterName, func(_ context.Context, common messaging.Config, log *logging.Logger, topic string) (messaging.Consumer, error) {
		return NewConsumer(common, cfg, topic, log) //nolint:contextcheck // factory construction is synchronous; request ctx is owned by Consume
	})
}
