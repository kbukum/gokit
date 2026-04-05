# gokit/messaging

Transport-agnostic message producer/consumer abstraction with Kafka provider, in-memory broker for testing, and composable middleware.

## Overview

The `messaging` module provides a unified interface for publishing and consuming messages across different transports. It defines core types (`Message`, `Event`), producer/consumer interfaces, and higher-level patterns like routing, batching, and managed consumers — all independent of any specific broker.

Concrete implementations (Kafka, in-memory) plug into these interfaces, so application code stays transport-agnostic. A middleware system allows cross-cutting concerns (retry, dead-letter, tracing, deduplication, metrics) to be composed around any handler.

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

### `kafka/` — Kafka Implementation

Production-ready Kafka producer and consumer using `segmentio/kafka-go`. Supports TLS, SASL authentication, consumer groups, compression, and configurable batching.

```go
import (
	"github.com/kbukum/gokit/messaging/kafka"
	kafkaproducer "github.com/kbukum/gokit/messaging/kafka/producer"
)

cfg := kafka.Config{
	Brokers:     []string{"localhost:9092"},
	GroupID:     "my-service",
	Compression: "snappy",
}
cfg.ApplyDefaults()

producer, _ := kafkaproducer.NewProducer(cfg, log)
defer producer.Close()

_ = producer.Publish(ctx, "events", event)
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
| `DeadLetterProducer` | Route failed messages to a dead-letter topic |
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

## Testing

```bash
cd messaging
go test -race ./...
```

## Contributing

Please refer to the root [CONTRIBUTING.md](../CONTRIBUTING.md) for guidelines.
