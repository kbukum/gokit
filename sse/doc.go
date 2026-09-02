// Package sse provides Server-Sent Events (SSE) infrastructure for real-time event delivery in gokit applications.
//
// It includes client connection management, event broadcasting to multiple subscribers,
// and a hub for managing event channels.
//
// # Architecture
//
//   - Hub: Central event router managing client subscriptions
//   - Broadcaster: Sends best-effort events to connected clients
//   - Bus: Typed, bounded multi-subscriber bus with event IDs and Last-Event-ID replay
//   - ServeSSE: HTTP handler for SSE endpoints
//
// # Backpressure
//
// The hub uses bounded queues for both inbound broadcasts and per-client delivery.
// Slow clients never block the hub: once a client's queue is full, new frames are dropped
// and the sender receives false from SendFrame. The Bus applies the same bounded,
// best-effort model to its per-subscriber queues and replay buffer.
//
// # Usage
//
//	hub := sse.NewHub()
//	go hub.Run()
//	router.GET("/events", func(w http.ResponseWriter, r *http.Request) {
//	    sse.ServeSSE(hub, w, r, "tenant:client-1")
//	})
//
// # Authentication
//
// ServeSSE accepts an injected [Authenticator] that gates the connection before
// the stream opens. Derive credentials from the Authorization header only — a
// token in the URL query string leaks into access logs, proxies, and browser
// history. [BearerAuthenticator] adapts any structural [TokenValidator] (such as
// auth.TokenValidator) without importing the auth module, keeping this transport
// layer decoupled from identity. A [WithClientIdentity] resolver maps the
// verified principal to a routing key so broadcasts scope per-subject:
//
//	sse.ServeSSE(hub, w, r, "",
//	    sse.WithAuthenticator(sse.BearerAuthenticator(validator)),
//	    sse.WithClientIdentity(func(_ *http.Request, identity any) (string, []sse.ClientOption, error) {
//	        claims := identity.(*Claims)
//	        return "user:" + claims.Subject, []sse.ClientOption{sse.WithUserID(claims.Subject)}, nil
//	    }),
//	)
package sse
