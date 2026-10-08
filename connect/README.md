# connect

Connect-Go integration for gokit — server-side interceptors, error mapping, service mounting, and client utilities.

## Install

```bash
go get github.com/kbukum/gokit/connect@latest
```

## Quick Start — Server

```go
import (
    goconnect "github.com/kbukum/gokit/connect"
    "github.com/kbukum/gokit/logging"
    "connectrpc.com/connect"
)

log := logging.NewDefault("my-service")

// Create Connect service handler with interceptors
path, handler := userv1connect.NewUserServiceHandler(svc,
    connect.WithInterceptors(goconnect.LoggingInterceptor(log), goconnect.NormalizingInterceptor(log)),
)

// Mount on any server implementing HandlerMounter (e.g. gokit/server.Server)
goconnect.Mount(srv, path, handler)
```

## Quick Start — Client

```go
import (
    "github.com/kbukum/gokit/connect/client"
)

cfg := client.Config{BaseURL: "http://localhost:8080"}
httpClient, err := client.NewHTTPClient(cfg)
if err != nil {
    return err
}
svcClient := userv1connect.NewUserServiceClient(httpClient, cfg.BaseURL)

// For bidi streaming, use gRPC protocol:
cfg := client.Config{BaseURL: "http://localhost:8080", Protocol: client.ProtocolGRPC}
httpClient, err := client.NewHTTPClient(cfg)
opts := client.ClientOptions(cfg)
svcClient := userv1connect.NewUserServiceClient(httpClient, cfg.BaseURL, opts...)
```

The client uses native HTTP/2 transport controls. Enabled TLS requires `https://`; the default h2c mode requires `http://`. Each request, including redirects, is checked before dialing, so neither mode silently falls back to HTTP/1. Call `httpClient.CloseIdleConnections()` when its owner shuts down.

## Typed authentication

Authenticate the complete HTTP request before it reaches Connect. The injected authentication provider owns cookie/API-key precedence, repeated credentials, CSRF for unsafe cookie requests, expiry, and revocation. A unary interceptor cannot reconstruct those semantics from RPC headers. Connect has no dependency on an authentication implementation or JWT library.

Use `server/middleware.HTTPAuth[T]` around the protected Connect handler and inject the paired context getter into `AuthInterceptor[T]` and `RequireAuth[T]`. The composition root can use `authctx.Set[T]` and `authctx.Get[T]`; service code sees one concrete identity type regardless of credential. Supply `WithAuthErrorWriter` with a protocol-aware writer when the outer boundary must return a Connect error envelope. Normalize the authentication error and encode it with the shared Connect error contract rather than exposing private causes or serializing an application error directly.

```go
import (
    "github.com/kbukum/gokit/auth"
    "github.com/kbukum/gokit/auth/authctx"
    goconnect "github.com/kbukum/gokit/connect"
    "github.com/kbukum/gokit/server/middleware"
    "connectrpc.com/connect"
)

// authenticator implements Authenticate(*http.Request) (auth.Principal, bool, error), e.g. auth.NewChain.
authenticate, err := middleware.HTTPAuth(authenticator, authctx.Set[auth.Principal],
    middleware.WithAuthErrorWriter(writeConnectFailure),
)
if err != nil {
    return err
}
requireIdentity, err := goconnect.AuthInterceptor(authctx.Get[auth.Principal])
if err != nil {
    return err
}
path, handler := userv1connect.NewUserServiceHandler(svc,
    connect.WithInterceptors(
        goconnect.NormalizingInterceptor(log),
        requireIdentity,
    ),
)
goconnect.Mount(srv, path, authenticate(handler))
```

Inside a service handler:

```go
principal, err := goconnect.RequireAuth(ctx, authctx.Get[auth.Principal])
if err != nil {
    return nil, err
}
// Pass principal to the application's injected authorization policy.
```

The guard rejects a missing or typed-nil identity with `CodeUnauthenticated`. It never turns a supplied token into an identity by itself, and it preserves the request context, cancellation, and downstream errors. `RequireAuth` uses the same shared error normalizer as other Connect failures.

For an explicitly public operation, omit the required-identity guard and call the injected getter directly. The outer authentication policy must still reject malformed, duplicate, expired, or otherwise invalid presented credentials; only genuinely absent credentials may become anonymous access. Never use a query token or a token-validation fallback inside a unary interceptor.

## Key Types & Functions

| Symbol | Description |
|---|---|
| `Config` | Server-side config: SendMaxBytes, ReadMaxBytes, Enabled |
| `Service` | Interface — `Path() string`, `Handler() http.Handler` |
| `NewService(path, handler)` | Create a Service from path and handler |
| `HandlerMounter` | Interface for servers that mount HTTP handlers |
| `Mount(srv, path, handler)` | Mount a single Connect handler on any HandlerMounter |
| `MountServices(srv, ...Service)` | Mount multiple services at once |
| `LoggingInterceptor(log)` | Log RPC calls with duration and status |
| `NormalizingInterceptor(log)` | Convert any handler error to a coded Connect error (unary + streaming); already-coded errors pass through |
| `ValidationInterceptor(v)` | Validate requests with an injected `protovalidate.Validator`, emitting shared violations |
| `DeadlineInterceptor(max)` | Bound unary RPCs to a server maximum deadline |
| `AuthInterceptor[T](getClaims)` | Require an identity verified by outer HTTP authentication; returns an interceptor and configuration error |
| `RequireAuth[T](ctx, getClaims)` | Retrieve the typed verified identity or return an unauthenticated error |
| `ClaimsGetter[T]` | Injected `func(context.Context) (T, bool)` paired with the outer middleware's setter |
| `ToConnectError(appErr, service)` | Encode shared google.rpc details; returns `(*connect.Error, error)` |
| `DecodeError(err)` | Return a remote `*rpc.Error` or an explicit decode error; never trust remote text as an AppError |
| `RetryDelay(err)` | Decode the server's minimum delay for a shared resilience policy |
| **client subpackage** | |
| `client.Config` | Client config: BaseURL, Timeout (or NoTimeout for long-lived streams), DialTimeout, Protocol, TLS |
| `client.NewHTTPClient(cfg)` | Create a native `net/http.Transport` HTTP/2 client (h2c or TLS) for ConnectRPC |
| `client.IsTransportFailure(err)` | Report a failure marked `client.ErrTransport`: the peer was unreachable or the connection broke mid-response, under any Connect code. Caller cancellation and a clean end of stream are not marked |
| `client.Unreachable(err)` | Report that the peer did not answer: a transport failure, `Unavailable`, `DeadlineExceeded`, `Canceled`, or a non-Connect error |
| `client.NewAvailability(name)` | Interceptor that records from real calls whether a peer answered; its `Health()` is degraded while the peer is unreachable. `Observe` lets the caller record outcomes the interceptor cannot see; failures after the caller's own context ended are ignored |
| `client.ClientOptions(cfg)` | Build connect.ClientOption slice from config |
| `client.ProtocolOption(cfg)` | Get wire protocol option (gRPC, gRPC-Web, or nil) |

---

Authenticate at the outer HTTP boundary, then install the RPC normalizer outside validation: logging → normalization → server deadline → identity guard → validation → handler. Validation returns application errors so the outer boundary can log evaluation causes before serialization. Deadlines only cancel work that honors its context.

The [shared error contract](../errors/README.md#shared-rpc-contract) defines namespaced identities, explicit retry verdicts, semantic violations, and HTTP-only extensions. Connect's HTTP status is protocol-owned. Client retry execution still requires idempotency and a bounded policy; `IsRetryable` alone does not authorize another attempt.

[← Back to main gokit README](../README.md)
