# sse

Scoped, resumable Server-Sent Events for one service instance. One `Bus` owns replay, routing, connection admission, and live queues. Application messages use proto-JSON and their full protobuf message names. Slow clients receive a reset and disconnect instead of silently losing events.

## Wire an endpoint

Create the bus at the composition root and inject it into publishers, the endpoint, and `NewComponent(bus)`. The bus needs no dispatcher goroutine.

```go
func endpoint(log *logging.Logger, authorize sse.Authorizer) (*sse.Bus, http.Handler, error) {
    bus, err := sse.NewBus(sse.DefaultLimits())
    if err != nil {
        return nil, nil, err
    }
    cfg := sse.DefaultHandlerConfig()
    cfg.Logger = log
    cfg.Authorize = authorize
    handler, err := sse.NewHandler(bus, cfg)
    if err != nil {
        bus.Close()
        return nil, nil, err
    }
    return bus, handler, nil
}
```

An `Authorizer` authenticates the request and returns a verified `Access{Principal, Route, Lifetime}`. `Principal` is the connection-limit key; `Route` is the authorized request scope. Neither comes directly from unverified request parameters. `Authenticated[T](authenticator, resolver)` composes an `Authenticator[T]` with `func(*http.Request, T) (Access, error)`. The resolver receives the same typed identity both as an argument and through `IdentityFromContext[T]`, without losing upstream context values. Different identity types use distinct context keys. `PublicAccess` explicitly exposes a public scope and shares one admission principal across its visitors. There is no implicit public fallback.

Use `Bus.Publish(ctx, pattern, message)` from trusted application adapters. `*` matches any route, including `/`; `?` matches one rune, and brackets are literal. Subscriber routes cannot contain wildcards. The same matching rule applies to replay. Publication errors are returned, including oversized messages and a closed bus. The caller owns error handling.

`sse/worker.Forward` maps worker events to a `Publication` through an explicit typed proto mapper. It runs in its caller's lifecycle and stops on cancellation, mapping/publication failure, or `worker.ErrEventOverflow`. Overflow is incomplete delivery, not normal completion. Worker execution does not wait for event readers; the task owner decides whether forwarding failure should cancel execution. `Result` remains authoritative.

Register the same bus through `sse.NewComponent` and mount its handler on the HTTP server. Registry quiescing closes the bus before HTTP draining begins. Unregister metric callbacks during resource cleanup, before closing the meter provider.

## Wire contract

The cursor is `<epoch>:<sequence>`. An epoch is 32 lowercase hex characters generated per bus instance. A sequence is a canonical unsigned 64-bit decimal string, including `0`, with no sign or leading zeroes. Parse sequences as `bigint` in JavaScript, not `number`.

| Frame | Payload | Cursor rule |
| --- | --- | --- |
| Full proto message name | Proto-JSON using the schema's JSON field names | `id:` contains the application cursor |
| `connected` | `{"epoch":"…","cursor":"…"}` | Atomic subscription boundary, not acknowledged delivery |
| `reset` | `{"reason":"epochChanged\|replayExpired\|overflow","cursor":"…"}` | New live boundary; invalidate snapshots |
| `failure` | `code`, safe `message`, `retryable`, optional `reason`, `violations`, `retryAfter`, `traceId` | No ID; settle the connection, then EOF |
| Comment | `: keepalive` | No application meaning |

Controls never carry `id:`. SSE decoders can carry forward the previous ID onto a control frame; clients must not treat it as a new acknowledgement. Failure JSON comes from `errors.Failure`, not direct `AppError` serialization. Retry delays are minimum seconds, including fractions. Explicit retry false is preserved. A retry hint never grants operation idempotency.

A missing `Last-Event-ID` starts live at the subscription boundary. A malformed cursor or a future sequence in the current epoch is rejected with HTTP 422 before allocating a queue. A foreign epoch or expired replay emits `reset` and starts live at a fresh boundary. Replay and live events use one global sequence; gaps caused by authorized filtering are valid, not evidence of loss. Changing authorized scope requires clearing the client cursor.

If a subscriber's live queue fills, its queued data is discarded and a priority `overflow` reset is sent, followed by EOF. A replay reader overtaken by eviction gets `replayExpired` and EOF. Controls use a separate slot, not the data queue. `Bus.Fail` terminates matching active streams; `Subscription.Fail` terminates an in-process subscription. Failures are not replayed.

## Recovery and snapshots

Use a fetch-based SSE client when delivery acknowledgement matters. Native `EventSource` advances its internal reconnect ID on receipt, before asynchronous application delivery.

1. Retain only the last successfully applied application cursor. Ignore duplicate or older sequences in the same epoch; never require contiguous sequences.
2. Subscribe before requesting a snapshot. On reset, clear the acknowledged cursor, discard buffered old-generation events, and invalidate any in-flight snapshot. An overflow-closed connection reopens without its old cursor.
3. Coalesce refetch requests. Without an atomic snapshot watermark from the application, treat events as cache invalidations rather than replaying arbitrary deltas onto a racing snapshot.
4. Accept a snapshot only if neither the connection generation nor the applied-event revision changed while it was fetched. Otherwise discard it and refetch within a finite budget. Once the budget is exhausted, keep the state visibly stale; do not report convergence.

