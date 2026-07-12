package sse

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/kbukum/gokit/logging"
)

const DefaultKeepAliveInterval = 30 * time.Second

// ConnectedEvent is sent when a client successfully connects.
type ConnectedEvent struct {
	ClientID  string            `json:"client_id"`
	UserID    string            `json:"user_id,omitempty"`
	SessionID string            `json:"session_id,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// ServeSSE handles an SSE connection for a specific client.
// This is the main entry point called from HTTP handlers.
func ServeSSE(hub *Hub, w http.ResponseWriter, r *http.Request, clientID string, opts ...ClientOption) {
	// Check SSE support (requires http.Flusher interface)
	flusher, ok := w.(http.Flusher)
	if !ok {
		logging.ErrorCtx(r.Context(), "[SSE] Streaming not supported", map[string]interface{}{
			"client_id": clientID,
		})
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Disable write deadline for SSE connections using ResponseController.
	// This is essential because SSE connections are long-lived and shouldn't be
	// terminated by the server's WriteTimeout setting.
	rc := http.NewResponseController(w)
	if err := rc.SetWriteDeadline(time.Time{}); err != nil {
		logging.WarnCtx(r.Context(), "[SSE] Could not disable write deadline", map[string]interface{}{
			"client_id": clientID,
			"error":     err.Error(),
		})
		// Continue anyway - the connection might still work with keep-alives
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering

	// Create and register client with options
	client := NewClient(clientID, opts...)
	hub.Register(client)
	defer func() {
		hub.Unregister(client)
	}()

	// Send initial connection event
	connectedEvent := ConnectedEvent{
		ClientID:  clientID,
		UserID:    client.UserID(),
		SessionID: client.SessionID(),
		Metadata:  client.Metadata(),
	}
	connectedData, _ := json.Marshal(connectedEvent)
	_, _ = fmt.Fprintf(w, "event: %s\n", EventTypeConnected)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", connectedData)
	flusher.Flush()

	logging.DebugCtx(r.Context(), "[SSE] Client connected", map[string]interface{}{
		"client_id":   clientID,
		"user_id":     client.UserID(),
		"session_id":  client.SessionID(),
		"remote_addr": r.RemoteAddr,
	})

	// Event loop - stream events to client
	// Keep-alive interval should be less than proxy timeouts (typically 60s).
	keepAlive := time.NewTicker(DefaultKeepAliveInterval)
	defer keepAlive.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			// Client disconnected (browser closed, network issue, etc.)
			logging.DebugCtx(ctx, "[SSE] Client disconnected", map[string]interface{}{
				"client_id": clientID,
				"reason":    ctx.Err().Error(),
			})
			return

		case frame, ok := <-client.Events():
			if !ok {
				// Channel closed, client unregistered
				logging.DebugCtx(ctx, "[SSE] Events channel closed", map[string]interface{}{
					"client_id": clientID,
				})
				return
			}
			// Send SSE frame: optional `event:` line + `data:` payload.
			// Browser EventSource named-event listeners only fire when the
			// frame includes an `event:` line matching the listener name.
			if frame.Event != "" {
				_, _ = fmt.Fprintf(w, "event: %s\n", frame.Event)
			}
			_, _ = fmt.Fprintf(w, "data: %s\n\n", frame.Data)
			flusher.Flush()
			logging.DebugCtx(ctx, "[SSE] Event sent", map[string]interface{}{
				"client_id": clientID,
				"event":     frame.Event,
				"data_size": len(frame.Data),
			})

		case <-keepAlive.C:
			// Send keep-alive comment (SSE spec: lines starting with : are comments)
			// This keeps the connection alive through proxies and load balancers
			_, _ = fmt.Fprintf(w, ": keepalive %d\n\n", time.Now().Unix())
			flusher.Flush()
			logging.DebugCtx(ctx, "[SSE] Keep-alive sent", map[string]interface{}{
				"client_id": clientID,
			})
		}
	}
}
