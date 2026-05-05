# gokit/messaging

Transport-agnostic message producer/consumer abstraction with explicit adapter registration, in-memory broker for testing, and composable middleware.

## Overview

The `messaging` module provides a unified interface for publishing and consuming messages across different transports. It defines core types (`Message`, `Event`), producer/consumer interfaces, and higher-level patterns like routing, batching, and managed consumers — all independent of any specific broker.

Concrete implementations (Kafka, NATS, RabbitMQ, in-memory) plug into these interfaces, so application code stays transport-agnostic. A middleware system allows cross-cutting concerns (retry, dead-letter, tracing, deduplication, metrics) to be composed around any handler.

## Installation

```bash
go get github.com/kbukum/gokit/messaging
```

## Quick Start

```go
package main

import (
	"context"
	"fmt"

	"github.com/kbukum/gokit/messaging"
	"github.com/kbukum/gokit/messaging/memory"
)

func main() {
	ctx := context.Background()

	// Create an in-memory broker (great for testing)
	broker := memory.NewBroker()
	producer := broker.Producer()
	consumer := broker.Consumer("user.events")

	// Publish a structured event
	event, _ := messaging.NewEvent("user.created", "auth-service", map[string]string{
		"id": "user-123", "email": "user@example.com",
	}, "user-123")

	_ = producer.Publish(ctx, "user.events", event)

	// Consume messages
	go consumer.Consume(ctx, func(ctx context.Context, msg messaging.Message) error {
		fmt.Printf("Received: %s\n", msg.Value)
		return nil
	})
}
```

## API Reference

### Core Types

| Type | Description |
|------|-------------|
| `Message` | Low-level broker message with key, value, topic, partition, offset, headers |
| `NewMessage` | Broker-neutral constructor for low-level messages with non-nil headers |
| `Event` | Structured domain event with ID, type, source, timestamp, and JSON data |
| `Producer` | Interface with `Publish`, `PublishJSON`, and `PublishBinary` methods |
| `Consumer` | Interface with blocking `Consume` loop and `Topic`/`Close` methods |
| `MessageHandler` | `func(ctx, Message) error` — the core handler signature |
| `HandlerMiddleware` | `func(MessageHandler) MessageHandler` — composable middleware |
| `ErrorClassifier` | Categorizes errors as connection or retryable |
| `BrokerComponent` | Factory + lifecycle interface for broker implementations |

### Publishing Methods

- `Publish(ctx, topic, event, key...)` — structured Event with headers
- `PublishJSON(ctx, topic, key, value)` — direct JSON marshaling
- `PublishBinary(ctx, topic, key, data)` — raw bytes (protobuf, avro)

### Higher-Level Patterns

- `NewRouter()` — route messages by topic with exact match, wildcard (`*`), and default fallback
- `NewBatchProducer(producer, topic, cfg)` — buffer and flush on size, time, or byte thresholds
- `NewManagedConsumer(cfg)` — background consumer lifecycle with start/stop/status
- `ChainHandlers(base, mw...)` — compose handler middlewares in order

## Sub-Packages

Broker SDKs live in opt-in nested modules (`messaging/kafka`, `messaging/nats`, and
`messaging/rabbitmq`) so importing core `messaging` only pulls abstractions, registry,
middleware, and the in-memory default into the module graph. Adapter packages register
factories only through explicit config-free `Register(registry)` calls; runtime config is
passed when creating producer/consumer instances. They do not use `init` registration side effects.

Core `messaging.Config` owns only broker-neutral policy: instance `Name`, `Enabled`,
`Adapter`, delivery guarantee, commit strategy, DLQ policy, max in-flight,
consumer group, allowed topics/subscriptions, request timeout, and retry attempts/
backoff. Adapter configs contain only provider-specific connection/protocol knobs:
Kafka keeps brokers/resolve, TLS/SASL, compression, required acks, batch settings,
session/heartbeat/rebalance tuning, and dial/idle/metadata TTLs; NATS keeps URL,
auth, TLS, reconnect, drain, queue-group, and subject-prefix settings; RabbitMQ keeps
URL, username/password, TLS, exchange/queue/routing, heartbeat, prefetch, and AMQP
timeouts. Factories explicitly map or reject common semantics before dialing; no
adapter uses `init` registration side effects or package-level mutable registries.
Kafka, NATS, and RabbitMQ SDKs stay isolated to their subpackages; importing core
`messaging` or `messaging/memory` does not pull optional broker SDKs.

### `kafka/` — Kafka Implementation

Production-ready Kafka producer and consumer using `segmentio/kafka-go`. Supports TLS, SASL authentication, consumer groups, compression, and configurable batching.

