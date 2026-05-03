# cache/redis

Opt-in Redis adapter for `github.com/kbukum/gokit/cache`.

Importing this package has no side effects. Register the backend explicitly:

```go
reg := cache.NewFactoryRegistry()
if err := redis.Register(reg); err != nil {
    return err
}

store, err := cache.New(reg, cache.Config{
    Provider: cache.ProviderRedis,
    Enabled:  true,
}, &redis.Config{
    Enabled: true,
    Addr:    "localhost:6379",
}, log)
```

The adapter depends on `github.com/redis/go-redis/v9`; the core cache module does not.
