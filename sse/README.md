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
    "github.com/kbukum/gokit/sse"
)

func main() {
    hub := sse.NewHub()
    go hub.Run()

    // SSE endpoint
    http.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
        clientID := r.URL.Query().Get("id")
        sse.ServeSSE(hub, w, r, clientID,
            sse.WithUserID("user-123"),
            sse.WithMetadata("role", "admin"),
        )
    })

    // Broadcast to clients matching a pattern
    hub.BroadcastToPattern("user-*", []byte(`{"type":"update","data":"hello"}`))

    http.ListenAndServe(":8080", nil)
}
```

## Key Types & Functions

| Name | Description |
|------|-------------|
| `Hub` | Manages SSE client connections and broadcasting |
| `Client` | Connected SSE client with metadata |
| `NewHub()` / `Run()` | Create and start the hub |
| `ServeSSE()` | HTTP handler for SSE connections |
| `BroadcastToPattern()` | Send data to clients matching a pattern |
| `Broadcaster` | Interface for broadcasting events |
| `WithUserID()` / `WithSessionID()` / `WithMetadata()` | Client options |
| `EventType*` | Constants: connected, keepalive, message, error, metric |

## Backpressure semantics

- `Hub` uses a bounded inbound broadcast queue of `DefaultBroadcastBufferSize`.
- Each client uses a bounded delivery queue of `DefaultClientBufferSize`.
- Delivery is best-effort, not durable: when a slow client's queue is full, the newest frame is dropped and `SendFrame` returns `false`.
- Slow clients never block the hub or other subscribers; callers should rely on reconnect + replay from their own durable source when lossless delivery matters.

## Operational notes

- `ServeSSE` sends keepalive comments every `DefaultKeepAliveInterval`.
- `Hub.Stop()` disconnects all clients and makes subsequent broadcasts no-ops.

---

[⬅ Back to main README](../README.md)
