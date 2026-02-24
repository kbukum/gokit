// Package redis provides a Redis client component with connection pooling,
// lifecycle management, and health checks for gokit applications.
//
// It wraps go-redis with gokit logging, configuration conventions, and
// component lifecycle (Init/Start/Stop/Health) for cache and session
// storage operations.
//
// # Typed Operations
//
// TypedStore provides generic JSON-serialized get/set operations and
// implements provider.ContextStore[C] for use with provider.Stateful:
//
//	store := redis.NewTypedStore[MyState](client, "sessions")
//	stateful := provider.NewStateful(provider.StatefulConfig[In, Out, MyState]{
//	    Store: store, ...
//	})
//
// For ad-hoc typed operations, use GetJSON/SetJSON on the Client directly:
//
//	client.SetJSON(ctx, "key", myStruct, 5*time.Minute)
//	client.GetJSON(ctx, "key", &myStruct)
//
// # Quick Start
//
//	cfg := redis.Config{
//	    Host: "localhost",
//	    Port: 6379,
//	}
//	component := redis.NewComponent(cfg)
package redis
