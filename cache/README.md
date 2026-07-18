# cache

Backend-neutral cache abstraction for gokit.

- Core ships the `Store` contract, explicit `FactoryRegistry`, typed JSON store,
  component integration, and in-memory backend.
- External backends are opt-in adapter modules. Redis lives under `cache/redis`
  and registers only when `redis.Register(registry)` is called.
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
