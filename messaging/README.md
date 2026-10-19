# gokit/messaging

Transport-agnostic message producer/consumer abstraction with explicit adapter registration, in-memory broker for testing, and composable middleware.

The local `memory` broker delivers only to current subscribers. It retains no message history, and publishing to a topic without subscribers does not allocate persistent topic storage. Subscriber queues default to 256 messages; a full queue returns an error. Close consumers to release their subscriptions, and close the broker to interrupt all blocked reads. For complete publication recording and assertions, use `messaging/testutil.MockProducer`, `MessagesForTopic`, and the testutil assertion helpers.

With `CommitAfterHandlerSuccess`, a failed delivery stays in one consumer-owned retry slot and `Consume` returns the handler error immediately. The next `Consume` retries it before queued messages, without waiting for queue space or redelivering it to other subscribers. Each consumer permits one active `Consume` call. The live retention bound is its configured queue capacity plus this single retry slot.

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
		fmt.Printf("Received: %s\n", msg.Payload)
		return nil
	})
}
```

## API Reference

### Core Types

| Type | Description |
|------|-------------|
| `Message` | Low-level broker message with key, payload, topic, partition, offset, headers |
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
- `NewManagedConsumer(cfg)` — runs a consumer in the background until Stop: optional restart with doubling delays, a per-message `HandleTimeout`, a `ConsumerObserver` for handled messages and failures (`LoggingObserver` logs them), `MessageAge` for lag, and health that is degraded while restarting or while a consumer implementing `ConsumerStatus` is not receiving. Stop closes the consumer but waits only as long as its context allows; a slow close finishes after Stop returns
- `ChainHandlers(base, mw...)` — compose handler middlewares in order

## Sub-Packages

Broker SDKs live in opt-in nested modules (`messaging/kafka`, `messaging/nats`, and `messaging/rabbitmq`) so importing core `messaging` only pulls abstractions, registry, middleware, and the in-memory default into the module graph. Adapter packages register factories only through explicit config-free `Register(registry)` calls; runtime config is passed when creating producer/consumer instances. They do not use `init` registration side effects.

Core `messaging.Config` owns only broker-neutral policy: instance `Name`, `Enabled`, `Adapter`, delivery guarantee, commit strategy, DLQ policy, max in-flight, consumer group, allowed topics/subscriptions, request timeout, and retry attempts/ backoff. Adapter configs contain only provider-specific connection/protocol knobs: Kafka keeps brokers/resolve, TLS/SASL, compression, required acks, batch settings, session/heartbeat/rebalance tuning, and dial/idle/metadata TTLs; NATS keeps URL, auth, TLS, reconnect, drain, queue-group, and subject-prefix settings; RabbitMQ keeps URL, username/password, TLS, exchange/queue/routing, heartbeat, prefetch, and AMQP timeouts. Factories explicitly map or reject common semantics before dialing; no adapter uses `init` registration side effects or package-level mutable registries. Kafka, NATS, and RabbitMQ SDKs stay isolated to their subpackages; importing core `messaging` or `messaging/memory` does not pull optional broker SDKs.

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

Credentials come from `Token`, `Username` with `Password`, or `Username` with `PasswordFile` (a bounded mounted secret, read again at every connect so a rotated password applies on reconnect).

#### `nats/jetstream/` — JetStream

The `nats` adapter is core NATS: at most once, no persistence. For durable, at-least-once delivery use the `jetstream` adapter in the same module. It implements `messaging.Producer` and `messaging.Consumer` and registers as `"jetstream"`.

- `Client.EnsureStream` declares a file-backed stream with limits retention, a message size cap and a deduplication window. Like Kafka's `EnsureTopics`, it is outside the shared ports.
- `Producer` stores each message and waits for the server's acknowledgement, bounded by `PublishTimeout`. The `Nats-Msg-Id` header (`MsgIDHeader`, matched in any letter case) is the deduplication ID: a republished message within the window is stored once. `Publish` sets it from `Event.ID`. Only a message with an ID is retried, since a lost acknowledgement could otherwise store it twice. Failures that cannot pass, such as a refused or oversized message, are not retried.
- `Consumer` declares a durable pull consumer with explicit acknowledgement, or binds to an existing one with `Bind`. A consumer that owns its stream sets `Declare` to declare the stream again whenever it is missing; otherwise a missing stream is retried until another process declares it. It fetches bounded batches and calls the handler in order, marking each message in progress first. `nil` acknowledges and waits for the server's confirmation. An error naks with a delay that doubles per delivery up to `MaxRetryDelay`, and consumption continues. `Delivery` (a `DeliveryPolicy`) decides when to give up, and the default never drops a message: it retries until the handler succeeds. Setting `MaxAttempts` also needs a choice for what happens next: `OnDeadLetter` hands the message off, or `DiscardExhausted` drops it and reports `ErrDeliveryLimit`. An `ErrTerminal` error skips the remaining attempts. The broker keeps redelivering until the hand-off succeeds, so a failed or interrupted hand-off is tried again without running the handler again. `Bind` refuses a consumer that could lose a failed message, one without explicit acks or with a server-side delivery limit, with `ErrUnsafeBoundConsumer`. Registry consumers use `jetstream.WithDeliveryPolicy`. `OnError` (required) receives adapter failures only, such as broker, bind and dead-letter failures; handler errors stay with the handler and its observers. Broker failures are retried with backoff, binding again after a failed fetch. `Consume` returns when its context ends, the consumer closes, or the broker refuses the consumer for good. `Message.Offset` is the stream sequence.
- `Client.Replay` reads one page (up to `MaxReplayLimit`) of retained messages for a subject filter from a stream sequence or time, through a short-lived consumer it deletes afterwards. `Next` is the checkpoint for the following page and never skips a message stored during the call.

Every JetStream API request is bounded by `PublishTimeout`. `ErrorClassifier` tells retryable failures from permanent ones. The package documentation lists the broker rights each operation needs.

Handlers must be idempotent, for example by recording the message ID in the same transaction as the effect. Subjects are absolute; `SubjectPrefix` and `QueueGroup` are refused. `New(ctx, cfg)` bounds the first connection attempt by `ctx`, then reconnects in the background, including when the server is not reachable at construction. A zero `MaxReconnects` means unlimited; a positive bound lets the connection close for good, after which every operation fails.

`NewComponent(name, cfg)` validates the configuration and returns an unstarted component that owns a client in a component registry: `Start` connects, `Stop(ctx)` drains and closes within `ctx`, and `Health` is degraded while the broker is unreachable, so a service keeps serving while it reconnects. It is unhealthy once the connection closed for good. Its `Producer`, `Consumer`, `EnsureStream` and `Replay` refuse calls before `Start`.

Through the registry, the configuration must be at-least-once, commit after handler success, `MaxInFlight` 1, without a dead-letter topic. The producers and consumers one `Register` call builds share one connection, closed when the last of them closes. Each topic gets its own durable consumer, named from the consumer group and the topic. Topics are exact subjects; use `Client.Consumer` for wildcard filters.

```go
import "github.com/kbukum/gokit/messaging/nats/jetstream"

