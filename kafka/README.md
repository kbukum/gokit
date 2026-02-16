# kafka

Kafka producer and consumer with managed lifecycle, TLS/SASL, retries, and structured event messaging.

## Install

```bash
go get github.com/kbukum/gokit/kafka@latest
```

## Quick Start

```go
import (
    "github.com/kbukum/gokit/kafka"
    "github.com/kbukum/gokit/kafka/producer"
    "github.com/kbukum/gokit/kafka/consumer"
)
cfg := kafka.Config{Enabled: true, Brokers: []string{"localhost:9092"}, GroupID: "my-group"}
p, _ := producer.NewProducer(cfg, log)
p.SendJSON(ctx, "events", "user-123", map[string]any{"action": "login"})

c, _ := consumer.NewConsumer(cfg, "events", log)
c.Consume(ctx, func(ctx context.Context, msg kafkago.Message) error {
    return processMessage(msg)
})

comp := kafka.NewComponent(cfg, log)
comp.SetProducer(p)
comp.AddConsumer(c)
comp.Start(ctx)
```

## Key Types & Functions

| Symbol | Description |
|---|---|
| `Component` | Managed lifecycle — `Start`, `Stop`, `Health` |
| `Config` | Brokers, TLS, SASL, compression, retries, timeouts |
| `Message` | Key, Value, Topic, Headers — `FromKafkaMessage`, `ToKafkaMessage` |
| `Event` | Structured domain event — ID, Type, Source, Data |

### `kafka/producer`

| Symbol | Description |
|---|---|
| `NewProducer(cfg, log)` | Create a kafka-go writer with TLS/SASL |
| `(*Producer) SendJSON(ctx, topic, key, value)` | Publish JSON-encoded message |
| `NewPublisher(producer, log)` | High-level `Publisher` — `Publish`, `PublishJSON` |

### `kafka/consumer`

| Symbol | Description |
|---|---|
| `NewConsumer(cfg, topic, log)` | Create a kafka-go reader |
| `(*Consumer) Consume(ctx, handler)` | Blocking consume loop |

---

[← Back to main gokit README](../README.md)
