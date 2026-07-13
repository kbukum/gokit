package rabbitmq

import (
	"context"
	"fmt"
	"time"

	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/messaging"
)

// Register adds typed RabbitMQ producer and consumer factories to registry.
func Register(registry *messaging.Registry, configs ...Config) error {
	if registry == nil {
		return fmt.Errorf("rabbitmq: messaging registry is nil")
	}
	cfg, err := configFromRegistration(configs...)
	if err != nil {
		return err
	}
	if err := registry.RegisterProducer(adapterName, func(_ context.Context, common messaging.Config, _ *logging.Logger) (messaging.Producer, error) {
		cfg := cfg
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
	return registry.RegisterConsumer(adapterName, func(_ context.Context, common messaging.Config, _ *logging.Logger, topic string) (messaging.Consumer, error) {
		cfg := cfg
		applied, err := applyCommonConsumer(common, cfg)
		if err != nil {
			return nil, err
		}
		return NewConsumer(applied, topic)
	})
}

func configFromRegistration(configs ...Config) (Config, error) {
	if len(configs) > 1 {
		return Config{}, fmt.Errorf("rabbitmq: at most one config may be provided, got %d", len(configs))
	}
	cfg := Config{}
	if len(configs) > 0 {
		cfg = configs[0]
	}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
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
