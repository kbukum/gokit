# cache

Backend-neutral cache abstraction for gokit.

- Core ships the `Store` contract, explicit `FactoryRegistry`, typed JSON store, component integration, and in-memory backend.
- External backends are opt-in adapter modules. Redis lives under `cache/redis` and registers only when `redis.Register(registry)` is called.
- There is no package-level mutable registry and no import-time backend registration.

## In-memory default

```go
reg := cache.NewFactoryRegistry()
if err := cache.RegisterMemory(reg); err != nil {
    return err
}

store, err := cache.New(reg, cache.Config{
    Provider: cache.ProviderMemory,
}, nil, log)
```

A memory store can be bounded and driven by an injected clock. `MaxEntries` caps how many keys it holds; adding a new key to a full store evicts the least recently used entry, and replacing a key never evicts. `NewMemoryStore` validates the configuration and returns an error for a negative TTL or bound.

```go
store, err := cache.NewMemoryStore(cache.MemoryConfig{
    DefaultTTL: 5 * time.Second,
    MaxEntries: 10_000,
    Clock:      clock, // util.Clock; nil means the system clock
})
```

## Redis adapter

```go
import cacheredis "github.com/kbukum/gokit/cache/redis"

reg := cache.NewFactoryRegistry()
if err := cacheredis.Register(reg); err != nil {
    return err
}

store, err := cache.New(reg, cache.Config{
    Provider: cache.ProviderRedis,
    Enabled:  true,
}, &cacheredis.Config{
    Enabled: true,
    Addr:    "127.0.0.1:6379",
}, log)
```
