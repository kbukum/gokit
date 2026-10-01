# grpc

gRPC client and server library with lazy initialization, a generic client wrapper, interceptors, and one table-driven AppError wire (the exact gRPC mirror of the Connect adapter).

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
    "github.com/kbukum/gokit/logging"
)

log := logging.NewDefault("my-service")
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
| `AppErrorToStatus(appErr, service)` | Encode shared google.rpc details; returns `(*status.Status, error)` |
| `DecodeError(err)` | Return a remote `*rpc.Error` or an explicit decode error |
| `IsRetryable(err)` | Read the transient-failure hint, never permission to repeat an operation |
| `RetryDelay(err)` | Read the server's minimum retry delay |

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
| `UnaryClientResilienceInterceptor(policy)` | Apply retry/timeout policy to unary calls |
| `Idempotent()` | Call option explicitly marking an operation safe to repeat |
| `UnaryServerNormalizingInterceptor(log)` | Convert any handler error to a coded status; already-coded errors pass through |
| `StreamServerNormalizingInterceptor(log)` | Streaming counterpart of the unary normalizer |
| `UnaryServerDeadlineInterceptor(max)` | Bound unary RPCs to a server maximum deadline |
| `UnaryServerLoggingInterceptor(log)` | Log incoming RPCs with method, duration, and status |

## Interceptor ordering

When composing interceptors, preserve this order for shared cross-cutting concerns:

1. tracing
2. logging
3. normalization
4. server deadline
5. auth
6. validation
7. handler

For gokit's gRPC client builder, the built-in unary chain is:

1. logging
2. resilience
3. user-supplied interceptors

This keeps logging around the whole RPC while letting resilience set deadlines and retries before custom per-call behavior.

Clients default to timeout-only behavior. Retrying requires both an explicit retry policy and `interceptor.Idempotent()` on the call. The shared resilience owner honors minimum server delays without shortening them to `MaxBackoff`; if the delay cannot fit the remaining budget, it stops. Do not enable a second application or transport retry loop around it.

The [shared error contract](../errors/README.md#shared-rpc-contract) is implemented once in `errors/rpc`. Decoded remote messages are not approved public application messages. Server deadlines cancel cooperative work; they do not forcibly terminate a blocked dependency.

## TLS policy

`grpc.Config.TLS` uses `security.TLSConfig`:

- default floor: TLS 1.2
- default negotiation target: TLS 1.3 whenever the peer supports it
- explicit legacy floors below TLS 1.2 are rejected during validation

Set `MinVersion: tls.VersionTLS13` when a deployment must require TLS 1.3 only.

## Testing

`grpc/testutil` provides an in-process gRPC server for deterministic, network-free transport tests. It runs a real `grpc.Server` over an in-memory bufconn listener, so tests exercise the genuine client stack — dialing, interceptors, deadlines, cancellation, and status propagation — without binding a port.

```go
srv := testutil.NewServer()
defer srv.Stop(ctx)

srv.SetUnaryHandler(func(ctx context.Context) error {
    status, err := grpc.AppErrorToStatus(appErr, "pkg.Service")
    if err != nil {
        return err
    }
    return status.Err()
})

conn, _ := srv.Dial()
err := srv.Invoke(ctx, conn) // drive the client and assert the mapped error
```

`SetUnaryHandler` controls the response: return `nil` for success, a `status` error to prove client-side mapping, or block on `ctx.Done()` to model deadline and cancellation. `Server` also implements `component.Component`/`testutil.TestComponent`.

---

[← Back to main gokit README](../README.md)
