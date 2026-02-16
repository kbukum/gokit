# redis

Go-redis wrapper with connection pooling, health checks, and component lifecycle management.

## Install

```bash
go get github.com/kbukum/gokit/redis@latest
```

## Quick Start

```go
import (
    "github.com/kbukum/gokit/redis"
    "github.com/kbukum/gokit/logger"
)

log := logger.New()
comp := redis.NewComponent(redis.Config{
    Enabled:  true,
    Addr:     "localhost:6379",
    Password: "",
    DB:       0,
    PoolSize: 10,
}, log)

comp.Start(ctx)
defer comp.Stop(ctx)

client := comp.Client()
client.Set(ctx, "key", "value", time.Hour)
val, err := client.Get(ctx, "key")
client.Del(ctx, "key")
```

## Key Types & Functions

| Symbol | Description |
|---|---|
| `Component` | Managed lifecycle wrapper — `Start`, `Stop`, `Health` |
| `Config` | Addr, password, DB index, pool size, timeouts |
| `Client` | Redis operations — `Get`, `Set`, `Del`, `Exists`, `Ping` |
| `NewComponent(cfg, log)` | Create a managed redis component |
| `New(cfg, log)` | Create a standalone `*Client` without lifecycle |
| `(*Client) Unwrap()` | Access the underlying `*goredis.Client` for advanced operations |

---

[← Back to main gokit README](../README.md)
