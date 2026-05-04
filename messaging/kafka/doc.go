// Package kafka provides Kafka producer and consumer lifecycle management
// as a gokit component.
//
// It wraps segmentio/kafka-go with gokit conventions including health checking,
// graceful shutdown, metrics collection, and structured logging for event
// streaming integration.
//
// # Architecture
//
//   - Component: Manages producer/consumer lifecycle (Init/Start/Stop/Health)
//   - kafka/producer: Message publishing with delivery guarantees
//   - kafka/consumer: Message consumption with managed consumer groups
//
// # Configuration
//
// Kafka-specific connection/protocol settings are provided via Config with
// ApplyDefaults()/Validate(). Broker-neutral policy (name, enabled, retries,
// request timeout, consumer group, topics, delivery guarantee, commit strategy,
// DLQ, and max in-flight) belongs in messaging.Config and is mapped by the
// registry/direct constructors.
//
//	messaging:
//	  backend: kafka
//	  consumer_group: "my-group"
//	  topics: ["events"]
//	kafka:
//	  brokers: ["localhost:9092"]
//	  compression: snappy
package kafka
