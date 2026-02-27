// Package dag provides a DAG (Directed Acyclic Graph) execution engine
// for orchestrating provider-based service calls in dependency order.
//
// It composes with gokit/provider — each node wraps a RequestResponse[I,O]
// and all existing provider middleware (resilience, stateful, logging, tracing)
// applies per-node without changes.
//
// Two execution modes share the same graph:
//   - ExecuteBatch: runs ALL nodes in dependency order (one-shot)
//   - ExecuteStreaming: runs only nodes whose schedule/condition is met
package dag
