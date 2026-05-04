package memory

import (
	"context"
	"fmt"

	"github.com/kbukum/gokit/logger"
	"github.com/kbukum/gokit/messaging"
)

const backendName = "memory"

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

// Register adds config-free in-memory producer and consumer factories to registry.
func Register(registry *messaging.Registry) error {
	if registry == nil {
		return fmt.Errorf("memory: messaging registry is nil")
	}
	if err := registry.RegisterProducer(backendName, func(_ context.Context, common messaging.Config, providerCfg any, _ *logger.Logger) (messaging.Producer, error) {
		broker, err := brokerFromProviderCfg(providerCfg)
		if err != nil {
			return nil, err
		}
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
	return registry.RegisterConsumer(backendName, func(_ context.Context, common messaging.Config, providerCfg any, _ *logger.Logger, topic string) (messaging.Consumer, error) {
		broker, err := brokerFromProviderCfg(providerCfg)
		if err != nil {
			return nil, err
		}
		if err := validateCommonConsumer(common); err != nil {
			return nil, err
		}
		return broker.consumer(topic, common.CommitStrategy), nil
	})
}

func brokerFromProviderCfg(providerCfg any) (*InMemoryBroker, error) {
	if providerCfg == nil {
		return NewBroker(), nil
	}
	if broker, ok := providerCfg.(*InMemoryBroker); ok {
		return broker, nil
	}
	if cfg, ok := providerCfg.(*Config); ok {
		out := *cfg
		out.ApplyDefaults()
		if err := out.Validate(); err != nil {
			return nil, err
		}
		if out.Broker != nil {
			return out.Broker, nil
		}
		return NewBrokerWithBuffer(out.BufferSize), nil
	}
	return nil, &messaging.ConfigTypeError{Backend: backendName, Expected: "*memory.InMemoryBroker or *memory.Config", Actual: providerCfg}
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
