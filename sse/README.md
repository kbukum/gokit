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
    "net/http"

    "github.com/google/uuid"
    "github.com/kbukum/gokit/sse"
)

func main() {
    hub := sse.NewHub()
    go hub.Run()

    // SSE endpoint — authenticate from the Authorization header only.
    http.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
        // Pass a unique per-connection id so concurrent streams for one principal
        // never evict each other; WithClientIdentity sets the shared routing key.
        sse.ServeSSE(hub, w, r, uuid.NewString(),
            sse.WithAuthenticator(sse.BearerAuthenticator(validator)),
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

> `validator` is any `auth.TokenValidator` (JWT, OIDC, API key, …). It is injected,
> not imported: `sse` is a transport layer and never depends on the `auth` module.

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

Credentials are read from the `Authorization` header **only**. A token in the URL
query string (`?token=…` / `?id=…`) leaks into access logs, proxy logs, the
`Referer` header, and browser history — so `sse` never reads one. The native
browser `EventSource` cannot set custom headers; for authenticated browser
streams use an `EventSource` polyfill built on `fetch` + `ReadableStream` (which
can send `Authorization`), or an `HttpOnly; Secure` session cookie. Both keep the
credential out of the URL.

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
option types. When this mirroring is scheduled, record it in an rskit tracking
issue (https://github.com/kbukum/rskit) and link it here.

---

[⬅ Back to main README](../README.md)
