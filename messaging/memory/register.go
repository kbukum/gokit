package memory

import (
	"context"
	"fmt"

	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/messaging"
)

const adapterName = "memory"

// Config contains in-memory adapter settings.
type Config struct {
	BufferSize int             `yaml:"buffer_size" mapstructure:"buffer_size"`
	Broker     *InMemoryBroker `yaml:"-" mapstructure:"-"`
}

// ApplyDefaults fills zero-valued fields.
func (c *Config) ApplyDefaults() {
	if c.BufferSize <= 0 {
		c.BufferSize = defaultBufferSize
	}
}

// Validate checks in-memory settings.
func (c Config) Validate() error {
	if c.BufferSize <= 0 {
		return fmt.Errorf("memory: buffer_size must be > 0")
	}
	return nil
}

// Register adds typed in-memory producer and consumer factories to registry.
func Register(registry *messaging.Registry, configs ...Config) error {
	if registry == nil {
		return fmt.Errorf("memory: messaging registry is nil")
	}
	broker, err := brokerFromConfig(configs...)
	if err != nil {
		return err
	}
	if err := registry.RegisterProducer(adapterName, func(_ context.Context, common messaging.Config, _ *logging.Logger) (messaging.Producer, error) {
		if common.DeliveryGuarantee == messaging.DeliveryExactlyOnce {
			return nil, fmt.Errorf("memory: exactly-once delivery is not supported")
		}
		if common.DLQ.Enabled {
			return nil, fmt.Errorf("memory: adapter-managed DLQ is not supported; use messaging middleware")
		}
		return broker.Producer(), nil
	}); err != nil {
		return err
	}
	return registry.RegisterConsumer(adapterName, func(_ context.Context, common messaging.Config, _ *logging.Logger, topic string) (messaging.Consumer, error) {
		if err := validateCommonConsumer(common); err != nil {
			return nil, err
		}
		return broker.consumer(topic, common.CommitStrategy), nil
	})
}

func brokerFromConfig(configs ...Config) (*InMemoryBroker, error) {
	if len(configs) == 0 {
		return NewBroker(), nil
	}
	cfg := configs[0]
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if cfg.Broker != nil {
		return cfg.Broker, nil
	}
	return NewBrokerWithBuffer(cfg.BufferSize), nil
}

func validateCommonConsumer(cfg messaging.Config) error {
	if cfg.MaxInFlight != 1 {
		return fmt.Errorf("memory: max_in_flight > 1 is not supported by the serial consumer")
	}
	switch cfg.DeliveryGuarantee {
	case messaging.DeliveryAtLeastOnce:
		if cfg.CommitStrategy != messaging.CommitAfterHandlerSuccess {
			return fmt.Errorf("memory: at-least-once delivery requires %s commits", messaging.CommitAfterHandlerSuccess)
		}
	case messaging.DeliveryAtMostOnce:
		if cfg.CommitStrategy != messaging.CommitAuto {
			return fmt.Errorf("memory: at-most-once delivery requires %s commits", messaging.CommitAuto)
		}
	case messaging.DeliveryExactlyOnce:
		return fmt.Errorf("memory: exactly-once delivery is not supported")
	}
	if cfg.DLQ.Enabled {
		return fmt.Errorf("memory: adapter-managed DLQ is not supported; use messaging middleware")
	}
	return nil
}
