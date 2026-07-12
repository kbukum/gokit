package consumer

import (
	"context"
	"fmt"

	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/messaging"
	"github.com/kbukum/gokit/messaging/kafka"
)

const adapterName = "kafka"

// Register adds a config-free Kafka consumer factory to registry.
func Register(registry *messaging.Registry) error {
	if registry == nil {
		return fmt.Errorf("kafka consumer: messaging registry is nil")
	}
	return registry.RegisterConsumer(adapterName, func(_ context.Context, common messaging.Config, adapterCfg any, log *logging.Logger, topic string) (messaging.Consumer, error) {
		cfg, ok := adapterCfg.(*kafka.Config)
		if !ok {
			return nil, &messaging.ConfigTypeError{Adapter: adapterName, Expected: "*kafka.Config", Actual: adapterCfg}
		}
		return NewConsumer(common, *cfg, topic, log) //nolint:contextcheck // factory construction is synchronous; request ctx is owned by Consume
	})
}
