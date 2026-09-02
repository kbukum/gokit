# sse

Server-Sent Events hub for real-time client communication with pattern-based broadcasting.

## Install

```bash
go get github.com/kbukum/gokit
```

## Quick Start

```go
package main

import (
    "errors"
    "net/http"

    "github.com/google/uuid"
    "github.com/kbukum/gokit/sse"
)

// Claims is the caller-defined identity your validator returns.
type Claims struct{ Subject string }

// validator is any auth.TokenValidator (JWT, OIDC, API key, …). It is injected,
// not imported: sse is a transport layer and never depends on the auth module.
// This minimal stand-in keeps the example self-contained and compilable.
type validator struct{}

func (validator) ValidateToken(token string) (any, error) {
    if token == "" {
        return nil, errors.New("empty token")
    }
    return &Claims{Subject: "user-1"}, nil // replace with real verification
}

func main() {
    hub := sse.NewHub()
    go hub.Run()

    // SSE endpoint — authenticate from the Authorization header only.
    http.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
        // Pass a unique per-connection id so concurrent streams for one principal
        // never evict each other; WithClientIdentity sets the shared routing key.
        sse.ServeSSE(hub, w, r, uuid.NewString(),
            sse.WithAuthenticator(sse.BearerAuthenticator(validator{})),
            sse.WithClientIdentity(func(_ *http.Request, identity any) (string, []sse.ClientOption, error) {
                claims := identity.(*Claims)
                // Derive the broadcast routing key from the verified principal.
                return "user:" + claims.Subject, []sse.ClientOption{sse.WithUserID(claims.Subject)}, nil
            }),
        )
    })

    // Broadcast to clients matching a pattern
    hub.BroadcastToPattern("user:*", []byte(`{"type":"update","data":"hello"}`))

    http.ListenAndServe(":8080", nil)
}
```

> `validator` here is a stand-in for any `auth.TokenValidator` (JWT, OIDC, API
> key, …). It is injected, not imported: `sse` is a transport layer and never
> depends on the `auth` module.

## Authentication

`ServeSSE` accepts an injected `Authenticator` that gates the connection **before**
the stream opens. On rejection it writes an RFC 9457 problem response with the
mapped status and never starts the event loop — no partial stream is emitted.

| Option | Purpose |
|--------|---------|
| `WithAuthenticator(a)` | Gate the connection; run before the stream opens. |
| `BearerAuthenticator(v)` | Adapt any `TokenValidator` to read `Authorization: Bearer <token>`. |
| `WithClientIdentity(fn)` | Derive the per-principal routing key + metadata from the verified identity. |
| `IdentityFromContext(ctx)` | Recover the resolved identity downstream. |

Failure semantics are explicit:

- **Missing or invalid credential** → `401 Unauthorized` (`BearerAuthenticator`, or
  any authenticator returning a non-`Forbidden` error).
- **Authenticated but not permitted** → `403 Forbidden` (return `errors.Forbidden(...)`
  from the authenticator or the `WithClientIdentity` resolver).

### Why header-only, never the query string

The invariant is simple: **no credential is ever read from the URL query string**
(`?token=…` / `?id=…`), because a token there leaks into access logs, proxy logs,
the `Referer` header, and browser history. Everything below keeps the credential
out of the URL.

The built-in `BearerAuthenticator` reads the token from the `Authorization` header
**only**. The general `Authenticator` seam is broader — it receives the full
`*http.Request`, so a custom authenticator may instead read an `HttpOnly; Secure`
session cookie, which is also kept out of the URL. The native browser
`EventSource` cannot set custom headers; for authenticated browser streams either
use an `EventSource` polyfill built on `fetch` + `ReadableStream` (which can send
`Authorization`), or authenticate from a secure cookie. Both honour the no-query
invariant.

## Testing

`sse/testutil` provides a reusable harness so consumers test their wiring without
hand-rolling SSE fakes: a call-counting `FakeAuthenticator` (`AllowAuthenticator`,
`RejectUnauthorized`, `RejectForbidden`), a `Harness` (running hub + httptest
server), and a `StreamClient` that decodes framed events and asserts on the
200/401/403 paths.

## Key Types & Functions

| Name | Description |
|------|-------------|
| `Hub` | Manages SSE client connections and broadcasting |
| `Client` | Connected SSE client with metadata |
| `NewHub()` / `Run()` | Create and start the hub |
| `ServeSSE()` | HTTP handler for SSE connections (accepts `ServeOption`s) |
| `Authenticator` / `AuthenticatorFunc` | Connection auth gate: `Authenticate(r) (identity, err)` |
| `BearerAuthenticator()` | Header-only `Authorization: Bearer` adapter over a `TokenValidator` |
| `WithAuthenticator()` / `WithClientIdentity()` | Serve options: auth gate, per-principal routing |
| `IdentityFromContext()` | Recover the resolved identity downstream |
| `BroadcastToPattern()` | Send data to clients matching a pattern |
| `Broadcaster` | Interface for broadcasting events |
| `WithClientOptions()` + `WithUserID()` / `WithSessionID()` / `WithMetadata()` | Client metadata options |
| `EventType*` | Constants: connected, keepalive, message, error, metric |

## Backpressure semantics

- `Hub` uses a bounded inbound broadcast queue of `DefaultBroadcastBufferSize`.
- Each client uses a bounded delivery queue of `DefaultClientBufferSize`.
- Delivery is best-effort, not durable: when a slow client's queue is full,
  the newest frame is dropped and `SendFrame` returns `false`.
- Slow clients never block the hub or other subscribers;
  callers should rely on reconnect + replay from their own durable source when lossless delivery matters.

## Operational notes

- `ServeSSE` sends keepalive comments every `DefaultKeepAliveInterval`.
- `Hub.Stop()` disconnects all clients and makes subsequent broadcasts no-ops.

## Cross-kit parity

The header-only authentication seam is a gokit-side security correction. Its
rskit counterpart (the SSE/streaming transport) should mirror the same concept
under the same name: an injected authenticator that derives credentials from the
`Authorization` header, rejects query-string tokens, and applies 401/403
semantics. Parity is scoped to the wire contract and the concept, not the Go
option types. No rskit tracking issue exists for this mirroring yet; when the
work is scheduled, open one in the [rskit](https://github.com/kbukum/rskit)
tracker and link its full issue URL here.

---

[⬅ Back to main README](../README.md)
