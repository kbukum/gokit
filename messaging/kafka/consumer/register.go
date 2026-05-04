package consumer

import (
	"context"
	"fmt"

	"github.com/kbukum/gokit/logger"
	"github.com/kbukum/gokit/messaging"
	"github.com/kbukum/gokit/messaging/kafka"
)

const backendName = "kafka"

// Register adds a config-free Kafka consumer factory to registry.
func Register(registry *messaging.Registry) error {
	if registry == nil {
		return fmt.Errorf("kafka consumer: messaging registry is nil")
	}
	return registry.RegisterConsumer(backendName, func(_ context.Context, common messaging.Config, providerCfg any, log *logger.Logger, topic string) (messaging.Consumer, error) {
		cfg, ok := providerCfg.(*kafka.Config)
		if !ok {
			return nil, &messaging.ConfigTypeError{Backend: backendName, Expected: "*kafka.Config", Actual: providerCfg}
		}
		return NewConsumer(common, *cfg, topic, log) //nolint:contextcheck // factory construction is synchronous; request ctx is owned by Consume
	})
}