```go
import (
	"github.com/kbukum/gokit/messaging"
	"github.com/kbukum/gokit/messaging/kafka"
	kafkaproducer "github.com/kbukum/gokit/messaging/kafka/producer"
)

common := messaging.Config{
	Adapter:        "kafka",
	Name:           "my-service-producer",
	Topics:         []string{"events"},
	ConsumerGroup:  "my-service",
	Subscriptions:  []string{"events"},
	RequestTimeout: "10s",
}

cfg := kafka.Config{
	Brokers:     []string{"kafka.internal:9093"},
	Compression: "snappy",
	// TLS is enabled by default; plaintext local endpoints require AllowInsecureDev.
}

producer, _ := kafkaproducer.NewProducer(common, cfg, log)
defer producer.Close()

_ = producer.Publish(ctx, "events", event)
```

### `nats/` — NATS Implementation

Opt-in NATS adapter using `github.com/nats-io/nats.go`, with typed connection, auth, timeout, reconnect, and subject settings.

```go
import natsadapter "github.com/kbukum/gokit/messaging/nats"

reg := messaging.NewRegistry()
_ = natsadapter.Register(reg)
producer, _ := reg.NewProducer(ctx,
	messaging.Config{
		Adapter:           "nats",
		DeliveryGuarantee: messaging.DeliveryAtMostOnce,
		CommitStrategy:    messaging.CommitAuto,
		Topics:            []string{"events"},
		RequestTimeout:    "5s",
	},
	&natsadapter.Config{URL: "tls://nats.internal:4222"},
	log,
)
```

### `rabbitmq/` — RabbitMQ Implementation

Opt-in RabbitMQ adapter using `github.com/rabbitmq/amqp091-go`, with typed connection, exchange, queue, acknowledgement, prefetch, timeout, and TLS settings.

```go
import (
	"os"

	rabbitmqadapter "github.com/kbukum/gokit/messaging/rabbitmq"
)

reg := messaging.NewRegistry()
_ = rabbitmqadapter.Register(reg)
producer, _ := reg.NewProducer(ctx,
	messaging.Config{Adapter: "rabbitmq", Topics: []string{"events"}, RequestTimeout: "5s"},
	&rabbitmqadapter.Config{
		URL:      "amqps://rabbitmq.internal:5671/",
		Username: os.Getenv("RABBITMQ_USERNAME"),
		Password: os.Getenv("RABBITMQ_PASSWORD"),
	},
	log,
)
```

### `memory/` — In-Memory Broker

Channel-based broker for unit and integration tests. No external dependencies.

```go
broker := memory.NewBroker()
producer := broker.Producer()
consumer := broker.Consumer("my-topic")

// Publish and assert
_ = producer.PublishBinary(ctx, "my-topic", "key", []byte("hello"))
memory.AssertPublished(t, broker, "my-topic", func(msg messaging.Message) bool {
	return string(msg.Value) == "hello"
})
```

### `middleware/` — Composable Middleware

Cross-cutting concerns that wrap any `MessageHandler`:

| Middleware | Description |
|------------|-------------|
| `RetryHandler` | Automatic retry with configurable backoff |
| `DeadLetterProducer` | Opt-in DLQ routing with canonical `original_topic`, `error`, `retry_count`, `timestamp`, `headers`, and `payload` fields plus redaction |
| `TracingHandler` | OpenTelemetry trace context propagation |
| `CircuitBreakerHandler` | Circuit breaker around message processing |
| `DedupHandler` | LRU-based message deduplication with TTL |
| `InstrumentHandler` | OTel counters and histogram for processing metrics |

```go
handler := messaging.ChainHandlers(
	baseHandler,
	middleware.RetryHandler(retryConfig),
	middleware.TracingHandler,
	middleware.InstrumentHandler("events", "my-service"),
)
```

### `bridge/` — Provider Integration

Adapts messaging primitives to the gokit provider pattern for use in DAGs and pipelines:

- `ProducerAsSink` — wraps a Producer as a `provider.Sink[Message]`
- `EventProducerAsSink` — wraps a Producer as a `provider.Sink[Event]`
- `ConsumerAsStream` — wraps a Consumer as a `provider.Stream`

### `testutil/` — Test Mocks

Broker-agnostic mocks for unit testing:

- `MockProducer` — records published messages with error injection
- `ChannelConsumer` — pre-fed message consumer for handler testing
- `Component` — mock `BrokerComponent` for lifecycle tests

## Security and DLQ defaults

Broker adapters are secure by default. Kafka requires TLS unless `AllowInsecureDev` is set;
NATS requires `tls://` or `wss://`; RabbitMQ requires `amqps://`. Credentials are provided
through typed config fields, not broker URLs or hardcoded examples. Topic, subject, queue,
and consumer-group names are validated before construction or use.

DLQ routing is disabled until explicitly configured. The shared config carries DLQ intent,
but adapters that cannot provide broker-managed DLQ reject enabled adapter DLQ settings and
expect callers to wire the broker-agnostic `DeadLetterProducer` middleware instead.

## Validation

Focused docs checks verify that NATS/RabbitMQ are documented as real opt-in adapters, examples
avoid hardcoded credentials, and core docs do not imply optional SDK imports. Final workspace
validation counts should come from CI or the dedicated validation pass.

## Testing

```bash
cd messaging
go test -race ./...
```

## Contributing

Please refer to the root [CONTRIBUTING.md](../CONTRIBUTING.md) for guidelines.
