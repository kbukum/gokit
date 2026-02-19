// Package kafka provides Kafka producer and consumer lifecycle management
// as a gokit component.
//
// It wraps confluent-kafka-go with gokit conventions including health checking,
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
// All settings are provided via Config with ApplyDefaults()/Validate():
//
//	kafka:
//	  brokers: ["localhost:9092"]
//	  consumer:
//	    group_id: "my-group"
//	    topics: ["events"]
package kafka
