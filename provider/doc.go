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
// # State Management
//
// The Stateful wrapper adds automatic state load/save around provider execution:
//
//	store := provider.NewMemoryStore[MyState]()
//	stateful := provider.NewStateful(provider.StatefulConfig[In, Out, MyState]{
//	    Inner:   myProvider,
//	    Store:   store,
//	    KeyFunc: func(in In) string { return in.SessionID },
//	    Inject:  func(in In, s *MyState) In { /* enrich input */ },
//	    Extract: func(in In, out Out) *MyState { /* derive state */ },
//	    TTL:     5 * time.Minute,
//	})
//
// ContextStore[C] is the state persistence interface; MemoryStore is the
// built-in in-memory implementation for testing. Production implementations
// (e.g., redis.TypedStore) live in sub-modules to avoid dependency coupling.
//
// # Middleware
//
// Middleware[I, O] is a function that wraps a RequestResponse provider.
// Use Chain to compose multiple middlewares:
//
//	wrapped := provider.Chain(
//	    provider.WithLogging[In, Out](log),
//	    provider.WithMetrics[In, Out](metrics),
//	    provider.WithTracing[In, Out]("my-service"),
//	)(rawProvider)
//
// SinkMiddleware[I] is the Sink equivalent. Use ChainSink to compose:
//
//	wrapped := provider.ChainSink(mw1, mw2)(rawSink)
//
// # Sink Combinators
//
// Sinks support composable combinators for push-based data flow:
//
//   - NewSinkFunc: wraps a plain func(ctx, I) error as a Sink (like http.HandlerFunc)
//   - FanOutSink: dispatches each input to multiple sinks in parallel
//   - AdaptSink: transforms input types before sending (bridges domain → backend)
//   - TapSink: adds a side-effect before forwarding to the inner sink
//   - WithSinkResilience: wraps a Sink with retry, circuit breaker, and rate limiting
//
// These compose naturally for push-based processing pipelines:
//
//	sink := provider.FanOutSink("multi",
//	    kafkaSink,
//	    provider.AdaptSink(analyticsSink, "adapt", mapFn),
//	    provider.TapSink(loggingSink, metricsObserver),
//	)
//
// # Usage
//
//	reg := provider.NewRegistry[MyProvider]()
//	reg.RegisterFactory("default", myFactory)
//	mgr := provider.NewManager(reg, &provider.HealthCheckSelector[MyProvider]{})
//	mgr.InitializeWithContext(ctx, "default", cfg)
//	p, _ := mgr.Get(ctx)
package provider
