// Package sse provides Server-Sent Events (SSE) infrastructure for real-time event delivery in gokit applications.
//
// It includes client connection management, event broadcasting to multiple subscribers, and a hub for managing event channels.
//
// # Architecture
//
//   - Hub: Central event router managing client subscriptions
//   - Broadcaster: Sends best-effort events to connected clients
//   - ServeSSE: HTTP handler for SSE endpoints
//
// # Backpressure
//
// The hub uses bounded queues for both inbound broadcasts and per-client delivery. Slow clients never block the hub: once a client's queue is full, new frames are dropped and the sender receives false from SendFrame.
//
// # Usage
//
//	hub := sse.NewHub()
//	go hub.Run()
//	router.GET("/events", func(w http.ResponseWriter, r *http.Request) {
//	    sse.ServeSSE(hub, w, r, "tenant:client-1")
//	})
package sse
