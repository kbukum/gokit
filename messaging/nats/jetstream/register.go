package jetstream

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/messaging"
	"github.com/kbukum/gokit/messaging/nats"
)

// Register adds JetStream producer and consumer factories to registry under [AdapterName]. The producers and consumers it builds share one [Client] per request timeout, so many topics use one connection; the last one to close releases it.
//
// Registry consumers retry a failing message until the handler succeeds or returns [messaging.ErrTerminal]; [WithDeliveryPolicy] chooses another [DeliveryPolicy] per topic.
//
// The common configuration must ask for at-least-once delivery committed after handler success, one message in flight and no adapter DLQ. A consumer's topic is its filter subject and its stream is the stream that stores that subject. The common consumer group is required: with the topic it names the durable consumer, so processes in one group share a topic's position and each topic keeps its own. The registry accepts only exact subjects; a wildcard filter needs [Client.Consumer] or [Component.Consumer]. Failures are logged as warnings.
func Register(registry *messaging.Registry, cfg nats.Config, opts ...RegisterOption) error {
	if registry == nil {
		return errors.New("jetstream: messaging registry is nil")
	}
	var o registerOptions
	for _, opt := range opts {
		if opt == nil {
			return errors.New("jetstream: register option is nil")
		}
		if err := opt(&o); err != nil {
			return err
		}
	}
	cfg.ApplyDefaults()
	if err := validateClientConfig(cfg); err != nil {
		return err
	}
	clients := newSharedClients(cfg)
	if err := registry.RegisterProducer(AdapterName, func(ctx context.Context, common messaging.Config, _ *logging.Logger) (messaging.Producer, error) {
		if err := validateCommon(common); err != nil {
			return nil, err
		}
		backoff, err := time.ParseDuration(common.RetryBackoff)
		if err != nil {
			return nil, fmt.Errorf("jetstream: invalid common retry_backoff: %w", err)
		}
		pcfg := ProducerConfig{RetryAttempts: common.RetryAttempts, RetryBackoff: backoff}
		if verr := pcfg.Validate(); verr != nil {
			return nil, verr
		}
		client, release, err := clients.acquire(ctx, common.RequestTimeout)
		if err != nil {
			return nil, err
		}
		producer, err := newProducer(client.self, pcfg, release)
		if err != nil {
			return nil, errors.Join(err, release.Close()) //nolint:contextcheck // Close drains with its own bounded budget, not the build context
		}
		return producer, nil
	}); err != nil {
		return err
	}
	return registry.RegisterConsumer(AdapterName, func(ctx context.Context, common messaging.Config, log *logging.Logger, topic string) (messaging.Consumer, error) {
		if err := validateCommon(common); err != nil {
			return nil, err
		}
		durable, err := durableName(common.ConsumerGroup, topic)
		if err != nil {
			return nil, err
		}
		ccfg := ConsumerConfig{Durable: durable, FilterSubjects: []string{topic}, OnError: func(err error) {
			if log != nil {
				log.Warn("JetStream consumer failure", map[string]any{"topic": topic, "error": err.Error()})
			}
		}}
		if o.delivery != nil {
			ccfg.Delivery = o.delivery(topic)
		}
		ccfg.ApplyDefaults()
		if invalid := ccfg.Validate(); invalid != nil {
			return nil, invalid
		}
		client, release, err := clients.acquire(ctx, common.RequestTimeout)
		if err != nil {
			return nil, err
		}
		consumer, err := newConsumer(client.self, ccfg, release)
		if err != nil {
			return nil, errors.Join(err, release.Close()) //nolint:contextcheck // Close drains with its own bounded budget, not the build context
		}
		return consumer, nil
	})
}

// RegisterOption configures [Register].
type RegisterOption func(*registerOptions) error

type registerOptions struct {
	delivery func(topic string) DeliveryPolicy
}

// WithDeliveryPolicy sets the [DeliveryPolicy] of each registry consumer from its topic. A policy that fails [DeliveryPolicy.Validate] fails the consumer's construction.
func WithDeliveryPolicy(policy func(topic string) DeliveryPolicy) RegisterOption {
	return func(o *registerOptions) error {
		if policy == nil {
			return errors.New("jetstream: delivery policy is nil")
		}
		o.delivery = policy
		return nil
	}
}

// durableIDLen is the hex length of the topic digest in a registry durable name.
const durableIDLen = 16

// durableName names the registry consumer of topic in group: the group, a dash and a digest of the topic, so two topics in one group never share a durable consumer and replace each other's filter.
func durableName(group, topic string) (string, error) {
	maxGroup := 64 - 1 - durableIDLen
	if group == "" || len(group) > maxGroup || !namePattern.MatchString(group) {
		return "", fmt.Errorf("jetstream: consumer_group is required and must be 1-%d letters, digits, _ or -", maxGroup)
	}
	sum := sha256.Sum256([]byte(topic))
	return group + "-" + hex.EncodeToString(sum[:])[:durableIDLen], nil
}

func validateCommon(cfg messaging.Config) error {
	if cfg.DeliveryGuarantee != messaging.DeliveryAtLeastOnce {
		return fmt.Errorf("jetstream: supports only %s delivery", messaging.DeliveryAtLeastOnce)
	}
	if cfg.CommitStrategy != messaging.CommitAfterHandlerSuccess {
		return fmt.Errorf("jetstream: supports only %s commits", messaging.CommitAfterHandlerSuccess)
	}
	if cfg.MaxInFlight != 1 {
		return errors.New("jetstream: handlers run one at a time; max_in_flight must be 1")
	}
	if cfg.DLQ.Enabled {
		return errors.New("jetstream: adapter-managed DLQ is not supported; use messaging middleware")
	}
	return nil
}
