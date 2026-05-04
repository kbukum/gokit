package rabbitmq

import (
	"context"
	"fmt"
	"time"

	"github.com/kbukum/gokit/logger"
	"github.com/kbukum/gokit/messaging"
)

// Register adds config-free RabbitMQ producer and consumer factories to registry.
func Register(registry *messaging.Registry) error {
	if registry == nil {
		return fmt.Errorf("rabbitmq: messaging registry is nil")
	}
	if err := registry.RegisterProducer(backendName, func(_ context.Context, common messaging.Config, providerCfg any, _ *logger.Logger) (messaging.Producer, error) {
		cfg, err := configFromProviderCfg(providerCfg)
		if err != nil {
			return nil, err
		}
		if commonErr := validateCommonProducer(common); commonErr != nil {
			return nil, commonErr
		}
		cfg.PublishTimeout = common.RequestTimeout
		retryBackoff, err := time.ParseDuration(common.RetryBackoff)
		if err != nil {
			return nil, fmt.Errorf("rabbitmq: invalid common retry_backoff: %w", err)
		}
		return newProducer(cfg, common.RetryAttempts, retryBackoff)
	}); err != nil {
		return err
	}
	return registry.RegisterConsumer(backendName, func(_ context.Context, common messaging.Config, providerCfg any, _ *logger.Logger, topic string) (messaging.Consumer, error) {
		cfg, err := configFromProviderCfg(providerCfg)
		if err != nil {
			return nil, err
		}
		cfg, err = applyCommonConsumer(common, cfg)
		if err != nil {
			return nil, err
		}
		return NewConsumer(cfg, topic)
	})
}

func configFromProviderCfg(providerCfg any) (Config, error) {
	if providerCfg == nil {
		cfg := Config{}
		cfg.ApplyDefaults()
		if err := cfg.Validate(); err != nil {
			return Config{}, err
		}
		return cfg, nil
	}
	cfg, ok := providerCfg.(*Config)
	if !ok {
		return Config{}, &messaging.ConfigTypeError{Backend: backendName, Expected: "*rabbitmq.Config", Actual: providerCfg}
	}
	out := *cfg
	out.ApplyDefaults()
	if err := out.Validate(); err != nil {
		return Config{}, err
	}
	return out, nil
}

func validateCommonProducer(cfg messaging.Config) error {
	if cfg.DeliveryGuarantee == messaging.DeliveryExactlyOnce {
		return fmt.Errorf("rabbitmq: exactly-once delivery is not supported")
	}
	if cfg.DLQ.Enabled {
		return fmt.Errorf("rabbitmq: adapter-managed DLQ is not supported; use messaging middleware")
	}
	return nil
}

func applyCommonConsumer(common messaging.Config, cfg Config) (Config, error) {
	switch common.DeliveryGuarantee {
	case messaging.DeliveryAtLeastOnce:
		if common.CommitStrategy != messaging.CommitAfterHandlerSuccess {
			return Config{}, fmt.Errorf("rabbitmq: at-least-once delivery requires %s commits", messaging.CommitAfterHandlerSuccess)
		}
		cfg.AutoAck = false
	case messaging.DeliveryAtMostOnce:
		if common.CommitStrategy != messaging.CommitAuto {
			return Config{}, fmt.Errorf("rabbitmq: at-most-once delivery requires %s commits", messaging.CommitAuto)
		}
		cfg.AutoAck = true
	case messaging.DeliveryExactlyOnce:
		return Config{}, fmt.Errorf("rabbitmq: exactly-once delivery is not supported")
	}
	if common.DLQ.Enabled {
		return Config{}, fmt.Errorf("rabbitmq: adapter-managed DLQ is not supported; use messaging middleware")
	}
	if common.ConsumerGroup != "" {
		if cfg.QueueName != "" && cfg.QueueName != common.ConsumerGroup {
			return Config{}, fmt.Errorf("rabbitmq: queue_name must match common consumer_group")
		}
		cfg.QueueName = common.ConsumerGroup
	}
	if cfg.PrefetchCount == 0 {
		cfg.PrefetchCount = common.MaxInFlight
	} else if cfg.PrefetchCount != common.MaxInFlight {
		return Config{}, fmt.Errorf("rabbitmq: prefetch_count must match common max_in_flight")
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