The [convergence fixtures](testdata/convergence.json) use a two-refetch budget and cover delivery acknowledgement, duplicates, filtered gaps, live/snapshot races, reset during refetch, repeated resets, and sustained churn. Their Go oracle specifies outcomes, not a shipped browser client. Consuming kits must execute them against their real cache and channel lifecycle.

## Resource bounds and ownership

| Resource | Default | Allowed configuration |
| --- | --- | --- |
| Replay events | 1,024 | 1–1,048,576 |
| Replay bytes, including frames and routing patterns | 8 MiB | 256 B–1 GiB |
| Live queue per subscription | 32 events | 1–65,536 |
| Encoded application/failure frame | 64 KiB | 256 B–1 MiB |
| Connections per instance | 1,024 | 1–1,048,576 |
| Connections per principal | 8 | 1–instance limit |
| Routing keys and patterns | 512 bytes | Fixed |
| Per-write deadline | 10 seconds | Positive duration |
| Heartbeat interval | 30 seconds | Positive duration |

Replay is a ring bounded by both count and bytes. Subscribers read replay directly from that ring, not from replay-sized private queues. Live queues store immutable frame strings. A conservative payload bound is replay bytes plus `connections × queue events × max frame bytes`, plus one control and one in-flight frame per connection. Ring slots and subscription metadata add fixed overhead. Proto encoding is serialized per bus; input wire size is checked before proto-JSON allocation and encoded size before retention. Configure all limits together for the deployment's memory budget.

Admission is checked before allocating a queue. An overflowed stream stays charged until its owner closes it, so repeated admission cannot bypass the bound while old handlers are unwinding. `Subscribe` callers close subscriptions; context cancellation also releases them. HTTP owns its subscription, heartbeat ticker, write, and cancellation callbacks.

Every frame and heartbeat gets its own write deadline, which is cleared once the frame is flushed, so a quiet stream (including HTTP/2) stays open between heartbeats. Any write, short write, or flush error ends the handler. A response wrapper must support `ResponseController` deadlines and flushes, usually through `Unwrap`; unsupported deadlines fail closed. Middleware must not replace a streaming writer with an unbounded buffer.

The generic decoder separately bounds normalized event-block bytes with `WithMaxFrameSize` and line bytes with `WithMaxLineSize`, both 1 MiB by default. Line limits exclude CR/LF delimiters but include a leading BOM. Limits smaller than the scanner's usual buffer are enforced exactly. A multi-line frame cannot bypass the aggregate bound.

## Authentication and deployment

Browser credentials belong in secure HttpOnly cookies; automation credentials belong in headers. `BearerAuthenticator[T]` accepts a `TokenValidator[T]` with `ValidateToken(context.Context, string) (T, error)` and forwards the request context. It reads exactly one bounded authorization header, never a URL parameter; duplicate, empty, oversized, malformed, or invalid credentials fail closed. An injected `Authenticator[T]` can instead compose the common HTTP cookie/API-key chain or retrieve the already-verified identity from its paired context getter. Authentication and scope checks finish before streaming begins. Pre-stream failures use the shared RFC 9457 encoding and normalization rules; unknown errors are safe internal failures, not successful anonymous access. `WithChallenge` adds a challenge only to a 401 response.

`Access.Lifetime` is an authoritative owner context. Its cancellation or expiry cancels the HTTP stream and interrupts a blocked write. Session implementations must return a lifetime tied atomically to their expiry/revocation state; checking a database row once without binding a lifetime is insufficient. SSE does not parse cookies, sessions, roles, or workspace membership. A closed connection alone is not proof that a browser is authenticated; terminal auth failures settle the session, and unexpected closes need a bounded reconnect/revalidation policy.

Replay and subscribers exist in one instance's memory. Reconnecting to another instance changes epoch and requires reset/refetch. Cross-instance live delivery needs a shared bus behind the publisher seam; cross-instance session revocation additionally needs signalling or bounded revalidation with a declared SLA. `Component.Stop` closes the bus; composition owns HTTP draining and its shutdown budget.

## Metrics and conformance

`Bus.Stats` exposes coherent active-stream, queue-depth/bytes, replay-count/bytes, drop, reset, and rejected-connection measurements. `sse/metrics.Register(bus, meter)` exports these through an injected OpenTelemetry meter with no identity or raw-route labels. The composition root unregisters the callback before telemetry shutdown.

The [protocol frames](testdata/protocol.json) cover scoped replay, malformed/future cursors, epoch changes, eviction, and overflow. Shared [failure fixtures](../contracttest/wire/testdata/wire/) include actual SSE frames for every application failure category, alongside Connect, gRPC, and problem JSON. Tests compare decoded proto-JSON semantically because insignificant whitespace is not part of protobuf's serialization guarantee.

Consumers vendor these fixtures from an immutable gokit commit and record that commit plus file digests. `sourceBaseCommit` in the protocol fixture identifies the baseline used to develop this contract; it is not the publication commit. Do not pin a moving branch or claim downstream adoption from these Go tests alone.

`sse/testutil` supplies the real HTTP harness, decoder client, authentication doubles, and deadline-aware fault-injecting response writer. Package tests use race/shuffle, virtual-time timer checks, exact boundary/overflow assertions, a never-reading peer with a 50 ms deadline and 2-second settling ceiling, and package-wide goleak.
