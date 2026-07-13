package producer

import (
	"context"
	"fmt"

	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/messaging"
	"github.com/kbukum/gokit/messaging/kafka"
)

const adapterName = "kafka"

// Register adds a typed lazy Kafka producer factory to registry.
func Register(registry *messaging.Registry, cfg kafka.Config) error {
	if registry == nil {
		return fmt.Errorf("kafka producer: messaging registry is nil")
	}
	return registry.RegisterProducer(adapterName, func(_ context.Context, common messaging.Config, log *logging.Logger) (messaging.Producer, error) {
		return NewLazyProducer(common, cfg, log) //nolint:contextcheck // lazy producer construction does not perform request-scoped I/O
	})
}
