package producer

import (
	"context"
	"fmt"

	"github.com/kbukum/gokit/logger"
	"github.com/kbukum/gokit/messaging"
	"github.com/kbukum/gokit/messaging/kafka"
)

const backendName = "kafka"

// Register adds a config-free lazy Kafka producer factory to registry.
func Register(registry *messaging.Registry) error {
	if registry == nil {
		return fmt.Errorf("kafka producer: messaging registry is nil")
	}
	return registry.RegisterProducer(backendName, func(_ context.Context, common messaging.Config, providerCfg any, log *logger.Logger) (messaging.Producer, error) {
		cfg, ok := providerCfg.(*kafka.Config)
		if !ok {
			return nil, &messaging.ConfigTypeError{Backend: backendName, Expected: "*kafka.Config", Actual: providerCfg}
		}
		return NewLazyProducer(common, *cfg, log) //nolint:contextcheck // lazy producer construction does not perform request-scoped I/O
	})
}
