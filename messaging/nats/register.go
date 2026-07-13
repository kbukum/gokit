package nats

import (
	"context"
	"fmt"
	"time"

	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/messaging"
)

// Register adds typed NATS producer and consumer factories to registry.
func Register(registry *messaging.Registry, configs ...Config) error {
	if registry == nil {
		return fmt.Errorf("nats: messaging registry is nil")
	}
	cfg, err := configFromRegistration(configs...)
	if err != nil {
		return err
	}
	if err := registry.RegisterProducer(adapterName, func(_ context.Context, common messaging.Config, _ *logging.Logger) (messaging.Producer, error) {
		cfg := cfg
		if commonErr := validateCommon(common); commonErr != nil {
			return nil, commonErr
		}
		cfg.PublishTimeout = common.RequestTimeout
		retryBackoff, err := time.ParseDuration(common.RetryBackoff)
		if err != nil {
			return nil, fmt.Errorf("nats: invalid common retry_backoff: %w", err)
		}
		return newProducer(cfg, common.RetryAttempts, retryBackoff)
	}); err != nil {
		return err
	}
	return registry.RegisterConsumer(adapterName, func(_ context.Context, common messaging.Config, _ *logging.Logger, topic string) (messaging.Consumer, error) {
		cfg := cfg
		if commonErr := validateCommon(common); commonErr != nil {
			return nil, commonErr
		}
		if common.ConsumerGroup != "" {
			if cfg.QueueGroup != "" && cfg.QueueGroup != common.ConsumerGroup {
				return nil, fmt.Errorf("nats: queue_group must match common consumer_group")
			}
			cfg.QueueGroup = common.ConsumerGroup
		}
		return NewConsumer(cfg, topic)
	})
}

func configFromRegistration(configs ...Config) (Config, error) {
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

func validateCommon(cfg messaging.Config) error {
	if cfg.DeliveryGuarantee != messaging.DeliveryAtMostOnce {
		return fmt.Errorf("nats: core NATS supports only %s delivery", messaging.DeliveryAtMostOnce)
	}
	if cfg.CommitStrategy != messaging.CommitAuto {
		return fmt.Errorf("nats: core NATS supports only %s commits", messaging.CommitAuto)
	}
	if cfg.MaxInFlight != 1 {
		return fmt.Errorf("nats: max_in_flight > 1 requires JetStream or application-level concurrency")
	}
	if cfg.DLQ.Enabled {
		return fmt.Errorf("nats: adapter-managed DLQ is not supported; use messaging middleware")
	}
	return nil
}
