package sse

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	apperrors "github.com/kbukum/gokit/errors"
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
//
// When a [WithAuthenticator] option is supplied, the connection is authenticated
// before the stream opens: on rejection the handler writes the mapped 401/403
// status and returns without registering a client or emitting any frame. Derive
// credentials from the Authorization header only (see [BearerAuthenticator]) —
// never the query string. A [WithClientIdentity] resolver can then attach a
// per-principal routing key so broadcasts scope to the authenticated subject
// while clientID stays the unique per-connection registration key, letting
// several concurrent streams for one principal coexist.
func ServeSSE(hub *Hub, w http.ResponseWriter, r *http.Request, clientID string, opts ...ServeOption) {
	cfg := newServeConfig(opts...)

	clientOpts := cfg.clientOpts
	if cfg.authenticator != nil {
		identity, err := cfg.authenticator.Authenticate(r)
		if err == nil && identity == nil {
			// An authenticator that returns (nil, nil) would admit the connection
			// while IdentityFromContext reports it unauthenticated and any resolver
			// that type-asserts the identity panics. Treat a missing identity as a
			// rejection so success always carries a usable principal.
			err = apperrors.Unauthorized("authenticator returned no identity")
		}
		if err != nil {
			rejectConnection(hub, r, w, clientID, "[SSE] Authentication rejected", err)
			return
		}
		r = r.WithContext(withIdentity(r.Context(), identity))
		resolvedOpts, ok := resolveIdentity(hub, r, w, clientID, cfg, identity, clientOpts)
		if !ok {
			return
		}
		clientOpts = resolvedOpts
	}

	// Check SSE support (requires http.Flusher interface)
	flusher, ok := w.(http.Flusher)
	if !ok {
		hub.log.ErrorCtx(r.Context(), "[SSE] Streaming not supported", map[string]any{
			"client_id": clientID,
		})
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Disable write deadline for SSE connections using ResponseController.
	// This is essential because SSE connections are long-lived
	// and shouldn't be terminated by the server's WriteTimeout setting.
	rc := http.NewResponseController(w)
	if err := rc.SetWriteDeadline(time.Time{}); err != nil {
		hub.log.WarnCtx(r.Context(), "[SSE] Could not disable write deadline", map[string]any{
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
	client := NewClient(clientID, clientOpts...)
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

	hub.log.DebugCtx(r.Context(), "[SSE] Client connected", map[string]any{
		"client_id":   clientID,
		"user_id":     client.UserID(),
		"session_id":  client.SessionID(),
		"remote_addr": r.RemoteAddr,
	})

	// Event loop - stream events to client Keep-alive interval should be less than proxy timeouts (typically 60s).
	keepAlive := time.NewTicker(DefaultKeepAliveInterval)
	defer keepAlive.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			// Client disconnected (browser closed, network issue, etc.)
			hub.log.DebugCtx(ctx, "[SSE] Client disconnected", map[string]any{
				"client_id": clientID,
				"reason":    ctx.Err().Error(),
			})
			return

		case frame, ok := <-client.Events():
			if !ok {
				// Channel closed, client unregistered
				hub.log.DebugCtx(ctx, "[SSE] Events channel closed", map[string]any{
					"client_id": clientID,
				})
				return
			}
			// Send SSE frame: optional `event:` line + `data:` payload.
			// Browser EventSource named-event listeners only fire when the frame includes an `event:` line matching the listener name.
			if frame.Event != "" {
				_, _ = fmt.Fprintf(w, "event: %s\n", frame.Event)
			}
			_, _ = fmt.Fprintf(w, "data: %s\n\n", frame.Data)
			flusher.Flush()
			hub.log.DebugCtx(ctx, "[SSE] Event sent", map[string]any{
				"client_id": clientID,
				"event":     frame.Event,
				"data_size": len(frame.Data),
			})

		case <-keepAlive.C:
			// Send keep-alive comment (SSE spec: lines starting with : are comments) This keeps the connection alive through proxies
			// and load balancers
			_, _ = fmt.Fprintf(w, ": keepalive %d\n\n", time.Now().Unix())
			flusher.Flush()
			hub.log.DebugCtx(ctx, "[SSE] Keep-alive sent", map[string]any{
				"client_id": clientID,
			})
		}
	}
}

// resolveIdentity runs the optional [IdentityResolver] for an authenticated
// request. It returns the client options to register with (resolver metadata
// first, then the verified routing key last so per-principal scoping always wins)
// and ok true; on a resolver rejection it writes the mapped 401/403 response and
// returns ok false. When no resolver is configured it returns clientOpts
// unchanged. The resolved value is a routing key, not a new connection id, so the
// caller's clientID stays the unique per-connection registration key and
// concurrent streams for one principal never evict one another.
func resolveIdentity(hub *Hub, r *http.Request, w http.ResponseWriter, clientID string, cfg *serveConfig, identity any, clientOpts []ClientOption) ([]ClientOption, bool) {
	if cfg.resolver == nil {
		return clientOpts, true
	}
	resolvedID, resolvedOpts, err := cfg.resolver(r, identity)
	if err != nil {
		rejectConnection(hub, r, w, clientID, "[SSE] Identity resolution rejected", err)
		return nil, false
	}
	clientOpts = append(clientOpts, resolvedOpts...)
	if resolvedID != "" {
		clientOpts = append(clientOpts, WithRoute(resolvedID))
	}
	return clientOpts, true
}

// rejectConnection logs a rejection by its canonical code only (never the raw
// error, which may carry credential or principal detail from an injected
// implementation) and writes the mapped RFC 9457 problem response, logging a
// failed body write on the runtime HTTP path.
func rejectConnection(hub *Hub, r *http.Request, w http.ResponseWriter, clientID, msg string, err error) {
	ctx := r.Context()
	hub.log.WarnCtx(ctx, msg, map[string]any{
		"client_id": clientID,
		"reason":    canonicalAuthError(err).Code,
	})
	if werr := writeAuthError(w, err); werr != nil {
		hub.log.ErrorCtx(ctx, "[SSE] Failed to write auth rejection", map[string]any{
			"client_id": clientID,
			"error":     werr.Error(),
		})
	}
}
