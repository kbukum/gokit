package nats

import (
	"context"
	"fmt"
	"time"

	"github.com/kbukum/gokit/logger"
	"github.com/kbukum/gokit/messaging"
)

// Register adds config-free NATS producer and consumer factories to registry.
func Register(registry *messaging.Registry) error {
	if registry == nil {
		return fmt.Errorf("nats: messaging registry is nil")
	}
	if err := registry.RegisterProducer(backendName, func(_ context.Context, common messaging.Config, providerCfg any, _ *logger.Logger) (messaging.Producer, error) {
		cfg, err := configFromProviderCfg(providerCfg)
		if err != nil {
			return nil, err
		}
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
	return registry.RegisterConsumer(backendName, func(_ context.Context, common messaging.Config, providerCfg any, _ *logger.Logger, topic string) (messaging.Consumer, error) {
		cfg, err := configFromProviderCfg(providerCfg)
		if err != nil {
			return nil, err
		}
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
		return Config{}, &messaging.ConfigTypeError{Backend: backendName, Expected: "*nats.Config", Actual: providerCfg}
	}
	out := *cfg
	out.ApplyDefaults()
	if err := out.Validate(); err != nil {
		return Config{}, err
	}
	return out, nil
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
