# sse

Server-Sent Events hub for real-time client communication with pattern-based broadcasting.

## Install

```bash
go get github.com/skillsenselab/gokit
```

## Quick Start

```go
package main

import (
    "net/http"
    "github.com/skillsenselab/gokit/sse"
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

---

[⬅ Back to main README](../README.md)
