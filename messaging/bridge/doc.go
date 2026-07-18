// Package bridge provides provider adapters that connect messaging primitives (Producer, Consumer) to the gokit provider pattern.
//
// ProducerAsSink wraps a Producer as a provider.Sink[Message].
// EventProducerAsSink wraps a Producer as a provider.Sink[Event].
// ConsumerAsStream wraps a Consumer as a provider.Stream returning messages.
//
// Once messaging components are expressed as providers,
// they compose naturally with all other kit patterns that accept providers:
//
//   - DAG: dag.FromProvider(sink) creates a DAG node
//   - Worker: worker.FromProvider(sink) creates a worker handler
//   - Stream: stream.From(iter) or stream.Drain(sink.Send)
package bridge
