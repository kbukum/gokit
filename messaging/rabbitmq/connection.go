package rabbitmq

import (
	"fmt"
	"net"
	"strings"

	amqp "github.com/rabbitmq/amqp091-go"
)

func dial(cfg Config) (*amqp.Connection, error) {
	tlsCfg, err := cfg.TLS.Build()
	if err != nil {
		return nil, fmt.Errorf("rabbitmq tls: %w", err)
	}
	connectionURL, err := cfg.connectionURL()
	if err != nil {
		return nil, fmt.Errorf("rabbitmq: invalid url")
	}
	dialer := net.Dialer{Timeout: mustDuration(cfg.ConnectionTimeout)}
	return amqp.DialConfig(connectionURL.String(), amqp.Config{
		Heartbeat:       mustDuration(cfg.Heartbeat),
		TLSClientConfig: tlsCfg,
		Dial:            dialer.Dial,
	})
}

func queueName(cfg Config, topic string) string {
	if cfg.QueueName != "" {
		return cfg.QueueName
	}
	prefix := strings.Trim(cfg.QueuePrefix, ".")
	if prefix == "" {
		return routingKey(cfg, topic)
	}
	return prefix + "." + topic
}

func routingKey(cfg Config, topic string) string {
	prefix := strings.Trim(cfg.RoutingKeyPrefix, ".")
	if prefix == "" {
		return topic
	}
	return prefix + "." + topic
}

func declareExchange(ch *amqp.Channel, cfg Config) error {
	if cfg.Exchange == "" {
		return nil
	}
	if err := ch.ExchangeDeclare(cfg.Exchange, cfg.ExchangeType, cfg.ExchangeDurable, false, false, false, nil); err != nil {
		return fmt.Errorf("rabbitmq declare exchange: %w", err)
	}
	return nil
}

func bindQueue(ch *amqp.Channel, cfg Config, queue, bindingKey string) error {
	if cfg.Exchange == "" {
		return nil
	}
	if err := ch.QueueBind(queue, bindingKey, cfg.Exchange, false, nil); err != nil {
		return fmt.Errorf("rabbitmq bind queue: %w", err)
	}
	return nil
}
