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

// Register adds JetStream producer and consumer factories to registry under [AdapterName]. Each producer or consumer
// it builds owns its own [Client] and closes it on Close.
//
// The common configuration must ask for at-least-once delivery committed after handler success, one message in flight
// and no adapter DLQ. A consumer's topic is its filter subject and its stream is the stream that stores that subject.
// The common consumer group is required: with the topic it names the durable consumer, so processes in one group share
// a topic's position and each topic keeps its own. The registry accepts only exact subjects; a wildcard filter needs
// [Client.Consumer] or [Component.Consumer]. Failures are logged as warnings.
func Register(registry *messaging.Registry, cfg nats.Config) error {
	if registry == nil {
		return errors.New("jetstream: messaging registry is nil")
	}
	cfg.ApplyDefaults()
	if err := validateClientConfig(cfg); err != nil {
		return err
	}
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
		client, err := clientFor(ctx, cfg, common)
		if err != nil {
			return nil, err
		}
		return newProducer(client.self, pcfg, client)
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
		ccfg.ApplyDefaults()
		if invalid := ccfg.Validate(); invalid != nil {
			return nil, invalid
		}
		client, err := clientFor(ctx, cfg, common)
		if err != nil {
			return nil, err
		}
		return newConsumer(client.self, ccfg, client)
	})
}

func clientFor(ctx context.Context, cfg nats.Config, common messaging.Config) (*Client, error) {
	cfg.PublishTimeout = common.RequestTimeout
	return New(ctx, cfg)
}

// durableIDLen is the hex length of the topic digest in a registry durable name.
const durableIDLen = 16

// durableName names the registry consumer of topic in group: the group, a dash and a digest of the topic, so two
// topics in one group never share a durable consumer and replace each other's filter.
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