client, err := jetstream.New(ctx, natsadapter.Config{
	URL: "tls://nats.internal:4222", Username: "runs", PasswordFile: "/run/secrets/nats_password",
})
// handle err; defer client.Close()
producer, err := client.Producer(jetstream.ProducerConfig{Stream: "FACTS"})
err = producer.Send(ctx, messaging.Message{
	Topic: "facts.acme.run", Payload: payload, Headers: map[string]string{jetstream.MsgIDHeader: eventID},
})
```

`messaging/nats/testutil` starts an authenticated single-node JetStream server for integration tests, with optional restricted users and a restart on the same address.

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
	return string(msg.Payload) == "hello"
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

Broker adapters are secure by default. Kafka requires TLS unless `AllowInsecureDev` is set; NATS requires `tls://` or `wss://`; RabbitMQ requires `amqps://`. Credentials are provided through typed config fields, not broker URLs or hardcoded examples. Topic, subject, queue, and consumer-group names are validated before construction or use.

DLQ routing is disabled until explicitly configured. The shared config carries DLQ intent, but adapters that cannot provide broker-managed DLQ reject enabled adapter DLQ settings and expect callers to wire the broker-agnostic `DeadLetterProducer` middleware instead.

## Testing

```bash
make test M=messaging
```

## Contributing

Please refer to the root [CONTRIBUTING.md](../CONTRIBUTING.md) for guidelines.
