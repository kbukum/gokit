// Package provider implements a generic provider framework using Go generics
// for swappable backends with runtime switching capabilities.
//
// It provides a registry for managing multiple provider implementations with
// factory-based instantiation, availability checking, and runtime selection.
//
// The package defines four interaction patterns:
//   - RequestResponse[I, O]: one input → one output (HTTP, gRPC, subprocess)
//   - Stream[I, O]: one input → many outputs (SSE, streaming gRPC, piped subprocess)
//   - Sink[I]: one input → ack (Kafka produce, webhook, push notification)
//   - Duplex[I, O]: bidirectional (WebSocket, gRPC bidi-stream)
//
// Opt-in lifecycle:
//   - Initializable: providers that need setup (dial gRPC, validate binary)
//   - Closeable: providers that hold resources (connections, daemon processes)
//
// # Usage
//
//	reg := provider.NewRegistry[MyProvider]()
//	reg.RegisterFactory("default", myFactory)
//	mgr := provider.NewManager(reg, &provider.HealthCheckSelector[MyProvider]{})
//	mgr.InitializeWithContext(ctx, "default", cfg)
//	p, _ := mgr.Get(ctx)
package provider
