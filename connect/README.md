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
if err := goconnect.Mount(srv, path, handler); err != nil {
    return err
}
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

The client uses native HTTP/2 transport controls. Enabled TLS requires `https://`; the default h2c mode requires `http://`. Each request is checked before dialing, so neither mode silently falls back to HTTP/1. Redirects, including same-origin redirects, fail with `client.ErrRedirect`: RPC credentials and bodies never follow a peer's `Location` header. Call `httpClient.CloseIdleConnections()` when its owner shuts down.

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
if err := goconnect.Mount(srv, path, authenticate(handler)); err != nil {
    return err
}
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
| `MountServices(srv, ...Service)` | Mount multiple services at once, stopping at the first error |
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
| `client.Config` | Client config: BaseURL, Timeout (or NoTimeout for long-lived streams), DialTimeout, Protocol, TLS. Negative timeouts fail validation. A set BaseURL must be an absolute `http` or `https` URL without credentials, query or fragment, and its scheme must match the transport: `https` with TLS settings, `http` (h2c) without |
| `client.NewHTTPClient(cfg)` | Create a native `net/http.Transport` HTTP/2 client (h2c or TLS) for ConnectRPC |
| `client.IsTransportFailure(err)` | Report a failure marked `client.ErrTransport`: the peer was unreachable, a gateway answered 502, 503 or 504 without an RPC error, or the connection broke mid-response, under any Connect code. Caller cancellation and a clean end of stream are not marked |
| `client.IsUnavailable(err)` | Report a marked transport failure, context/unary/first-message timeout, or a received `Unavailable`, `DeadlineExceeded` or `Canceled`. Other locally constructed errors carry no peer evidence, regardless of their code |
| `client.FirstMessageTimeoutInterceptor(limit, clock)` | Interceptor that ends a server or bidi stream whose peer sends no first message within `limit`, counted from opening the stream, with a `deadline_exceeded` error wrapping `ErrFirstMessageTimeout`. Bounds `NoTimeout` streams against a peer that accepts but never answers. Install it after `NewAvailability` so the outage is recorded against the caller's context |
| `client.UnaryTimeoutInterceptor(limit)` | Interceptor that bounds each unary client call by `limit`. When the limit passes while the caller's context is live, the call fails with a `deadline_exceeded` error wrapping `ErrUnaryTimeout`, so a hung peer counts as an outage instead of reading as the caller's own deadline. Install it after `NewAvailability`. Streams pass through |
| `client.MapCallFailure(ctx, peer, err)` | Map outages to local errors while preserving causes: `Timeout` or `Canceled` if the caller ended, otherwise `ServiceUnavailable` with reason `ReasonUnavailable` (`PEER_UNAVAILABLE`). Preserves validated remote retry verdicts and minimum delays without exposing remote messages. Malformed details become non-retryable `Internal` errors. Returns `false` for success, clean EOF, local rejections and non-outage peer errors |
| `client.NewAvailability(name)` | Interceptor that records peer availability from real calls. Success, clean EOF and non-outage wire errors mark it available. Observed outages mark it unavailable unless the caller's context ended or is within 1ms of its deadline (Connect propagates deadlines truncated to milliseconds, so a peer can enforce the caller's deadline first; `MapCallFailure` then reports the caller's timeout too); local validation/policy errors leave the previous state unchanged. `Health()` is degraded while unavailable; `Observe` records adapter-specific outcomes |
| `client.ClientOptions(cfg)` | Build connect.ClientOption slice from config |
| `client.ProtocolOption(cfg)` | Get wire protocol option (gRPC, gRPC-Web, or nil) |

---

Authenticate at the outer HTTP boundary, then install the RPC normalizer outside validation: logging → normalization → server deadline → identity guard → validation → handler. Validation returns application errors so the outer boundary can log evaluation causes before serialization. Deadlines only cancel work that honors its context.

The [shared error contract](../errors/README.md#shared-rpc-contract) defines namespaced identities, explicit retry verdicts, semantic violations, and HTTP-only extensions. Connect's HTTP status is protocol-owned. Client retry execution still requires idempotency and a bounded policy; `IsRetryable` alone does not authorize another attempt.

[← Back to main gokit README](../README.md)
