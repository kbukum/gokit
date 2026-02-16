# grpc

gRPC client library with lazy initialization, generic client wrapper, interceptors, and error mapping.

## Install

```bash
go get github.com/kbukum/gokit/grpc@latest
```

## Quick Start

```go
import (
    grpccfg "github.com/kbukum/gokit/grpc"
    "github.com/kbukum/gokit/grpc/client"
    "github.com/kbukum/gokit/grpc/interceptor"
    "github.com/kbukum/gokit/logger"
)

log := logger.New()
cfg := grpccfg.Config{Host: "localhost", Port: 50051, Enabled: true}

// Direct connection
conn, _ := client.NewClient(cfg, log)
defer conn.Close()

// Lazy generic client — connects on first use
factory := client.NewDefaultConnectionFactory(cfg, log)
lazy := client.NewLazyClient[pb.UserServiceClient]("user-service", factory, pb.NewUserServiceClient, log)
svc, _ := lazy.GetClient()
```

## Key Types & Functions

| Symbol | Description |
|---|---|
| `Config` | Host, Port, TLS, keepalive, message size limits, call timeout |
| `FromGRPC(err, svc)` | Map gRPC error → `*AppError` |
| `ToGRPCStatus(appErr)` | Map `*AppError` → gRPC status error |

### `grpc/client`

| Symbol | Description |
|---|---|
| `NewClient(cfg, log)` | Dial and return `*grpc.ClientConn` |
| `NewDefaultConnectionFactory(cfg, log)` | Factory implementing `ConnectionFactory` |
| `NewLazyClient[T](name, factory, create, log)` | Generic lazy-init client — `GetClient`, `Close`, `Reset` |
| `(*LazyClient[T]) IsConnected()` | Check initialization state |

### `grpc/interceptor`

| Symbol | Description |
|---|---|
| `UnaryClientLoggingInterceptor(log)` | Log unary RPC calls |
| `UnaryClientTimeoutInterceptor(timeout)` | Apply deadline to unary calls |

---

[← Back to main gokit README](../README.md)
